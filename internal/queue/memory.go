package queue

import (
	"context"
	"log"
	"sync"
	"time"
)

// MemoryQueue is an in memory queue of infinite size.
type MemoryQueue struct {
	ctx   context.Context // stop listening when this context is cancelled
	c     chan<- interface{}
	mu    sync.Mutex // protects queue
	queue []interface{}
}

var _ Queuer = &MemoryQueue{}

// NewMemoryQueue creates a new Queuer and listens on the queue, sending new
// jobs to the channel c. Calls wg.Done() when finished after context has ben
// cancelled and current job has finished.
func NewMemoryQueue(ctx context.Context, wg *sync.WaitGroup, c chan<- interface{}) *MemoryQueue {
	q := &MemoryQueue{ctx: ctx, c: c}
	wg.Add(1)
	go q.listen(wg)
	return q
}

// Queue implements the Queue interface.
func (q *MemoryQueue) Queue(job interface{}) error {
	q.mu.Lock()
	q.queue = append(q.queue, job)
	q.mu.Unlock()
	return nil
}

// listen polls the queue for new jobs and sends them on the pop channel.
func (q *MemoryQueue) listen(wg *sync.WaitGroup) {
	defer wg.Done()
	ticker := time.NewTicker(time.Second) // poll interval
	for {
		select {
		case <-q.ctx.Done():
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
			// this could block for a long time, we're ok with that
			q.c <- job
		}
	}
}
