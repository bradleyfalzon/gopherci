package main

import (
	"fmt"
	"html"
	"log"
	"net/http"
)

func main() {
	http.Handle("/", appHandler(homeHandler))
	http.Handle("/callback", appHandler(callbackHandler))
	http.Handle("/webhook", appHandler(webhookHandler))

	http.HandleFunc("/bar", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Hello, %q", html.EscapeString(r.URL.Path))
	})

	fmt.Println("Listening on :3000")
	log.Fatal(http.ListenAndServe(":3000", nil))
}
