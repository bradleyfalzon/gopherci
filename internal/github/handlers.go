package github

import (
	"io/ioutil"
	"log"
	"net/http"

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

	switch e := event.(type) {
	case *github.IntegrationInstallationEvent:
		go g.integrationInstallationEvent(e)
	case *github.PullRequestEvent:
		go g.pullRequestEvent(e)
	}
}

func (g *GitHub) integrationInstallationEvent(e *github.IntegrationInstallationEvent) {
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
		log.Println(errors.Wrap(err, "database error handling integration installation event"))
	}
}

func (g *GitHub) pullRequestEvent(e *github.PullRequestEvent) {
	if e.Action == nil || *e.Action != "opened" {
		log.Printf("ignoring PR #%v action: %q", *e.Number, *e.Action)
		return
	}
	if e.Repo == nil || e.PullRequest == nil {
		log.Printf("malformed PR webhook, no repo or pullrequest set")
		return
	}
	pr := e.PullRequest

	// Lookup installation
	install, err := g.NewInstallation(*e.Repo.Owner.ID)
	if err != nil {
		log.Println(errors.Wrap(err, "error getting installation"))
		return
	}
	if install == nil {
		log.Println("could not find installation")
		return
	}

	// Set the CI status API to pending
	err = install.SetStatus(*pr.StatusesURL, StatusStatePending)
	if err != nil {
		log.Printf("could not set status to pending for %v: %v", *pr.StatusesURL, err)
		return
	}

	// Analyse
	issues, err := g.analyser.Analyse(*pr.Head.Repo.CloneURL, *pr.Head.Ref, *pr.DiffURL)
	if err != nil {
		log.Printf("could not analyse %v pr %v: %v", *e.Repo.URL, *e.Number, err)
		return
	}

	// Post issues as comments on github pr
	install.WriteIssues(*pr.Number, *pr.Head.SHA, issues)

	// Set the CI status API to success
	err = install.SetStatus(*pr.StatusesURL, StatusStateSuccess)
	if err != nil {
		log.Printf("could not set status to success for %v: %v", *pr.StatusesURL, err)
		return
	}
}
