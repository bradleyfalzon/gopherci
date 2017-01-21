package github

import (
	"fmt"
	"log"
	"net/http"

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

	log.Printf("github: parsed webhook event: %T", event)

	log.Printf("event: %#v", event)

	switch e := event.(type) {
	case *github.IntegrationInstallationEvent:
		err = g.integrationInstallationEvent(e)
	case *github.PullRequestEvent:
		log.Printf("prevent: %#v", e.Installation)
		err = g.queuer.Queue(e)
	}
	if err != nil {
		log.Println("github: event handler error:", err)
	}
}

func (g *GitHub) integrationInstallationEvent(e *github.IntegrationInstallationEvent) error {
	log.Printf("integration event: %v, installation id: %v, on account %v, by account %v",
		*e.Action, *e.Installation.ID, *e.Installation.Account.Login, *e.Sender.Login,
	)
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
	if e.Action == nil || *e.Action != "opened" {
		return fmt.Errorf("ignoring PR #%v action: %q", *e.Number, *e.Action)
	}

	// Lookup installation
	install, err := g.NewInstallation(*e.Installation.ID)
	if err != nil {
		return errors.Wrap(err, "error getting installation")
	}
	if install == nil {
		return fmt.Errorf("could not find installation with ID %v", *e.Installation.ID)
	}

	if e.Repo == nil || e.PullRequest == nil {
		return fmt.Errorf("malformed PR webhook, no repo or pullrequest set")
	}
	pr := e.PullRequest

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
	}

	issues, err := analyser.Analyse(g.analyser, tools, config)
	if err != nil {
		if err = install.SetStatus(*pr.StatusesURL, StatusStateError, "Internal error"); err != nil {
			log.Printf("could not set status to error for %v", *pr.StatusesURL)
		}
		return errors.Wrap(err, fmt.Sprintf("could not analyse %v pr %v", *e.Repo.URL, *e.Number))
	}

	// Post issues as comments on github pr
	err = install.WriteIssues(*pr.Base.Repo.Owner.Login, *pr.Base.Repo.Name, *e.Number, *pr.Head.SHA, issues)
	if err != nil {
		return errors.Wrapf(err, "could not write comment on %v", *pr.HTMLURL)
	}

	statusDesc := fmt.Sprintf("Found %v issues", len(issues))
	if len(issues) == 0 {
		statusDesc += ` \ʕ◔ϖ◔ʔ/`
	}

	// Set the CI status API to success
	if err := install.SetStatus(*pr.StatusesURL, StatusStateSuccess, statusDesc); err != nil {
		return errors.Wrap(err, fmt.Sprintf("could not set status to success for %v", *pr.StatusesURL))
	}
	return nil
}
