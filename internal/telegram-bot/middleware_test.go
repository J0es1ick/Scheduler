package bot

import (
	"errors"
	"testing"

	tele "gopkg.in/telebot.v3"
)

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
