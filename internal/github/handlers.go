package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"os"

	"github.com/google/go-github/github"
	"github.com/pkg/errors"
)

func dumpRequest(r *http.Request) []byte {
	log.Println("-------")
	dump, err := httputil.DumpRequest(r, false)
	if err != nil {
		log.Println("could not dump request:", err)
	}
	log.Printf("%s", dump)

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Println(err)
	}

	var out bytes.Buffer
	json.Indent(&out, body, "", "  ")
	out.WriteTo(os.Stdout)

	return body
}

// CallBackHandler is the net/http handler for github callbacks.
func (g *GitHub) CallBackHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("callbackHandler")
	dumpRequest(r)
}

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

	fmt.Println(string(body))

	event, err := github.ParseWebHook(github.WebHookType(r), body)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	log.Printf("parsed webhook event: %T", event)

	switch e := event.(type) {
	case *github.IntegrationInstallationEvent:
		switch *e.Action {
		case "created":
			// Record the installation event in the database
			log.Printf("integration installation, installation id: %v, on account %v, by account %v",
				*e.Installation.ID, *e.Installation.Account.Login, *e.Sender.Login,
			)
			err := g.db.GHAddInstallation(*e.Installation.ID, *e.Installation.Account.ID)
			if err != nil {
				log.Println(errors.Wrap(err, "could not insert installation into database"))
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
		case "deleted":
			// Remove the installation event from the database
			log.Printf("integration removal, installation id: %v, on account %v, by account %v",
				*e.Installation.ID, *e.Installation.Account.Login, *e.Sender.Login,
			)
			err := g.db.GHRemoveInstallation(*e.Installation.ID, *e.Installation.Account.ID)
			if err != nil {
				log.Println(errors.Wrap(err, "could not delete installation from database"))
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
		default:
			log.Printf("ignoring integration installation event action: %q", *e.Action)
			break
		}
	case *github.PullRequestEvent:
		if e.Action == nil || *e.Action != "opened" {
			log.Printf("ignoring PR #%v action: %q", *e.Number, *e.Action)
			break
		}
		if e.Repo == nil || e.PullRequest == nil {
			log.Printf("malformed PR webhook, no repo or pullrequest set")
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		pr := e.PullRequest

		// Lookup installation
		install, err := g.NewInstallation(*e.Repo.Owner.ID)
		if err != nil {
			log.Println(errors.Wrap(err, "error getting installation"))
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		if install == nil {
			log.Println("could not find installation")
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}

		// Set the CI status API to pending
		err = install.SetStatus(*pr.StatusesURL, StatusStatePending)
		if err != nil {
			log.Printf("could not set status to pending for %v: %v", *pr.StatusesURL, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// TODO we want to background this and reply to http request
		issues, err := g.analyser.Analyse(*pr.Head.Repo.CloneURL, *pr.Head.Ref, *pr.DiffURL)
		if err != nil {
			log.Printf("could not analyse %v pr %v: %v", *e.Repo.URL, *e.Number, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Post as comments on github pr
		install.WriteIssues(*pr.Number, *pr.Head.SHA, issues)

		// Set the CI status API to success
		err = install.SetStatus(*pr.StatusesURL, StatusStateSuccess)
		if err != nil {
			log.Printf("could not set status to success for %v: %v", *pr.StatusesURL, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	default:
		log.Printf("ignoring unknown event: %T", event)
	}
}
