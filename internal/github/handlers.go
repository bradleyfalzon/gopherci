package github

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"os"
)

func dumpRequest(r *http.Request) []byte {
	log.Println("-------")
	dump, err := httputil.DumpRequest(r, false)
	if err != nil {
		log.Println("could not dump request:", err)
	}
	log.Print(dump)

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Println(err)
	}

	var out bytes.Buffer
	json.Indent(&out, body, "", "  ")
	out.WriteTo(os.Stdout)

	return body
}

func (g *GitHub) CallBackHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("callbackHandler")
	dumpRequest(r)
}

func (g *GitHub) WebHookHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("webhookHandler")
	dumpRequest(r)
}
