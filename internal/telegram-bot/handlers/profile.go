package handlers

import (
	"context"
	"fmt"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/dto"
)

// restoreProfile recreates only the durable, completed part of a Telegram
// session. In-progress dialogs remain intentionally ephemeral.
func (h *Handler) restoreProfile(
	ctx context.Context,
	telegramID int64,
) (*dto.UserState, *domain.User, error) {
	userID := fmt.Sprint(telegramID)
	user, err := h.UserService.GetUser(ctx, userID)
	if err != nil || user == nil {
		return nil, user, err
	}
	if user.DefaultGroupID == "" {
		return nil, user, nil
	}

	group, err := h.GroupService.GetGroupByID(ctx, user.DefaultGroupID)
	if err != nil {
		return nil, user, err
	}
	if group == nil || !group.IsActive {
		if err = h.UserService.SetDefaultGroup(ctx, userID, ""); err != nil {
			return nil, user, err
		}
		h.StateManager.Delete(telegramID)
		user.DefaultGroupID = ""
		return nil, user, nil
	}

	university, err := h.UniversityService.GetByID(ctx, group.UniversityID)
	if err != nil {
		return nil, user, err
	}
	if university == nil || !university.IsActive {
		return nil, user, nil
	}

	state := &dto.UserState{
		UniversityID: group.UniversityID,
		University:   university.Name,
		SearchType:   dto.SearchTypeGroup,
		Query:        group.Name,
		GroupID:      group.ID,
		Step:         "done",
	}
	h.StateManager.Set(telegramID, state)
	return state, user, nil
}

func (h *Handler) readyState(ctx context.Context, telegramID int64) (*dto.UserState, error) {
	if current := h.StateManager.Get(telegramID); current != nil && current.Step == "done" {
		return current, nil
	}
	restored, _, err := h.restoreProfile(ctx, telegramID)
	return restored, err
}
