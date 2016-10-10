package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"os"
)

type appHandler func(http.ResponseWriter, *http.Request) error

func (fn appHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Println("-------")

	dump, err := httputil.DumpRequest(r, false)
	if err != nil {
		log.Println("could not dump request:", err)
	}
	log.Printf("%s", dump)

	if err := fn(w, r); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func homeHandler(w http.ResponseWriter, r *http.Request) error {
	fmt.Fprint(w, "homepageHandler")
	return nil
}

func callbackHandler(w http.ResponseWriter, r *http.Request) error {
	log.Println("callbackHandler")

	return nil
}

func webhookHandler(w http.ResponseWriter, r *http.Request) error {
	log.Println("webhookHandler")

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}

	var out bytes.Buffer
	json.Indent(&out, body, "=", "\t")
	out.WriteTo(os.Stdout)

	return nil
}
