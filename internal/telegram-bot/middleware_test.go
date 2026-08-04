package bot

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	tele "gopkg.in/telebot.v3"
)

type senderContext struct {
	tele.Context
	sender *tele.User
}

func (c senderContext) Sender() *tele.User {
	return c.sender
}

func TestRecoverPanicsConvertsPanicToError(t *testing.T) {
	handler := RecoverPanics()(func(tele.Context) error {
		panic("boom")
	})

	if err := handler(nil); err == nil {
		t.Fatal("panic was not converted to an error")
	}
}

func TestLimitConcurrentRejectsExcessWork(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	handler := LimitConcurrent(1)(func(tele.Context) error {
		close(entered)
		<-release
		return nil
	})
	done := make(chan error, 1)
	go func() { done <- handler(nil) }()
	<-entered

	if err := handler(nil); !errors.Is(err, ErrBotBusy) {
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

func TestSerializeBySenderRejectsQueueOverflow(t *testing.T) {
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
