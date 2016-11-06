package github

import (
	"fmt"
	"io/ioutil"
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
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// TODO
	//payload, err := github.ValidatePayload(r, g.webhookSecretKey)
	//if err != nil {
	//http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
	//return
	//}

	event, err := github.ParseWebHook(github.WebHookType(r), body)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	log.Printf("parsed webhook event: %T", event)

	// process hook in background
	go func() {
		var err error
		switch e := event.(type) {
		case *github.IntegrationInstallationEvent:
			err = g.integrationInstallationEvent(e)
		case *github.PullRequestEvent:
			err = g.pullRequestEvent(e)
		}
		if err != nil {
			log.Println(err)
		}
	}()
}

func (g *GitHub) integrationInstallationEvent(e *github.IntegrationInstallationEvent) error {
	log.Printf("integration event: %v, installation id: %v, on account %v, by account %v",
		*e.Action, *e.Installation.ID, *e.Installation.Account.Login, *e.Sender.Login,
	)
	var err error
	switch *e.Action {
	case "created":
		// Record the installation event in the database
		err = g.db.AddGHInstallation(*e.Installation.ID, *e.Installation.Account.ID)
	case "deleted":
		// Remove the installation event from the database
		err = g.db.RemoveGHInstallation(*e.Installation.Account.ID)
	}
	if err != nil {
		return errors.Wrap(err, "database error handling integration installation event")
	}
	return nil
}

func (g *GitHub) pullRequestEvent(e *github.PullRequestEvent) error {
	if e.Action == nil || *e.Action != "opened" {
		return fmt.Errorf("ignoring PR #%v action: %q", *e.Number, *e.Action)
	}
	if e.Repo == nil || e.PullRequest == nil {
		return fmt.Errorf("malformed PR webhook, no repo or pullrequest set")
	}
	pr := e.PullRequest

	// Lookup installation
	install, err := g.NewInstallation(*e.Repo.Owner.ID)
	if err != nil {
		return errors.Wrap(err, "error getting installation")
	}
	if install == nil {
		return errors.New(fmt.Sprintf("could not find installation for accountID %v", *e.Repo.Owner.ID))
	}

	// Find tools for this repo
	tools, err := g.db.ListTools()
	if err != nil {
		return errors.Wrap(err, "could not get tools")
	}

	// Set the CI status API to pending
	err = install.SetStatus(*pr.StatusesURL, StatusStatePending)
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
		if err := install.SetStatus(*pr.StatusesURL, StatusStateError); err != nil {
			log.Printf("could not set status to error for %v", *pr.StatusesURL)
		}
		return errors.Wrap(err, fmt.Sprintf("could not analyse %v pr %v", *e.Repo.URL, *e.Number))
	}

	// Post issues as comments on github pr
	install.WriteIssues(*e.Number, *pr.Head.SHA, issues)

	// Set the CI status API to success
	if err := install.SetStatus(*pr.StatusesURL, StatusStateSuccess); err != nil {
		return errors.Wrap(err, fmt.Sprintf("could not set status to success for %v", *pr.StatusesURL))
	}
	return nil
}
