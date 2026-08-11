package worker

import (
	"testing"
	"time"
)

func TestMonitorDetectsStoppedAndStaleWorkers(t *testing.T) {
	now := time.Date(2026, time.August, 11, 12, 0, 0, 0, time.UTC)
	monitor := NewMonitor()
	monitor.now = func() time.Time { return now }
	monitor.Register(ParserWorkerName, time.Minute)
	if monitor.Checks()[ParserWorkerName] {
		t.Fatal("registered but unstarted worker reported ready")
	}
	monitor.Started(ParserWorkerName)
	if !monitor.Checks()[ParserWorkerName] {
		t.Fatal("started worker reported unready")
	}
	now = now.Add(2 * time.Minute)
	if monitor.Checks()[ParserWorkerName] {
		t.Fatal("stale worker reported ready")
	}
	monitor.Heartbeat(ParserWorkerName)
	monitor.Stopped(ParserWorkerName)
	if monitor.Checks()[ParserWorkerName] {
		t.Fatal("stopped worker reported ready")
	}
}
