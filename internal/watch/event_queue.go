package watch

import "sync"

// EventQueue is an unbounded thread-safe queue for events.
// It allows non-blocking sends and provides a channel to receive from.
type EventQueue struct {
	out      chan Event
	mu       sync.Mutex
	cond     *sync.Cond
	queue    []Event
	closed   bool
	done     chan struct{}
	stopOnce sync.Once
}

type WatchEventPublisher interface {
	Publish(Event)
}

type QueueEventPublisher struct {
	Queue *EventQueue
}

func (p QueueEventPublisher) Publish(event Event) {
	emit(p.Queue, event)
}

func emit(eq *EventQueue, event Event) {
	if eq == nil {
		return
	}
	eq.Push(event)
}

// NewEventQueue creates a new unbounded event queue.
func NewEventQueue() *EventQueue {
	eq := &EventQueue{
		out:   make(chan Event, 64),
		queue: make([]Event, 0, 64),
		done:  make(chan struct{}),
	}
	eq.cond = sync.NewCond(&eq.mu)
	go eq.run()
	return eq
}

// Push adds an event to the queue without blocking.
func (eq *EventQueue) Push(e Event) {
	if eq == nil {
		return
	}
	eq.mu.Lock()
	defer eq.mu.Unlock()
	if eq.closed {
		return
	}
	eq.queue = append(eq.queue, e)
	eq.cond.Signal()
}

// Out returns the channel to read events from.
func (eq *EventQueue) Out() <-chan Event {
	if eq == nil {
		return nil
	}
	return eq.out
}

// Close stops the queue.
func (eq *EventQueue) Close() {
	if eq == nil {
		return
	}
	eq.stopOnce.Do(func() {
		eq.mu.Lock()
		eq.closed = true
		eq.cond.Broadcast()
		eq.mu.Unlock()
		close(eq.done)
	})
}

func (eq *EventQueue) run() {
	defer close(eq.out)
	for {
		eq.mu.Lock()
		for len(eq.queue) == 0 && !eq.closed {
			eq.cond.Wait()
		}
		if len(eq.queue) == 0 {
			eq.mu.Unlock()
			return
		}
		event := eq.queue[0]
		eq.queue[0] = Event{}
		eq.queue = eq.queue[1:]
		if len(eq.queue) == 0 && cap(eq.queue) > 1024 {
			eq.queue = make([]Event, 0, 64)
		}
		eq.mu.Unlock()

		select {
		case eq.out <- event:
			continue
		default:
		}
		select {
		case eq.out <- event:
		case <-eq.done:
			return
		}
	}
}
