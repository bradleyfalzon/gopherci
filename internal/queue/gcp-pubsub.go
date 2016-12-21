package queue

import (
	"bytes"
	"context"
	"encoding/gob"
	"log"
	"sync"
	"time"

	"github.com/google/go-github/github"
	"github.com/pkg/errors"

	"google.golang.org/api/iterator"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"cloud.google.com/go/pubsub"
)

func init() {
	// List of all types that could be added to the queue
	gob.Register(&github.PullRequestEvent{})
}

const (
	// version should be changed each time the message format changes in an
	// incompatible way. This will then cause new subscribers to listen on the
	// new topic.
	version          = "1"
	defaultSubName   = "worker"
	defaultTopicName = "gopherci-ci"
)

// GCPPubSubQueue is a queue using Google Compute Platform's PubSub product.
type GCPPubSubQueue struct {
	ctx   context.Context // stop listening when this context is cancelled
	c     chan<- interface{}
	topic *pubsub.Topic
}

var _ Queuer = &GCPPubSubQueue{}

// NewGCPPubSubQueue creates a new Queuer and listens on the queue, sending
// new jobs to the channel c, projectID is required but topicName is optional.
// Calls wg.Done() when finished after context has ben cancelled and current
// job has finished.
func NewGCPPubSubQueue(ctx context.Context, wg *sync.WaitGroup, c chan<- interface{}, projectID, topicName string) (*GCPPubSubQueue, error) {
	q := &GCPPubSubQueue{ctx: ctx, c: c}

	if projectID == "" {
		return nil, errors.New("projectID must not be empty")
	}

	client, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		return nil, errors.Wrap(err, "NewGCPPubSubQueue: could not create client")
	}

	if topicName == "" {
		topicName = defaultTopicName
	}
	topicName += "-v" + version

	log.Printf("NewGCPPubSubQueue: creating topic %q", topicName)
	q.topic, err = client.CreateTopic(ctx, topicName)
	if code := grpc.Code(err); code != codes.OK && code != codes.AlreadyExists {
		return nil, errors.Wrap(err, "NewGCPPubSubQueue: could not create topic")
	}

	subName := topicName + "-" + defaultSubName

	log.Printf("NewGCPPubSubQueue: creating subscription %q", subName)
	subscription, err := client.CreateSubscription(ctx, subName, q.topic, 0, nil)
	if code := grpc.Code(err); code != codes.OK && code != codes.AlreadyExists {
		return nil, errors.Wrap(err, "NewGCPPubSubQueue: could not create subscription")
	}

	itr, err := subscription.Pull(q.ctx)
	if err != nil {
		return nil, errors.Wrap(err, "GCPPubSubQueue: could not pull subscription")
	}

	// Close iterator when context closes
	go func() {
		<-q.ctx.Done()
		itr.Stop()
		client.Close()
	}()

	wg.Add(1)
	go q.listen(wg, itr)
	return q, nil
}

// Queue implements the Queue interface.
func (q *GCPPubSubQueue) Queue(job interface{}) error {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(container{job}); err != nil {
		return errors.Wrap(err, "GCPPubSubQueue: could not gob encode job")
	}

	msgIDs, err := q.topic.Publish(q.ctx, &pubsub.Message{
		Data: buf.Bytes(),
	})
	if err != nil {
		return errors.Wrap(err, "GCPPubSubQueue: could not publish job")
	}
	log.Println("GCPPubSubQueue: published a message with a message ID:", msgIDs[0])

	return nil
}

type container struct {
	Job interface{}
}

// listen listens for messages from queue and runs the jobs, returns when
// iterator is stopped, calls wg.Done when returning.
func (q *GCPPubSubQueue) listen(wg *sync.WaitGroup, itr *pubsub.MessageIterator) {
	defer wg.Done()
	for {
		msg, err := itr.Next()
		switch {
		case err == iterator.Done:
			log.Println("GCPPubSubQueue: stopping listening")
			time.Sleep(10 * time.Second)
			log.Println("GCPPubSubQueue: returning")
			return
		case err != nil:
			log.Println("GCPPubSubQueue: could not read next message:", err)
			time.Sleep(3 * time.Second) // back-off
			continue
		}
		// Acknowledge the job now, anything else that could fail by this instance
		// will fail in others.
		msg.Done(true)

		log.Printf("GCPPubSubQueue: processing ID %v, published at %v", msg.ID, msg.PublishTime)

		reader := bytes.NewReader(msg.Data)
		dec := gob.NewDecoder(reader)

		var job container
		if err := dec.Decode(&job); err != nil {
			log.Println("GCPPubSubQueue: could not decode job:", err)
			continue
		}
		q.c <- job.Job
	}
}

// delete deletes the topic and subcriptions, used to cleanup unit tests
func (q *GCPPubSubQueue) delete() {
	itr := q.topic.Subscriptions(q.ctx)
	for {
		sub, err := itr.Next()
		if err != nil {
			break
		}
		err = sub.Delete(q.ctx)
		if err != nil {
			log.Println("GCPPubSubQueue: delete subscription error:", err)
		}
	}
	err := q.topic.Delete(q.ctx)
	if err != nil {
		log.Println("GCPPubSubQueue: delete topic error:", err)
	}
}
