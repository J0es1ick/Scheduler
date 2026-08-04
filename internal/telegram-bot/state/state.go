package state

import (
	"sync"
	"time"

	"github.com/J0es1ick/Scheduler/internal/telegram-bot/dto"
)

type Manager struct {
	mu         sync.Mutex
	userStates map[int64]entry
	ttl        time.Duration
	now        func() time.Time
}

type entry struct {
	state     dto.UserState
	expiresAt time.Time
}

const defaultTTL = 30 * time.Minute

func NewManager() *Manager {
	return NewManagerWithTTL(defaultTTL)
}

func NewManagerWithTTL(ttl time.Duration) *Manager {
	if ttl <= 0 {
		ttl = defaultTTL
	}
	return &Manager{
		userStates: make(map[int64]entry),
		ttl:        ttl,
		now:        time.Now,
	}
}

func (m *Manager) Get(userID int64) *dto.UserState {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.userStates[userID]
	if !ok {
		return nil
	}
	if !item.expiresAt.After(m.now()) {
		delete(m.userStates, userID)
		return nil
	}
	stateCopy := item.state
	return &stateCopy
}

func (m *Manager) Set(userID int64, state *dto.UserState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if state == nil {
		delete(m.userStates, userID)
		return
	}
	m.userStates[userID] = entry{
		state:     *state,
		expiresAt: m.now().Add(m.ttl),
	}
}

func (m *Manager) Delete(userID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.userStates, userID)
}
