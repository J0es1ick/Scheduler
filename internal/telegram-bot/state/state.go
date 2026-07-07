package state

import (
	"sync"

	"github.com/J0es1ick/Scheduler/internal/telegram-bot/dto"
)

type Manager struct {
	mu         sync.RWMutex
	userStates map[int64]*dto.UserState
}

func NewManager() *Manager {
	return &Manager{
		userStates: make(map[int64]*dto.UserState),
	}
}

func (m *Manager) Get(userID int64) *dto.UserState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.userStates[userID]
}

func (m *Manager) Set(userID int64, state *dto.UserState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.userStates[userID] = state
}
