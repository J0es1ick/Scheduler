package worker

import (
	"sync"
	"time"
)

const (
	ParserWorkerName       = "parser"
	ReminderWorkerName     = "reminder"
	NotificationWorkerName = "notification"
	ConnectorWorkerName    = "connector"
)

type workerProbe struct {
	running       bool
	lastHeartbeat time.Time
	maxSilence    time.Duration
}

type Monitor struct {
	mu      sync.RWMutex
	workers map[string]workerProbe
	now     func() time.Time
}

func NewMonitor() *Monitor {
	return &Monitor{workers: make(map[string]workerProbe), now: time.Now}
}

func (m *Monitor) Register(name string, maxSilence time.Duration) {
	if m == nil {
		return
	}
	if maxSilence <= 0 {
		maxSilence = time.Minute
	}
	m.mu.Lock()
	m.workers[name] = workerProbe{maxSilence: maxSilence}
	m.mu.Unlock()
}

func (m *Monitor) Started(name string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	probe := m.workers[name]
	probe.running = true
	probe.lastHeartbeat = m.now()
	m.workers[name] = probe
	m.mu.Unlock()
}

func (m *Monitor) Heartbeat(name string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	probe := m.workers[name]
	probe.lastHeartbeat = m.now()
	m.workers[name] = probe
	m.mu.Unlock()
}

func (m *Monitor) Stopped(name string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	probe := m.workers[name]
	probe.running = false
	m.workers[name] = probe
	m.mu.Unlock()
}

func (m *Monitor) Checks() map[string]bool {
	result := make(map[string]bool)
	if m == nil {
		return result
	}
	now := m.now()
	m.mu.RLock()
	defer m.mu.RUnlock()
	for name, probe := range m.workers {
		result[name] = probe.running && !probe.lastHeartbeat.IsZero() && now.Sub(probe.lastHeartbeat) <= probe.maxSilence
	}
	return result
}
