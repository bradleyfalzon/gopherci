package queue

// Queuer pushes jobs onto a queue and pushes the next job on the provided
// channel.
type Queuer interface {
	Queue(interface{}) error
}
