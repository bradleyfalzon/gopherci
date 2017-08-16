package queue

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/bradleyfalzon/gopherci/internal/logger"
)

func TestMemoryQueue(t *testing.T) {
	var (
		ctx, cancel = context.WithCancel(context.Background())
		wg          sync.WaitGroup
		c           = make(chan interface{})
		haveJob     bool
	)
	q := NewMemoryQueue(logger.Testing())

	f := func(interface{}) {
		haveJob = true
	}

	q.Wait(ctx, &wg, c, f)
	c <- 1

	t.Log("waiting")
	time.Sleep(pollInterval * 2)
	t.Log("waited")

	if !haveJob {
		t.Errorf("did not process job")
	}
	cancel()
}
