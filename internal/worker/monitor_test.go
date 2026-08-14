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
	if monitor.Checks()[ParserWorkerName] {
		t.Fatal("worker without a successful pass reported ready")
	}
	monitor.Succeeded(ParserWorkerName)
	if !monitor.Checks()[ParserWorkerName] {
		t.Fatal("started worker reported unready")
	}
	monitor.Degraded(ParserWorkerName, assertError("schedule site unavailable"))
	if !monitor.Checks()[ParserWorkerName] {
		t.Fatal("worker degraded by an external source reported unready")
	}
	if !monitor.DegradedChecks()[ParserWorkerName] {
		t.Fatal("external source degradation was not exposed")
	}
	monitor.Failed(ParserWorkerName, assertError("database unavailable"))
	if monitor.Checks()[ParserWorkerName] {
		t.Fatal("worker with a newer failed pass reported ready")
	}
	monitor.Succeeded(ParserWorkerName)
	if !monitor.Checks()[ParserWorkerName] {
		t.Fatal("worker did not recover after a successful pass")
	}
	if monitor.DegradedChecks()[ParserWorkerName] {
		t.Fatal("successful pass did not clear degraded state")
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

type assertError string

func (e assertError) Error() string { return string(e) }
