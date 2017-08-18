package github

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/bradleyfalzon/gopherci/internal/analyser"
	"github.com/bradleyfalzon/gopherci/internal/db"
	"github.com/google/go-github/github"
	"github.com/pkg/errors"
)

// CallbackHandler is the net/http handler for github callbacks. This may
// have only been for old integration behaviour, as it doesn't appear to
// be documented anymore, but because our installation still has it set, it's
// still used for us. So just redirect.
//
// https://web-beta.archive.org/web/20161114212139/https://developer.github.com/early-access/integrations/creating-an-integration/
// https://web-beta.archive.org/web/20161114200029/https://developer.github.com/early-access/integrations/identifying-users/
func (g *GitHub) CallbackHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	target := r.Form.Get("target_url")
	if target == "" {
		http.Error(w, "no target_url set", http.StatusBadRequest)
		return
	}
	// No open redirects
	if !strings.HasPrefix(target, g.gciBaseURL) {
		http.Error(w, "invalid target_url", http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// WebHookHandler is the net/http handler for github webhooks.
func (g *GitHub) WebHookHandler(w http.ResponseWriter, r *http.Request) {
	logger := g.logger.With("deliveryID", github.DeliveryID(r))

	payload, err := github.ValidatePayload(r, g.webhookSecret)
	if err != nil {
		logger.With("error", err).Error("failed to validate payload")
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	event, err := github.ParseWebHook(github.WebHookType(r), payload)
	if err != nil {
		if strings.HasPrefix(err.Error(), "unknown X-Github-Event in message: integration_installation") {
			// Ignore error message about deprecated integration_installation and integration_installation_repositories events.
			// Remove after November 22nd 2017.
			// https://github.com/google/go-github/issues/627#issuecomment-304146513
			return
		}
		logger.With("error", err).Error("failed to parse webhook")
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	switch e := event.(type) {
	case *github.InstallationEvent:
		logger = logger.With("installationID", *e.Installation.ID).With("event", "InstallationEvent")
		err = g.integrationInstallationEvent(e)
	case *github.PushEvent:
		var installation *Installation
		logger = logger.With("installationID", *e.Installation.ID).With("event", "PushEvent")
		if installation, err = g.NewInstallation(*e.Installation.ID); err != nil {
			break
		}
		if !installation.IsEnabled() {
			err = &ignoreEvent{reason: ignoreNoInstallation}
			break
		}
		if !checkPushAffectsGo(e) {
			err = &ignoreEvent{reason: ignoreNoGoFiles}
			break
		}
		if e.Repo.GetPrivate() {
			err = &ignoreEvent{reason: ignorePrivateRepos}
			break
		}
		g.queuePush <- e
	case *github.PullRequestEvent:
		logger = logger.With("installationID", *e.Installation.ID).With("event", "PullRequestEvent").With("action", *e.Action)
		if err = checkPRAction(e); err != nil {
			break
		}
		var (
			installation *Installation
			ok           bool
		)
		if installation, err = g.NewInstallation(*e.Installation.ID); err != nil {
			break
		}
		if !installation.IsEnabled() {
			err = &ignoreEvent{reason: ignoreNoInstallation}
			break
		}
		if e.Repo.GetPrivate() || e.PullRequest.Head.Repo.GetPrivate() || e.PullRequest.Base.Repo.GetPrivate() {
			err = &ignoreEvent{reason: ignorePrivateRepos}
			break
		}
		ok, err = checkPRAffectsGo(r.Context(), installation, *e.Repo.Owner.Login, *e.Repo.Name, *e.Number)
		if err != nil {
			break
		}
		if !ok {
			err = &ignoreEvent{reason: ignoreNoGoFiles}
			break
		}
		g.queuePush <- e
	default:
		err = &ignoreEvent{reason: ignoreUnknownEvent}
	}

	switch err.(type) {
	case nil:
	case *ignoreEvent:
		logger.With("error", err).Info("ignoring event")
	default:
		logger.With("error", err).Error("cannot handle event")
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	logger.Info("received event")
}

type ignoreReason int

const (
	ignoreUnknownEvent ignoreReason = iota
	ignoreInvalidAction
	ignoreNoAction
	ignoreNoInstallation
	ignoreNoGoFiles
	ignorePrivateRepos
)

// ignoreEvent indicates the event should be accepted but ignored.
type ignoreEvent struct {
	reason ignoreReason
	extra  string
}

// Error implements the error interface.
func (e *ignoreEvent) Error() string {
	switch e.reason {
	case ignoreUnknownEvent:
		return "unknown event"
	case ignoreInvalidAction:
		return "invalid action: " + e.extra
	case ignoreNoAction:
		return "no action"
	case ignoreNoInstallation:
		return "no enabled installation found"
	case ignoreNoGoFiles:
		return "no go files affected"
	case ignorePrivateRepos:
		return "private repositories are not yet supported"
	}
	return e.extra
}

// checkPRAction checks a pull request's action to determine whether the event
// should continue to be processed. Returns error type *ignoreEvent if the event
// should be ignored, nil if it should be processed, or other error if check
// could not be completed.
func checkPRAction(e *github.PullRequestEvent) error {
	if e.Action == nil {
		return &ignoreEvent{reason: ignoreNoAction}
	}
	if *e.Action != "opened" && *e.Action != "synchronize" && *e.Action != "reopened" {
		return &ignoreEvent{reason: ignoreInvalidAction, extra: *e.Action}
	}
	return nil
}

const configFilename = ".gopherci.yml"

// checkPRAffectsGo returns true if a pull request modifies, adds or removes
// Go files, else returns error if an error occurs.
func checkPRAffectsGo(ctx context.Context, installation *Installation, owner, repo string, number int) (bool, error) {
	opt := &github.ListOptions{PerPage: 100}
	for {
		files, resp, err := installation.client.PullRequests.ListFiles(ctx, owner, repo, number, opt)
		if err != nil {
			return false, errors.Wrap(err, "could not list files")
		}
		for _, file := range files {
			if hasGoExtension(*file.Filename) || *file.Filename == configFilename {
				return true, nil
			}
		}
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}
	return false, nil
}

// checkPushAffectsGo returns true if the event modifies, adds or removes Go files.
func checkPushAffectsGo(event *github.PushEvent) bool {
	hasGoFile := func(files []string) bool {
		for _, filename := range files {
			if hasGoExtension(filename) || filename == configFilename {
				return true
			}
		}
		return false
	}
	for _, commit := range event.Commits {
		if hasGoFile(commit.Modified) || hasGoFile(commit.Added) || hasGoFile(commit.Removed) {
			return true
		}
	}
	return false
}

// hasGoExtension returns true if the filename has the suffix ".go".
func hasGoExtension(filename string) bool {
	return strings.HasSuffix(filename, ".go")
}

func (g *GitHub) integrationInstallationEvent(e *github.InstallationEvent) error {
	var err error
	switch *e.Action {
	case "created":
		// Record the installation event in the database
		err = g.db.AddGHInstallation(*e.Installation.ID, *e.Installation.Account.ID, *e.Sender.ID)
	case "deleted":
		// Remove the installation event from the database
		err = g.db.RemoveGHInstallation(*e.Installation.ID)
	}
	if err != nil {
		return errors.Wrap(err, "database error handling integration installation event")
	}
	return nil
}

// PushConfig returns an AnalyseConfig for a GitHub Push Event.
func PushConfig(e *github.PushEvent) AnalyseConfig {
	// commitFrom is after~numCommits for the same reason as baseRef but
	// also because first pushes's before is 000000.... which can't be
	// used in api request
	commitFrom := fmt.Sprintf("%v~%v", *e.After, len(e.Commits))
	if e.Created != nil && *e.Created {
		commitFrom = ""
	}

	return AnalyseConfig{
		cloner: &analyser.PushCloner{
			HeadURL: *e.Repo.CloneURL,
			HeadRef: *e.After,
		},
		refReader: &analyser.FixedRef{
			// baseRef is after~numCommits to better handle forced pushes, as a
			// forced push has the before ref of a commit that's been overwritten.
			BaseRef: fmt.Sprintf("%v~%v", *e.After, len(e.Commits)),
		},
		installationID:  *e.Installation.ID,
		repositoryID:    *e.Repo.ID,
		statusesContext: "ci/gopherci/push",
		statusesURL:     strings.Replace(*e.Repo.StatusesURL, "{sha}", *e.After, -1),
		commitFrom:      commitFrom,
		commitTo:        *e.After,
		commitCount:     len(e.Commits),
		headRef:         *e.After,
		goSrcPath:       stripScheme(*e.Repo.HTMLURL),
		owner:           *e.Repo.Owner.Name,
		repo:            *e.Repo.Name,
		sha:             *e.After,
	}
}

// PullRequestConfig return an AnalyseConfig for a GitHub Pull Request.
func PullRequestConfig(e *github.PullRequestEvent) AnalyseConfig {
	pr := e.PullRequest
	return AnalyseConfig{
		cloner: &analyser.PullRequestCloner{
			BaseURL: *pr.Base.Repo.CloneURL,
			BaseRef: *pr.Base.Ref,
			HeadURL: *pr.Head.Repo.CloneURL,
			HeadRef: *pr.Head.Ref,
		},
		refReader:       &analyser.MergeBase{},
		installationID:  *e.Installation.ID,
		repositoryID:    *e.Repo.ID,
		statusesContext: "ci/gopherci/pr",
		statusesURL:     *pr.StatusesURL,
		headRef:         *pr.Head.Ref,
		goSrcPath:       stripScheme(*pr.Base.Repo.HTMLURL),
		owner:           *pr.Base.Repo.Owner.Login,
		repo:            *pr.Base.Repo.Name,
		pr:              *e.Number,
		sha:             *pr.Head.SHA,
	}
}

// AnalyseConfig is a configuration struct for the Analyse method, all fields
// are required, unless otherwise stated.
type AnalyseConfig struct {
	cloner          analyser.Cloner
	refReader       analyser.RefReader
	installationID  int
	repositoryID    int
	statusesContext string
	statusesURL     string

	// if push (EventTypePush)
	commitFrom  string
	commitTo    string
	commitCount int

	// if pull request (EventTypePullRequest)
	pr int

	// for analyser.
	headRef   string // ref can be branch for pr or sha (after) for push.
	goSrcPath string

	// for issue comments.
	owner string
	repo  string
	sha   string
}

// Analyse analyses a GitHub event. If cfg.pr is not 0, comments will also be
// written on the Pull Request.
func (g *GitHub) Analyse(cfg AnalyseConfig) (err error) {
	logger := g.logger.With("installationID", cfg.installationID)
	logger = logger.With("owner", cfg.owner).With("repo", cfg.repo).With("ref", cfg.sha).With("pr", cfg.pr)
	logger.Info("analysing")

	// For functions that support context, set a maximum execution time.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	// Lookup installation
	install, err := g.NewInstallation(cfg.installationID)
	if err != nil {
		return errors.Wrap(err, "error getting installation")
	}
	if !install.IsEnabled() {
		return fmt.Errorf("could not find installation with ID %v", cfg.installationID)
	}

	// Find tools for this repo. StartAnalysis could return these tools instead
	// as part of the analysis type, which Analyser then fills out.
	tools, err := g.db.ListTools()
	if err != nil {
		return errors.Wrap(err, "could not get tools")
	}

	// Record start of analysis
	analysis, err := g.db.StartAnalysis(install.ID, cfg.repositoryID, cfg.commitFrom, cfg.commitTo, cfg.pr)
	if err != nil {
		return errors.Wrap(err, "error starting analysis")
	}
	logger = logger.With("analysisID", analysis.ID)
	logger.Info("created new analysis record")
	analysisURL := analysis.HTMLURL(g.gciBaseURL)

	// Set the CI status API to pending
	statusAPIReporter := NewStatusAPIReporter(logger, install.client, cfg.statusesURL, cfg.statusesContext, analysisURL)
	err = statusAPIReporter.SetStatus(ctx, StatusStatePending, "In progress")
	if err != nil {
		return err
	}

	// if Analyse returns an error, set status as internally failed, and if
	// we were panicking, catch it, set the error, and then panic again, the
	// stacktrack should be maintained
	defer func() {
		var r interface{}
		if r = recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}

		if err != nil {
			if serr := statusAPIReporter.SetStatus(ctx, StatusStateError, "Internal error"); serr != nil {
				logger.With("error", serr).Error("could not set status API to error")
			}

			if ferr := g.db.FinishAnalysis(analysis.ID, db.AnalysisStatusError, nil); ferr != nil {
				logger.With("error", ferr).Error("could not set analysis to error")
			}
		}

		if r != nil {
			panic(r) // panic maintaining the stacktrace
		}
	}()

	// Analyse
	acfg := analyser.Config{
		HeadRef: cfg.headRef,
	}

	configReader := &analyser.YAMLConfig{
		Tools: tools,
	}

	// Get a new executer/environment to execute in
	executer, err := g.analyser.NewExecuter(ctx, cfg.goSrcPath)
	if err != nil {
		return errors.Wrap(err, "analyser could create new executer")
	}
	defer func() {
		if err := executer.Stop(ctx); err != nil {
			logger.With("error", err).Error("could not stop executer")
		}
	}()

	// Wrap it with our DB as it wants to record the results.
	executer = g.db.ExecRecorder(analysis.ID, executer)

	err = analyser.Analyse(ctx, logger, executer, cfg.cloner, configReader, cfg.refReader, acfg, analysis)
	if err != nil {
		return errors.Wrap(err, "could not run analyser")
	}

	// Report the issues.
	var reporters []analyser.Reporter
	reporters = append(reporters, statusAPIReporter) // Status API.

	switch {
	case cfg.pr != 0:
		// Inline code comments on the PR.
		reporters = append(reporters, NewPRCommentReporter(install.client, cfg.owner, cfg.repo, cfg.pr, cfg.sha))
	case cfg.commitCount == 1:
		// Comment on the single commit the issues inline.
		reporters = append(reporters, NewInlineCommitCommentReporter(install.client, cfg.owner, cfg.repo, cfg.sha))
	case cfg.commitCount > 1:
		// Comment on the latest commit a summary of all commits.
		reporters = append(reporters, NewCommitCommentReporter(install.client, cfg.owner, cfg.repo, cfg.sha, cfg.commitCount, analysisURL))
	}

	for _, reporter := range reporters {
		err := reporter.Report(ctx, analysis.Issues())
		if err != nil {
			return errors.WithMessage(err, "error reporting issues")
		}
	}

	err = g.db.FinishAnalysis(analysis.ID, db.AnalysisStatusSuccess, analysis)
	if err != nil {
		return errors.Wrapf(err, "could not set analysis status for analysisID %v", analysis.ID)
	}

	return nil
}

// stripScheme removes the scheme/protocol and :// from a URL.
func stripScheme(url string) string {
	return regexp.MustCompile(`[a-zA-Z0-9+.-]+://`).ReplaceAllString(url, "")
}
