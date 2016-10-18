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

	"github.com/bradleyfalzon/go-github/github"
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
		log.Printf("%v pr %v", *e.Action, *e.Number)
		log.Printf("Diff url: %v", *e.PullRequest.DiffURL)
		log.Printf("Ref (branch): %v", *e.PullRequest.Head.Ref)
		log.Printf("Clone url: %v", *e.PullRequest.Head.Repo.CloneURL)

		// TODO we want to background this and reply to http request
		pr := e.PullRequest
		issues, err := g.analyser.Analyse(*pr.Head.Repo.CloneURL, *pr.Head.Ref, *pr.DiffURL)
		if err != nil {
			log.Printf("could not analyse %v pr %v: %v", *e.Repo.URL, *e.Number, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		log.Printf("Comment url: %v", *e.PullRequest.ReviewCommentURL)
		log.Printf("Comments url: %v", *e.PullRequest.ReviewCommentsURL)

		// Post as comments on github pr
		g.WriteIssues(*e.PullRequest.ReviewCommentsURL, issues)

	default:
		log.Printf("ignoring unknown event: %T", event)
	}
}
