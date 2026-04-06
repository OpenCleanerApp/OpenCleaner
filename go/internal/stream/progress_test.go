package stream

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/opencleaner/opencleaner/pkg/types"
)

func TestNewBroker(t *testing.T) {
	b := NewBroker()
	if b == nil {
		t.Fatal("expected non-nil broker")
	}
	if b.SubscriberCount() != 0 {
		t.Errorf("expected 0 subscribers, got %d", b.SubscriberCount())
	}
}

func TestPublishNoSubscribers(t *testing.T) {
	b := NewBroker()
	// Should not panic.
	b.Publish(types.ProgressEvent{Type: "test"})
}

func TestSubscribeReceivesEvent(t *testing.T) {
	b := NewBroker()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := b.Subscribe(ctx)
	if b.SubscriberCount() != 1 {
		t.Fatalf("expected 1 subscriber, got %d", b.SubscriberCount())
	}

	b.Publish(types.ProgressEvent{Type: "scanning", Progress: 0.5, Message: "half"})

	select {
	case evt := <-ch:
		if evt.Type != "scanning" || evt.Progress != 0.5 {
			t.Errorf("unexpected event: %+v", evt)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestSubscribeReceivesLastEvent(t *testing.T) {
	b := NewBroker()
	b.Publish(types.ProgressEvent{Type: "prior", Message: "cached"})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := b.Subscribe(ctx)

	select {
	case evt := <-ch:
		if evt.Type != "prior" {
			t.Errorf("expected prior event, got %+v", evt)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for last event replay")
	}
}

func TestSubscribeCancelUnsubscribes(t *testing.T) {
	b := NewBroker()
	ctx, cancel := context.WithCancel(context.Background())
	ch := b.Subscribe(ctx)

	if b.SubscriberCount() != 1 {
		t.Fatalf("expected 1, got %d", b.SubscriberCount())
	}

	cancel()
	// Wait for goroutine to unsubscribe.
	time.Sleep(50 * time.Millisecond)

	if b.SubscriberCount() != 0 {
		t.Errorf("expected 0 after cancel, got %d", b.SubscriberCount())
	}

	// Channel should be closed.
	_, ok := <-ch
	if ok {
		t.Error("expected channel to be closed")
	}
}

func TestMultipleSubscribers(t *testing.T) {
	b := NewBroker()
	ctx1, cancel1 := context.WithCancel(context.Background())
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel1()
	defer cancel2()

	ch1 := b.Subscribe(ctx1)
	ch2 := b.Subscribe(ctx2)
	if b.SubscriberCount() != 2 {
		t.Fatalf("expected 2, got %d", b.SubscriberCount())
	}

	b.Publish(types.ProgressEvent{Type: "broadcast"})

	for i, ch := range []<-chan types.ProgressEvent{ch1, ch2} {
		select {
		case evt := <-ch:
			if evt.Type != "broadcast" {
				t.Errorf("subscriber %d: expected broadcast, got %+v", i, evt)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d: timed out", i)
		}
	}
}

func TestPublishDropsWhenFull(t *testing.T) {
	b := NewBroker()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := b.Subscribe(ctx)

	// Fill the channel buffer (64).
	for i := 0; i < 70; i++ {
		b.Publish(types.ProgressEvent{Type: "flood"})
	}

	// Drain what we can.
	count := 0
	for {
		select {
		case <-ch:
			count++
		default:
			goto done
		}
	}
done:
	if count == 0 {
		t.Error("expected at least some events")
	}
	// Should not have more than buffer size + last event replay.
	if count > 66 {
		t.Errorf("expected ≤66 events (64 buf + last replay + 1), got %d", count)
	}
}

func TestConcurrentPublish(t *testing.T) {
	b := NewBroker()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := b.Subscribe(ctx)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				b.Publish(types.ProgressEvent{Type: "concurrent"})
			}
		}()
	}
	wg.Wait()

	count := 0
	for {
		select {
		case <-ch:
			count++
		default:
			goto done2
		}
	}
done2:
	if count == 0 {
		t.Error("expected events from concurrent publish")
	}
}
