package queue

import (
	"context"
	"log"
	"sync"
	"time"
)

const pollInterval = 500 * time.Millisecond

// MemoryQueue is an in memory queue of infinite size.
type MemoryQueue struct {
	mu    sync.Mutex // protects queue
	queue []interface{}
}

// NewMemoryQueue creates a new in memory queue
func NewMemoryQueue() *MemoryQueue {
	return &MemoryQueue{}
}

// Wait waits for messages on queuePush and adds them to the queue. New
// message are checked for regularly and when a new message is ready f
// will be called with the argument of the job.
func (q *MemoryQueue) Wait(ctx context.Context, wg *sync.WaitGroup, queuePush <-chan interface{}, f func(interface{})) {
	// Routine to add jobs to the queue
	wg.Add(1)
	go func() {
		for {
			select {
			case <-ctx.Done():
				log.Println("MemoryQueue job waiter exiting")
				wg.Done()
				return
			case job := <-queuePush:
				log.Println("MemoryQueue job waiter got message, queuing...")
				q.mu.Lock()
				q.queue = append(q.queue, job)
				q.mu.Unlock()
			}
		}
	}()

	// Routine to listen for jobs and process one at a time
	wg.Add(1)
	go func() {
		q.receive(ctx, f)
		log.Println("GCPPubSubQueue job receiver exiting")
		wg.Done()
	}()
}

// receive polls the queue for new jobs and sends them on the pop channel.
func (q *MemoryQueue) receive(ctx context.Context, f func(interface{})) {
	ticker := time.NewTicker(pollInterval)
	for {
		select {
		case <-ctx.Done():
			log.Println("listen stopping")
			ticker.Stop()
			return
		case <-ticker.C:
			if len(q.queue) == 0 {
				break
			}
			// queue the next item
			var job interface{}
			q.mu.Lock()
			job, q.queue = q.queue[len(q.queue)-1], q.queue[:len(q.queue)-1]
			q.mu.Unlock()
			f(job)
		}
	}
}
