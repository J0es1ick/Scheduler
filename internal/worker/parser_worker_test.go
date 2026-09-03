package worker

import (
	"context"
	"testing"
	"time"
)

type blockingParserService struct {
	started          chan struct{}
	cancelled        chan struct{}
	release          chan struct{}
	cleanupCompleted chan struct{}
}

func (s *blockingParserService) CleanupInterruptedRuns(context.Context, time.Duration) error {
	close(s.cleanupCompleted)
	return nil
}

func (s *blockingParserService) RunAllActiveSources(ctx context.Context) error {
	close(s.started)
	<-ctx.Done()
	close(s.cancelled)
	<-s.release
	return ctx.Err()
}

func TestParserWorkerWaitsForActiveRunOnShutdown(t *testing.T) {
	parser := &blockingParserService{
		started:          make(chan struct{}),
		cancelled:        make(chan struct{}),
		release:          make(chan struct{}),
		cleanupCompleted: make(chan struct{}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := NewParserWorker(parser, time.Hour).Start(ctx)

	select {
	case <-parser.started:
	case <-time.After(time.Second):
		t.Fatal("parser run did not start")
	}
	cancel()
	select {
	case <-parser.cancelled:
	case <-time.After(time.Second):
		t.Fatal("parser run did not observe cancellation")
	}
	select {
	case <-done:
		t.Fatal("worker stopped before active parser run returned")
	case <-time.After(50 * time.Millisecond):
	}

	close(parser.release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not stop after active parser run returned")
	}
}
