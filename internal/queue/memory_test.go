package queue

import (
	"context"
	"testing"

	"github.com/fortytw2/leaktest"
)

func TestMemoryQueue(t *testing.T) {
	defer leaktest.Check(t)() // ensure all goroutines exit

	ctx, cancelFunc := context.WithCancel(context.Background())
	c := make(chan interface{})
	q := NewMemoryQueue(ctx, c)

	job := 1
	q.Queue(job)

	if have := <-c; have != job {
		t.Errorf("have: %#v, want: %#v", have, job)
	}
	cancelFunc()
}
