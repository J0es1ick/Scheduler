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
	lastSuccess   time.Time
	lastFailure   time.Time
	lastError     string
	hasResult     bool
	lastResultOK  bool
	degraded      bool
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

func (m *Monitor) Succeeded(name string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	probe := m.workers[name]
	probe.lastSuccess = m.now()
	probe.hasResult = true
	probe.lastResultOK = true
	probe.degraded = false
	probe.lastError = ""
	m.workers[name] = probe
	m.mu.Unlock()
}

func (m *Monitor) Failed(name string, err error) {
	if m == nil {
		return
	}
	m.mu.Lock()
	probe := m.workers[name]
	probe.lastFailure = m.now()
	probe.hasResult = true
	probe.lastResultOK = false
	probe.degraded = false
	if err != nil {
		probe.lastError = err.Error()
	} else {
		probe.lastError = "worker pass failed"
	}
	m.workers[name] = probe
	m.mu.Unlock()
}

func (m *Monitor) Degraded(name string, err error) {
	if m == nil {
		return
	}
	m.mu.Lock()
	probe := m.workers[name]
	probe.lastFailure = m.now()
	probe.hasResult = true
	probe.lastResultOK = true
	probe.degraded = true
	if err != nil {
		probe.lastError = err.Error()
	} else {
		probe.lastError = "external dependency degraded"
	}
	m.workers[name] = probe
	m.mu.Unlock()
}

func (m *Monitor) Record(name string, err error) {
	if err != nil {
		m.Failed(name, err)
		return
	}
	m.Succeeded(name)
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
		result[name] = probe.running && probe.hasResult && probe.lastResultOK &&
			!probe.lastHeartbeat.IsZero() && now.Sub(probe.lastHeartbeat) <= probe.maxSilence
	}
	return result
}

func (m *Monitor) DegradedChecks() map[string]bool {
	result := make(map[string]bool)
	if m == nil {
		return result
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for name, probe := range m.workers {
		result[name] = probe.degraded
	}
	return result
}
