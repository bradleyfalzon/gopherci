package main

import (
	"context"
	"database/sql"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/bradleyfalzon/gopherci/internal/analyser"
	"github.com/bradleyfalzon/gopherci/internal/db"
	"github.com/bradleyfalzon/gopherci/internal/github"
	"github.com/bradleyfalzon/gopherci/internal/queue"
	_ "github.com/go-sql-driver/mysql"
	gh "github.com/google/go-github/github"
	"github.com/joho/godotenv"
	"github.com/pkg/errors"
	migrate "github.com/rubenv/sql-migrate"
)

func main() {
	// Load environment from .env, ignore errors as it's optional and dev only
	_ = godotenv.Load()

	srv := &http.Server{Addr: ":3000"} // http server, for graceful shutdown

	// Graceful shutdown handler
	ctx, cancelFunc := context.WithCancel(context.Background())
	go SignalHandler(cancelFunc, srv)

	switch {
	case os.Getenv("GITHUB_ID") == "":
		log.Fatalln("GITHUB_ID is not set")
	case os.Getenv("GITHUB_PEM_FILE") == "":
		log.Fatalln("GITHUB_PEM_FILE is not set")
	case os.Getenv("GITHUB_WEBHOOK_SECRET") == "":
		log.Fatalln("GITHUB_WEBHOOK_SECRET is not set")
	}

	// Database
	log.Printf("Connecting to %q db name: %q, username: %q, host: %q, port: %q",
		os.Getenv("DB_DRIVER"), os.Getenv("DB_DATABASE"), os.Getenv("DB_USERNAME"), os.Getenv("DB_HOST"), os.Getenv("DB_PORT"),
	)

	dsn := fmt.Sprintf(`%s:%s@tcp(%s:%s)/%s?charset=utf8&collation=utf8_unicode_ci&timeout=6s&time_zone='%%2B00:00'&parseTime=true`,
		os.Getenv("DB_USERNAME"), os.Getenv("DB_PASSWORD"), os.Getenv("DB_HOST"), os.Getenv("DB_PORT"), os.Getenv("DB_DATABASE"),
	)

	sqlDB, err := sql.Open(os.Getenv("DB_DRIVER"), dsn)
	if err != nil {
		log.Fatal("Error setting up DB:", err)
	}

	// Do DB migrations
	migrations := &migrate.FileMigrationSource{Dir: "migrations"}
	migrate.SetTable("migrations")
	direction := migrate.Up
	migrateMax := 0
	if len(os.Args) > 1 && os.Args[1] == "down" {
		direction = migrate.Down
		migrateMax = 1
	}
	n, err := migrate.ExecMax(sqlDB, os.Getenv("DB_DRIVER"), migrations, direction, migrateMax)
	log.Printf("Applied %d migrations to database", n)
	if err != nil {
		log.Fatal(errors.Wrap(err, "could not execute all migrations"))
	}

	db, err := db.NewSQLDB(sqlDB, os.Getenv("DB_DRIVER"))
	if err != nil {
		log.Fatalln("could not initialise db:", err)
	}

	// Analyser
	log.Printf("Using analyser %q", os.Getenv("ANALYSER"))
	var analyse analyser.Analyser
	switch os.Getenv("ANALYSER") {
	case "filesystem":
		if os.Getenv("ANALYSER_FILESYSTEM_PATH") == "" {
			log.Fatalln("ANALYSER_FILESYSTEM_PATH is not set")
		}
		analyse, err = analyser.NewFileSystem(os.Getenv("ANALYSER_FILESYSTEM_PATH"))
		if err != nil {
			log.Fatalln("could not initialise file system analyser:", err)
		}
	case "docker":
		image := os.Getenv("ANALYSER_DOCKER_IMAGE")
		if image == "" {
			image = analyser.DockerDefaultImage
		}
		analyse, err = analyser.NewDocker(image)
		if err != nil {
			log.Fatalln("could not initialise Docker analyser:", err)
		}
	case "":
		log.Fatalln("ANALYSER is not set")
	default:
		log.Fatalf("Unknown ANALYSER option %q", os.Getenv("ANALYSER"))
	}

	// Queuer
	var (
		wg        sync.WaitGroup           // wait for queue to finish before exiting
		queueChan = make(chan interface{}) // receive jobs on this chan
	)
	var q queue.Queuer
	switch os.Getenv("QUEUER") {
	case "memory":
		q = queue.NewMemoryQueue(ctx, &wg, queueChan)
	case "gcppubsub":
		switch {
		case os.Getenv("QUEUER_GCPPUBSUB_PROJECT_ID") == "":
			log.Fatalf("QUEUER_GCPPUBSUB_PROJECT_ID is not set")
		}
		q, err = queue.NewGCPPubSubQueue(ctx, &wg, queueChan, os.Getenv("QUEUER_GCPPUBSUB_PROJECT_ID"), os.Getenv("QUEUER_GCPPUBSUB_TOPIC"))
		if err != nil {
			log.Fatal("Could not initialise GCPPubSubQueue:", err)
		}
	case "":
		log.Fatalln("QUEUER is not set")
	default:
		log.Fatalf("Unknown QUEUER option %q", os.Getenv("QUEUER"))
	}

	// GitHub
	log.Printf("GitHub Integration ID: %q, GitHub Integration PEM File: %q", os.Getenv("GITHUB_ID"), os.Getenv("GITHUB_PEM_FILE"))
	integrationID, err := strconv.ParseInt(os.Getenv("GITHUB_ID"), 10, 64)
	if err != nil {
		log.Fatalf("could not parse integrationID %q", os.Getenv("GITHUB_ID"))
	}

	integrationKey, err := ioutil.ReadFile(os.Getenv("GITHUB_PEM_FILE"))
	if err != nil {
		log.Fatalf("could not read private key for GitHub integration: %s", err)
	}

	gh, err := github.New(analyse, db, q, int(integrationID), integrationKey, os.Getenv("GITHUB_WEBHOOK_SECRET"))
	if err != nil {
		log.Fatalln("could not initialise GitHub:", err)
	}
	http.HandleFunc("/gh/webhook", gh.WebHookHandler)
	http.HandleFunc("/gh/callback", gh.CallBackHandler)

	// Listen for jobs from the queue
	go queueListen(ctx, queueChan, gh)

	// Health checks
	http.HandleFunc("/health-check", HealthCheckHandler)

	// Listen
	log.Println("main: listening on", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Println("main: http server error:", err)
		cancelFunc()
	}

	// Wait for current item in queue to finish
	log.Println("main: waiting for queuer to finish")
	wg.Wait()
	log.Println("main: exiting gracefully")
}

// queueListen listens for jobs on the queue and executes the relevant handlers.
func queueListen(ctx context.Context, queueChan <-chan interface{}, g *github.GitHub) {
	for {
		select {
		case <-ctx.Done():
			log.Println("queueListen: returning from queueListen")
			return
		case job := <-queueChan:
			start := time.Now()
			log.Printf("queueListen: reading job type %T", job)
			var err error
			switch e := job.(type) {
			case *gh.PushEvent:
				err = g.Analyse(github.PushConfig(e))
				if err != nil {
					err = errors.Wrapf(err, "cannot analyse push event for sha %v on repo %v", *e.After, *e.Repo.HTMLURL)
				}
			case *gh.PullRequestEvent:
				err = g.Analyse(github.PullRequestConfig(e))
				if err != nil {
					err = errors.Wrapf(err, "cannot analyse pr %v", *e.PullRequest.HTMLURL)
				}
			default:
				err = fmt.Errorf("unknown queue job type %T", e)
			}
			if err != nil {
				log.Println("queueListen: processing error:", err)
			}
			log.Printf("queueListen: finished processing in %v", time.Since(start))
		}
	}
}
