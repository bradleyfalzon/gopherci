package queue

import (
	"context"
	"sync"
	"testing"

	"github.com/fortytw2/leaktest"
)

func TestMemoryQueue(t *testing.T) {
	defer leaktest.Check(t)() // ensure all goroutines exit

	var (
		ctx, cancelFunc = context.WithCancel(context.Background())
		wg              sync.WaitGroup
		c               = make(chan interface{})
	)
	q := NewMemoryQueue(ctx, &wg, c)

	job := 1
	q.Queue(job)

	if have := <-c; have != job {
		t.Errorf("have: %#v, want: %#v", have, job)
	}
	cancelFunc()
}
