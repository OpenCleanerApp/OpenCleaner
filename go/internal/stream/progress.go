package stream

import (
	"context"
	"sync"

	"github.com/opencleaner/opencleaner/pkg/types"
)

type Broker struct {
	mu     sync.Mutex
	nextID int
	subs   map[int]chan types.ProgressEvent
	last   *types.ProgressEvent
}

func NewBroker() *Broker {
	return &Broker{subs: map[int]chan types.ProgressEvent{}}
}

func (b *Broker) Publish(evt types.ProgressEvent) {
	b.mu.Lock()
	b.last = &evt
	for _, ch := range b.subs {
		select {
		case ch <- evt:
		default:
			// drop/coalesce: progress events are lossy by design
		}
	}
	b.mu.Unlock()
}

func (b *Broker) Subscribe(ctx context.Context) <-chan types.ProgressEvent {
	ch := make(chan types.ProgressEvent, 64)

	b.mu.Lock()
	id := b.nextID
	b.nextID++
	b.subs[id] = ch
	last := b.last
	b.mu.Unlock()

	if last != nil {
		ch <- *last
	}

	go func() {
		<-ctx.Done()
		b.mu.Lock()
		delete(b.subs, id)
		close(ch)
		b.mu.Unlock()
	}()

	return ch
}
