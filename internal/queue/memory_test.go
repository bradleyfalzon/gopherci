package queue

import (
	"context"
	"log"
	"sync"
	"testing"
	"time"
)

func TestMemoryQueue(t *testing.T) {
	var (
		ctx, cancel = context.WithCancel(context.Background())
		wg          sync.WaitGroup
		c           = make(chan interface{})
		haveJob     bool
	)
	q := NewMemoryQueue()

	f := func(interface{}) {
		haveJob = true
	}

	q.Wait(ctx, &wg, c, f)
	c <- 1

	log.Println("waiting")
	time.Sleep(pollInterval * 2)
	log.Println("waited")

	if !haveJob {
		t.Errorf("did not process job")
	}
	cancel()
}
