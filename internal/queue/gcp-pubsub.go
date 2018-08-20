package queue

import (
	"bytes"
	"context"
	"encoding/gob"
	"sync"
	"time"

	xContext "golang.org/x/net/context"

	"github.com/bradleyfalzon/gopherci/internal/logger"
	"github.com/google/go-github/github"
	"github.com/pkg/errors"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"cloud.google.com/go/pubsub"
)

func init() {
	// List of all types that could be added to the queue
	gob.Register(&github.PullRequestEvent{})
	gob.Register(&github.PushEvent{})
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
	logger       logger.Logger
	topic        *pubsub.Topic
	subscription *pubsub.Subscription
}

var cxnTimeout = 15 * time.Second

// NewGCPPubSubQueue creates connects to Google Pub/Sub with a topic and
// subscriber in a one-to-one architecture.
func NewGCPPubSubQueue(ctx context.Context, logger logger.Logger, projectID, topicName string) (*GCPPubSubQueue, error) {
	q := &GCPPubSubQueue{logger: logger}

	if projectID == "" {
		return nil, errors.New("projectID must not be empty")
	}

	// create a context with a timeout for exclusive use of connection setup to
	// ensure connnection setup doesn't block and can fail early.
	cxnCtx, cancel := context.WithTimeout(ctx, cxnTimeout)
	defer cancel()

	client, err := pubsub.NewClient(cxnCtx, projectID)
	if err != nil {
		return nil, errors.Wrap(err, "could not create client")
	}

	if topicName == "" {
		topicName = defaultTopicName
	}
	topicName += "-v" + version

	logger.Infof("creating topic %q", topicName)
	q.topic, err = client.CreateTopic(cxnCtx, topicName)
	if code := grpc.Code(err); code != codes.OK && code != codes.AlreadyExists {
		return nil, errors.Wrap(err, "could not create topic")
	}

	subName := topicName + "-" + defaultSubName

	logger.Infof("creating subscription %q", subName)
	q.subscription, err = client.CreateSubscription(cxnCtx, subName, pubsub.SubscriptionConfig{
		Topic:       q.topic,
		AckDeadline: 0,
	})
	if code := grpc.Code(err); code != codes.OK && code != codes.AlreadyExists {
		return nil, errors.Wrap(err, "could not create subscription")
	}

	q.subscription.ReceiveSettings.MaxOutstandingMessages = 1 // limit concurrency

	return q, nil
}

// Wait waits for messages on queuePush and adds them to the Pub/Sub queue.
// Upon receiving messages from Pub/Sub, f is invoked with the message. Wait
// is non-blocking, increments wg for each routine started, and when context
// is closed will mark the wg as done as routines are shutdown.
func (q GCPPubSubQueue) Wait(ctx context.Context, wg *sync.WaitGroup, queuePush <-chan interface{}, f func(interface{})) {
	// Routine to add jobs to the GCP Pub/Sub Queue
	wg.Add(1)
	go func() {
		for {
			select {
			case <-ctx.Done():
				q.logger.Info("job waiter exiting")
				q.topic.Stop()
				wg.Done()
				return
			case job := <-queuePush:
				q.logger.Info("job waiter got message, queuing...")
				if err := q.queue(ctx, job); err != nil {
					q.logger.With("error", err).Error("could not queue job")
				}
			}
		}
	}()

	// Routine to listen for jobs and process one at a time
	wg.Add(1)
	go func() {
		q.receive(ctx, f)
		q.logger.Info("job receiver exiting")
		wg.Done()
	}()
}

// queue adds a message to the queue.
func (q *GCPPubSubQueue) queue(ctx context.Context, job interface{}) error {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(container{job}); err != nil {
		return errors.Wrap(err, "could not gob encode job")
	}

	var (
		msg         = &pubsub.Message{Data: buf.Bytes()}
		maxAttempts = 3
		msgID       string
		err         error
	)
	for i := 1; i <= maxAttempts; i++ {
		res := q.topic.Publish(ctx, msg)
		msgID, err = res.Get(ctx)
		if err == nil {
			break
		}
		q.logger.With("error", err).Infof("failed publishing message attempt %v of %v", i, maxAttempts)
		time.Sleep(time.Duration(i) * time.Second)
	}
	if err != nil {
		return errors.Wrap(err, "could not publish job")
	}
	q.logger.With("messageID", msgID).Info("published job")

	return nil
}

type container struct {
	Job interface{}
}

// receive calls sub.Receive, which blocks forever waiting for new jobs.
func (q *GCPPubSubQueue) receive(ctx context.Context, f func(interface{})) {
	err := q.subscription.Receive(ctx, func(ctx xContext.Context, msg *pubsub.Message) {
		logger_ := q.logger.With("messageID", msg.ID)

		logger_.With("publishTime", msg.PublishTime).Info("processing job published")

		// Acknowledge the job now, anything else that could fail by this instance
		// will probably fail for others.
		msg.Ack()
		logger_.Info("acknowledged job")

		reader := bytes.NewReader(msg.Data)
		dec := gob.NewDecoder(reader)

		var job container
		if err := dec.Decode(&job); err != nil {
			logger_.With("error", err).Errorf("could not decode job")
			return
		}
		logger_.Info("processing")

		f(job.Job)
	})
	if err != nil && err != context.Canceled {
		q.logger.With("error", err).Error("could not receive on subscription")
	}
}

// delete deletes the topic and subcriptions, used to cleanup unit tests.
func (q *GCPPubSubQueue) delete(ctx context.Context) {
	itr := q.topic.Subscriptions(ctx)
	for {
		sub, err := itr.Next()
		if err != nil {
			break
		}
		err = sub.Delete(ctx)
		if err != nil {
			q.logger.With("error", err).Error("could not delete subscription")
		}
	}
	err := q.topic.Delete(ctx)
	if err != nil {
		q.logger.With("error", err).Error("could not delete topic")
	}
}
