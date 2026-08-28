package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/J0es1ick/Scheduler/internal/telegram-bot/dto"
	tele "gopkg.in/telebot.v3"
)

func newFlowNonce() string {
	buffer := make([]byte, 8)
	if _, err := rand.Read(buffer); err == nil {
		return hex.EncodeToString(buffer)
	}
	return fmt.Sprintf("%x", time.Now().UnixNano())
}

func (h *Handler) finishTransientFlow(c tele.Context) error {
	if isGroupChat(c) || c.Sender() == nil || h.StateManager == nil ||
		h.UserService == nil || h.GroupService == nil || h.UniversityService == nil {
		return nil
	}
	ctx, cancel := reqCtx()
	defer cancel()
	state, _, err := h.restoreProfile(ctx, c.Sender().ID)
	if err != nil {
		return err
	}
	if state == nil {
		h.StateManager.Delete(c.Sender().ID)
	}
	return nil
}

func validFlow(state *dto.UserState, step, nonce string) bool {
	return state != nil && state.Step == step && state.FlowNonce != "" && state.FlowNonce == nonce
}
