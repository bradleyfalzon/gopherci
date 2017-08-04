package github

import (
	"context"
	"fmt"
	"log"
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
	payload, err := github.ValidatePayload(r, g.webhookSecret)
	if err != nil {
		log.Println("github: failed to validate payload:", err)
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	event, err := github.ParseWebHook(github.WebHookType(r), payload)
	if err != nil {
		log.Println("github: failed to parse webhook:", err)
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	switch e := event.(type) {
	case *github.InstallationEvent:
		log.Printf("github: integration event: %v, installation id: %v", *e.Action, *e.Installation.ID)
		err = g.integrationInstallationEvent(e)
	case *github.PushEvent:
		var installation *Installation
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
		log.Printf("github: push event: installation id: %v", *e.Installation.ID)
		g.queuePush <- e
	case *github.PullRequestEvent:
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

		ok, err = checkPRAffectsGo(r.Context(), installation, *e.Repo.Owner.Login, *e.Repo.Name, *e.Number)
		if err != nil {
			break
		}
		if !ok {
			err = &ignoreEvent{reason: ignoreNoGoFiles}
			break
		}
		log.Printf("github: pull request event: %v, installation id: %v", *e.Action, *e.Installation.ID)
		g.queuePush <- e
	default:
		err = &ignoreEvent{reason: ignoreUnknownEvent}
	}

	switch err.(type) {
	case nil:
	case *ignoreEvent:
		log.Printf("github: ignoring event %T: %v", event, err)
	default:
		log.Println("github: event handler error:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

type ignoreReason int

const (
	ignoreUnknownEvent ignoreReason = iota
	ignoreInvalidAction
	ignoreNoAction
	ignoreNoInstallation
	ignoreNoGoFiles
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
		headRef:         *e.After,
		goSrcPath:       stripScheme(*e.Repo.HTMLURL),
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
	commitFrom string
	commitTo   string

	// if pull request (EventTypePullRequest)
	pr int

	// for analyser.
	headRef   string // ref can be branch for pr or sha (after) for push.
	goSrcPath string

	// for issue comments.
	owner string // required if eventType is EventTypePullRequest.
	repo  string // required if eventType is EventTypePullRequest.
	sha   string // required if eventType is EventTypePullRequest.
}

// Analyse analyses a GitHub event. If cfg.pr is not 0, comments will also be
// written on the Pull Request.
func (g *GitHub) Analyse(cfg AnalyseConfig) (err error) {
	log.Printf("analysing config: %#v", cfg)

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
	log.Println("analysisID:", analysis.ID)
	analysisURL := analysis.HTMLURL(g.gciBaseURL)

	// Set the CI status API to pending
	err = install.SetStatus(ctx, cfg.statusesContext, cfg.statusesURL, StatusStatePending, "In progress", analysisURL)
	if err != nil {
		return errors.Wrapf(err, "could not set status to pending for %v", cfg.statusesURL)
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
			if serr := install.SetStatus(ctx, cfg.statusesContext, cfg.statusesURL, StatusStateError, "Internal error", analysisURL); serr != nil {
				log.Printf("could not set status to error for %v: %s", cfg.statusesURL, serr)
			}

			if ferr := g.db.FinishAnalysis(analysis.ID, db.AnalysisStatusError, nil); ferr != nil {
				log.Printf("could not set analysis to error for analysisID %v: %s", analysis.ID, ferr)
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
			log.Printf("warning: could not stop executer: %v", err)
		}
	}()

	err = analyser.Analyse(ctx, executer, cfg.cloner, configReader, cfg.refReader, acfg, analysis)
	if err != nil {
		return errors.Wrap(err, "could not run analyser")
	}

	// if this is a PR add comments, suppressed is the number of comments that
	// would have been submitted if it wasn't for an internal fixed limit. For
	// pushes, there are no comments, so suppressed is 0.
	var suppressed = 0
	if cfg.pr != 0 {
		var issues []db.Issue
		suppressed, issues, err = install.FilterIssues(ctx, cfg.owner, cfg.repo, cfg.pr, analysis.Issues())
		if err != nil {
			return err
		}

		err = install.WriteIssues(ctx, cfg.owner, cfg.repo, cfg.pr, cfg.sha, issues)
		if err != nil {
			return err
		}
		log.Printf("wrote %v issues as comments, suppressed %v", len(issues)-suppressed, suppressed)
	}

	// Set the CI status API to success
	statusDesc := statusDesc(analysis.Issues(), suppressed)
	if err := install.SetStatus(ctx, cfg.statusesContext, cfg.statusesURL, StatusStateSuccess, statusDesc, analysisURL); err != nil {
		return errors.Wrapf(err, "could not set status to success for %v", cfg.statusesURL)
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

// statusDesc builds a status description based on issues.
func statusDesc(issues []db.Issue, suppressed int) string {
	desc := fmt.Sprintf("Found %d issues", len(issues))
	switch {
	case len(issues) == 0:
		return `Found no issues \ʕ◔ϖ◔ʔ/`
	case len(issues) == 1:
		return `Found 1 issue`
	case suppressed == 1:
		desc += fmt.Sprintf(" (%v comment suppressed)", suppressed)
	case suppressed > 1:
		desc += fmt.Sprintf(" (%v comments suppressed)", suppressed)
	}
	return desc
}
