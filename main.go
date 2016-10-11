package main

import (
	"log"
	"net/http"
	"os"

	"github.com/bradleyfalzon/gopherci/internal/github"
)

func main() {
	switch {
	case os.Getenv("GITHUB_ID") == "":
		log.Fatal("GITHUB_ID is not set")
	case os.Getenv("GITHUB_PEM_FILE") == "":
		log.Fatal("GITHUB_PEM_FILE is not set")
	}

	log.Printf("GitHub ID: %q, GitHub PEM File: %q", os.Getenv("GITHUB_ID"), os.Getenv("GITHUB_PEM_FILE"))
	gh, err := github.New(os.Getenv("GITHUB_ID"), os.Getenv("GITHUB_PEM_FILE"))
	if err != nil {
		log.Fatalln("could not initialise GitHub:", err)
	}
	http.HandleFunc("/gh/webhook", gh.WebHookHandler)
	http.HandleFunc("/gh/callback", gh.CallBackHandler)

	log.Println("Listening on :3000")
	log.Fatal(http.ListenAndServe(":3000", nil))
}
