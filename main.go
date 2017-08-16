package main

import (
	"context"
	"database/sql"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/bradleyfalzon/gopherci/internal/analyser"
	"github.com/bradleyfalzon/gopherci/internal/db"
	"github.com/bradleyfalzon/gopherci/internal/github"
	"github.com/bradleyfalzon/gopherci/internal/logger"
	"github.com/bradleyfalzon/gopherci/internal/queue"
	"github.com/bradleyfalzon/gopherci/internal/web"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	_ "github.com/go-sql-driver/mysql"
	gh "github.com/google/go-github/github"
	"github.com/joho/godotenv"
	"github.com/pkg/errors"
	migrate "github.com/rubenv/sql-migrate"
)

// build tracks the build version of the binary.
var build string

func main() {
	// Load environment from .env, ignore errors as it's optional and dev only
	_ = godotenv.Load()

	rootLogger := logger.New(os.Stdout, "nobuild", os.Getenv("LOGGER_ENV"), os.Getenv("LOGGER_SENTRY_DSN"))
	logger := rootLogger.With("area", "main")
	logger.With("build", build).Info("starting gopherci")

	r := chi.NewRouter()
	r.Use(middleware.RealIP) // Blindly accept XFF header, ensure LB overwrites it
	r.Use(middleware.DefaultCompress)
	r.Use(middleware.Recoverer)
	r.Use(middleware.NoCache)

	// http server for graceful shutdown
	srv := &http.Server{
		Addr:    ":3000",
		Handler: r,
	}

	// Graceful shutdown handler
	ctx, cancel := context.WithCancel(context.Background())
	go SignalHandler(rootLogger.With("area", "signalHandler"), cancel, srv)

	switch {
	case os.Getenv("GCI_BASE_URL") == "":
		logger.Info("GCI_BASE_URL is blank, URLs linking back to GopherCI will not work")
	case os.Getenv("GITHUB_ID") == "":
		logger.Error("GITHUB_ID is not set")
	case os.Getenv("GITHUB_PEM_FILE") == "":
		logger.Fatal("GITHUB_PEM_FILE is not set")
	case os.Getenv("GITHUB_WEBHOOK_SECRET") == "":
		logger.Fatal("GITHUB_WEBHOOK_SECRET is not set")
	}

	// Database
	logger.Infof("connecting to %q db name: %q, username: %q, host: %q, port: %q",
		os.Getenv("DB_DRIVER"), os.Getenv("DB_DATABASE"), os.Getenv("DB_USERNAME"), os.Getenv("DB_HOST"), os.Getenv("DB_PORT"),
	)

	dsn := fmt.Sprintf(`%s:%s@tcp(%s:%s)/%s?charset=utf8&collation=utf8_unicode_ci&timeout=6s&time_zone='%%2B00:00'&parseTime=true`,
		os.Getenv("DB_USERNAME"), os.Getenv("DB_PASSWORD"), os.Getenv("DB_HOST"), os.Getenv("DB_PORT"), os.Getenv("DB_DATABASE"),
	)

	sqlDB, err := sql.Open(os.Getenv("DB_DRIVER"), dsn)
	if err != nil {
		logger.With("error", err).Fatal("could not open database")
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
	logger.Infof("applied %d migrations to database", n)
	if err != nil {
		logger.With("error", err).Fatal("could not execute all migrations")
	}

	db, err := db.NewSQLDB(sqlDB, os.Getenv("DB_DRIVER"))
	if err != nil {
		logger.With("error", err).Fatal("could not initialise database")
	}
	go db.Cleanup(ctx, rootLogger.With("area", "db"))

	var analyserMemoryLimit int64
	if os.Getenv("ANALYSER_MEMORY_LIMIT") != "" {
		analyserMemoryLimit, err = strconv.ParseInt(os.Getenv("ANALYSER_MEMORY_LIMIT"), 10, 32)
		if err != nil {
			logger.With("error", err).Fatal("could not parse ANALYSER_MEMORY_LIMIT")
		}
	}

	// Analyser
	logger.Infof("using analyser %q", os.Getenv("ANALYSER"))
	var analyse analyser.Analyser
	switch os.Getenv("ANALYSER") {
	case "filesystem":
		if os.Getenv("ANALYSER_FILESYSTEM_PATH") == "" {
			logger.Fatal("ANALYSER_FILESYSTEM_PATH is not set")
		}
		analyse, err = analyser.NewFileSystem(os.Getenv("ANALYSER_FILESYSTEM_PATH"), int(analyserMemoryLimit))
		if err != nil {
			logger.Fatal("could not initialise file system analyser:", err)
		}
	case "docker":
		image := os.Getenv("ANALYSER_DOCKER_IMAGE")
		if image == "" {
			image = analyser.DockerDefaultImage
		}
		analyse, err = analyser.NewDocker(rootLogger.With("area", "docker"), image, int(analyserMemoryLimit))
		if err != nil {
			logger.Fatal("could not initialise Docker analyser:", err)
		}
	case "":
		logger.Fatal("ANALYSER is not set")
	default:
		logger.Fatalf("Unknown ANALYSER option %q", os.Getenv("ANALYSER"))
	}

	// GitHub
	logger.Infof("github Integration ID: %q, GitHub Integration PEM File: %q", os.Getenv("GITHUB_ID"), os.Getenv("GITHUB_PEM_FILE"))
	integrationID, err := strconv.ParseInt(os.Getenv("GITHUB_ID"), 10, 64)
	if err != nil {
		logger.Fatalf("could not parse integrationID %q", os.Getenv("GITHUB_ID"))
	}

	integrationKey, err := ioutil.ReadFile(os.Getenv("GITHUB_PEM_FILE"))
	if err != nil {
		logger.Fatalf("could not read private key for GitHub integration: %s", err)
	}

	// queuePush is used to add a job to the queue
	var queuePush = make(chan interface{})

	gh, err := github.New(rootLogger, analyse, db, queuePush, int(integrationID), integrationKey, os.Getenv("GITHUB_WEBHOOK_SECRET"), os.Getenv("GCI_BASE_URL"))
	if err != nil {
		logger.Fatal("could not initialise GitHub:", err)
	}
	r.Post("/gh/webhook", gh.WebHookHandler)
	r.Get("/gh/callback", gh.CallbackHandler)

	var (
		wg         sync.WaitGroup // wait for queue to finish before exiting
		qProcessor = queueProcessor{github: gh, logger: rootLogger.With("area", "queueProcessor")}
	)

	switch os.Getenv("QUEUER") {
	case "memory":
		memq := queue.NewMemoryQueue(rootLogger.With("area", "memoryQueue"))
		memq.Wait(ctx, &wg, queuePush, qProcessor.Process)
	case "gcppubsub":
		switch {
		case os.Getenv("QUEUER_GCPPUBSUB_PROJECT_ID") == "":
			logger.Fatalf("QUEUER_GCPPUBSUB_PROJECT_ID is not set")
		}
		gcp, err := queue.NewGCPPubSubQueue(ctx, rootLogger.With("area", "gcpPubSubQueue"), os.Getenv("QUEUER_GCPPUBSUB_PROJECT_ID"), os.Getenv("QUEUER_GCPPUBSUB_TOPIC"))
		if err != nil {
			logger.Fatal("Could not initialise GCPPubSubQueue:", err)
		}
		gcp.Wait(ctx, &wg, queuePush, qProcessor.Process)
	case "":
		logger.Fatal("QUEUER is not set")
	default:
		logger.Fatalf("Unknown QUEUER option %q", os.Getenv("QUEUER"))
	}

	// Web routes
	web, err := web.NewWeb(rootLogger.With("area", "web"), db, gh)
	if err != nil {
		logger.With("error", err).Fatal("could not instantiate web")
	}
	workDir, _ := os.Getwd()
	FileServer(r, "/static", http.Dir(filepath.Join(workDir, "internal", "web", "static")))

	r.NotFound(web.NotFoundHandler)
	r.Get("/analysis/{analysisID}", web.AnalysisHandler)

	// Health checks
	r.Get("/health-check", HealthCheckHandler)

	// Listen
	logger.Infof("listening on %s", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.With("error", err).Error("http server error")
		cancel()
	}

	// Wait for current item in queue to finish
	logger.Info("waiting for queuer to finish")
	wg.Wait()
	logger.Info("exiting gracefully")
}

// FileServer conveniently sets up a http.FileServer handler to serve
// static files from a http.FileSystem.
// https://github.com/go-chi/chi/blob/524a020446146841512dd1639e736422e7af53a4/_examples/fileserver/main.go
func FileServer(r chi.Router, path string, root http.FileSystem) {
	fs := http.StripPrefix(path, http.FileServer(root))

	if path != "/" && path[len(path)-1] != '/' {
		r.Get(path, http.RedirectHandler(path+"/", 301).ServeHTTP)
		path += "/"
	}
	path += "*"

	r.Get(path, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fs.ServeHTTP(w, r)
	}))
}

// Queue processor is the callback called by queuer when receiving a job
type queueProcessor struct {
	github *github.GitHub
	logger logger.Logger
}

// queueListen listens for jobs on the queue and executes the relevant handlers.
func (q *queueProcessor) Process(job interface{}) {
	start := time.Now()
	q.logger.Infof("processing job type %T", job)
	var err error
	switch e := job.(type) {
	case *gh.PushEvent:
		err = q.github.Analyse(github.PushConfig(e))
		if err != nil {
			err = errors.Wrapf(err, "cannot analyse push event for sha %v on repo %v", *e.After, *e.Repo.HTMLURL)
		}
	case *gh.PullRequestEvent:
		err = q.github.Analyse(github.PullRequestConfig(e))
		if err != nil {
			err = errors.Wrapf(err, "cannot analyse pr %v", *e.PullRequest.HTMLURL)
		}
	default:
		err = fmt.Errorf("unknown queue job type %T", e)
	}
	q.logger.Infof("finished processing in %v", time.Since(start))
	if err != nil {
		q.logger.With("error", err).Error("processing error")
	}
}
