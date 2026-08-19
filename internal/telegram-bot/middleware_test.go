package bot

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	tele "gopkg.in/telebot.v3"
)

type senderContext struct {
	tele.Context
	sender *tele.User
	chat   *tele.Chat
}

func (c senderContext) Sender() *tele.User {
	return c.sender
}

func (c senderContext) Chat() *tele.Chat {
	return c.chat
}

func TestRecoverPanicsConvertsPanicToError(t *testing.T) {
	handler := RecoverPanics()(func(tele.Context) error {
		panic("boom")
	})

	if err := handler(nil); err == nil {
		t.Fatal("panic was not converted to an error")
	}
}

func TestLimitConcurrentRejectsWhenCapacityIsFull(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	handler := LimitConcurrent(1)(func(tele.Context) error {
		select {
		case <-entered:
		default:
			close(entered)
		}
		<-release
		return nil
	})
	done := make(chan error, 1)
	go func() { done <- handler(nil) }()
	<-entered

	if err := handler(nil); !errors.Is(err, ErrBotBusy) {
		close(release)
		t.Fatalf("second handler error = %v, want ErrBotBusy", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("first handler error = %v", err)
	}
}

func TestSenderQueueDoesNotConsumeGlobalCapacity(t *testing.T) {
	firstEntered := make(chan struct{})
	secondSenderEntered := make(chan struct{})
	release := make(chan struct{})
	var firstCalls atomic.Int32

	base := func(c tele.Context) error {
		if c.Sender().ID == 1 {
			if firstCalls.Add(1) == 1 {
				close(firstEntered)
			}
		} else {
			close(secondSenderEntered)
		}
		<-release
		return nil
	}
	handler := SerializeBySender(2)(LimitConcurrent(2)(base))

	done := make(chan error, 3)
	go func() { done <- handler(senderContext{sender: &tele.User{ID: 1}}) }()
	<-firstEntered
	go func() { done <- handler(senderContext{sender: &tele.User{ID: 1}}) }()
	go func() { done <- handler(senderContext{sender: &tele.User{ID: 2}}) }()

	select {
	case <-secondSenderEntered:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("another sender was starved by the first sender queue")
	}
	close(release)
	for range 3 {
		if err := <-done; err != nil {
			t.Fatalf("handler error = %v", err)
		}
	}
}

func TestSerializeBySenderRejectsAfterQueueCapacity(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	handler := SerializeBySender(1)(func(tele.Context) error {
		select {
		case <-entered:
		default:
			close(entered)
		}
		<-release
		return nil
	})
	context := senderContext{sender: &tele.User{ID: 1}}
	done := make(chan error, 2)
	go func() { done <- handler(context) }()
	<-entered
	go func() { done <- handler(context) }()
	time.Sleep(20 * time.Millisecond)

	if err := handler(context); !errors.Is(err, ErrSenderBusy) {
		close(release)
		t.Fatalf("overflow handler error = %v, want ErrSenderBusy", err)
	}
	close(release)
	for range 2 {
		if err := <-done; err != nil {
			t.Fatalf("queued handler error = %v", err)
		}
	}
}

func TestSerializeBySenderSerializesWholeGroupChat(t *testing.T) {
	firstEntered := make(chan struct{})
	secondEntered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	handler := SerializeBySender(1)(func(tele.Context) error {
		if calls.Add(1) == 1 {
			close(firstEntered)
		} else {
			close(secondEntered)
		}
		<-release
		return nil
	})
	chat := &tele.Chat{ID: -100123, Type: tele.ChatGroup}
	done := make(chan error, 2)
	go func() {
		done <- handler(senderContext{sender: &tele.User{ID: 1}, chat: chat})
	}()
	<-firstEntered
	go func() {
		done <- handler(senderContext{sender: &tele.User{ID: 2}, chat: chat})
	}()

	select {
	case <-secondEntered:
		close(release)
		t.Fatal("handlers from one group chat ran concurrently")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	for range 2 {
		if err := <-done; err != nil {
			t.Fatalf("handler error = %v", err)
		}
	}
}

func TestHandlerTrackerWaitsForAcceptedHandlers(t *testing.T) {
	tracker := NewHandlerTracker()
	entered := make(chan struct{})
	release := make(chan struct{})
	handler := tracker.Middleware()(func(tele.Context) error {
		close(entered)
		<-release
		return nil
	})
	done := make(chan error, 1)
	go func() { done <- handler(nil) }()
	<-entered

	tracker.StopAccepting()
	if err := handler(nil); !errors.Is(err, ErrBotBusy) {
		t.Fatalf("handler accepted during shutdown: %v", err)
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	waitDone := make(chan error, 1)
	go func() { waitDone <- tracker.Wait(waitCtx) }()
	select {
	case err := <-waitDone:
		t.Fatalf("tracker returned before handler completed: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if err := <-waitDone; err != nil {
		t.Fatalf("tracker wait error = %v", err)
	}
}
