package main

import (
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"
)

// shuttingDown tracks whether this instance of GopherCI is shutting down
// it's written to by the SignalHandler and read by HealthCheckHandler
var shuttingDown bool

// SignalHandler listens for a shutdown signal and calls cancel, if
// multiple signals are received in short succession, forcible quit.
func SignalHandler(cancel context.CancelFunc, srv *http.Server) {
	// chan size 2 as multiple interrupts is force quit (supports ^C for dev)
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt)

	var lastSignal time.Time
	for {
		s := <-c
		if time.Since(lastSignal) < time.Second {
			log.Fatal("Two signals in short succession, forcing quit")
		}

		lastSignal = time.Now()
		log.Printf("Received %v, preparing to shutdown", s)
		shuttingDown = true
		srv.Shutdown(context.Background())
		cancel()
	}
}

// HealthCheckHandler checks whether the instance is shutting down, and if so,
// responds with 503 Service Unavailable.
func HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	if shuttingDown {
		http.Error(w, "Service shutting down", http.StatusServiceUnavailable)
	}
	io.WriteString(w, "Service OK")
}
