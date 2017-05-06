//+build integration_gcppubsub

package queue

import (
	"context"
	"encoding/gob"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/pkg/errors"
)

// TODO read from .env
const projectID = "gopherci-dev"

func TestGCPPubSubQueue(t *testing.T) {
	// it appears some other routine maybe leaked
	// by the http client
	//defer leaktest.Check(t)() // ensure all goroutines exit

	var (
		ctx, cancel = context.WithCancel(context.Background())
		wg          sync.WaitGroup
		c           = make(chan interface{})
		topic       = fmt.Sprintf("%s-unit-tests-%v", defaultTopicName, time.Now().Unix())
		have        interface{}
	)
	q, err := NewGCPPubSubQueue(ctx, projectID, topic)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	f := func(job interface{}) {
		have = job
	}

	q.Wait(ctx, &wg, c, f)

	type S struct{ Job string }
	gob.Register(&S{})
	job := S{"unit-test-" + topic}
	c <- job

	for i := 0; i < 10; i++ {
		time.Sleep(1 * time.Second)
		if have == nil {
			continue
		}

		concrete, ok := have.(*S)
		if !ok {
			t.Fatalf("have type: %T is not %T", have, &S{})
		}

		if !reflect.DeepEqual(*concrete, job) {
			t.Errorf("have (concrete): %#v, want: %#v", *concrete, job)
		}
	}

	if have == nil {
		t.Error("did not receive job from queue")
	}

	q.delete(ctx)
	cancel()
	wg.Wait()
}

func TestGCPPubSubQueue_timeout(t *testing.T) {
	// Set cxnTimeout to a value that will be exceeded
	cxnTimeout = time.Millisecond

	var (
		ctx   = context.Background()
		topic = fmt.Sprintf("%s-unit-tests-%v", defaultTopicName, time.Now().Unix())
	)
	_, err := NewGCPPubSubQueue(ctx, projectID, topic)

	have := errors.Cause(err)
	if want := context.DeadlineExceeded; have != want {
		t.Fatalf("have %v, want %v", have, want)
	}
}
