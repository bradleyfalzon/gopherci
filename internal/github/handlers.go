package github

import (
	"fmt"
	"log"
	"net/http"
	"regexp"

	"github.com/bradleyfalzon/gopherci/internal/analyser"
	"github.com/google/go-github/github"
	"github.com/pkg/errors"
)

// CallBackHandler is the net/http handler for github callbacks.
func (g *GitHub) CallBackHandler(w http.ResponseWriter, r *http.Request) {}

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
	case *github.IntegrationInstallationEvent:
		log.Printf("github: integration event: %v, installation id: %v", *e.Action, *e.Installation.ID)
		err = g.integrationInstallationEvent(e)
	case *github.PullRequestEvent:
		log.Printf("github: pull request event: %v, installation id: %v", *e.Action, *e.Installation.ID)
		err = g.queuer.Queue(e)
	default:
		log.Printf("github: ignored webhook event: %T", event)
	}
	if err != nil {
		log.Println("github: event handler error:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (g *GitHub) integrationInstallationEvent(e *github.IntegrationInstallationEvent) error {
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

// PullRequestEvent processes as Pull Request from GitHub.
func (g *GitHub) PullRequestEvent(e *github.PullRequestEvent) error {
	log.Printf("pr action %q on repo %v", *e.Action, *e.PullRequest.HTMLURL)
	if !validPRAction(*e.Action) {
		log.Printf("ignoring action %q", *e.Action)
		return nil
	}
	pr := e.PullRequest

	// Lookup installation
	install, err := g.NewInstallation(*e.Installation.ID)
	if err != nil {
		return errors.Wrap(err, "error getting installation")
	}
	if install == nil {
		return fmt.Errorf("could not find installation with ID %v", *e.Installation.ID)
	}

	// Find tools for this repo
	tools, err := g.db.ListTools()
	if err != nil {
		return errors.Wrap(err, "could not get tools")
	}

	// Set the CI status API to pending
	err = install.SetStatus(*pr.StatusesURL, StatusStatePending, "In progress")
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("could not set status to pending for %v", *pr.StatusesURL))
	}

	// Analyse
	config := analyser.Config{
		BaseURL:    *pr.Base.Repo.CloneURL,
		BaseBranch: *pr.Base.Ref,
		HeadURL:    *pr.Head.Repo.CloneURL,
		HeadBranch: *pr.Head.Ref,
		DiffURL:    *pr.DiffURL,
		GoSrcPath:  stripScheme(*pr.Base.Repo.HTMLURL),
	}

	issues, err := analyser.Analyse(g.analyser, tools, config)
	if err != nil {
		if err := install.SetStatus(*pr.StatusesURL, StatusStateError, "Internal error"); err != nil {
			log.Printf("could not set status to error for %v", *pr.StatusesURL)
		}
		return errors.Wrap(err, fmt.Sprintf("could not analyse %v pr %v", *e.Repo.URL, *e.Number))
	}

	// Post issues as comments on github pr
	suppressed, err := install.WriteIssues(*pr.Base.Repo.Owner.Login, *pr.Base.Repo.Name, *e.Number, *pr.Head.SHA, issues)
	if err != nil {
		return errors.Wrapf(err, "could not write comment on %v", *pr.HTMLURL)
	}

	statusDesc := statusDesc(issues, suppressed)

	// Set the CI status API to success
	if err := install.SetStatus(*pr.StatusesURL, StatusStateSuccess, statusDesc); err != nil {
		return errors.Wrap(err, fmt.Sprintf("could not set status to success for %v", *pr.StatusesURL))
	}
	return nil
}

// validPRAction return true if a pull request action is valid and should not
// be ignored.
func validPRAction(action string) bool {
	return action == "opened" || action == "synchronize" || action == "reopened"
}

// stripScheme removes the scheme/protocol and :// from a URL.
func stripScheme(url string) string {
	return regexp.MustCompile(`[a-zA-Z0-9+.-]+://`).ReplaceAllString(url, "")
}

// statusDesc builds a status description based on issues.
func statusDesc(issues []analyser.Issue, suppressed int) string {
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
