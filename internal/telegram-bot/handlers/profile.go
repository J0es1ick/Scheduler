package handlers

import (
	"context"
	"fmt"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/dto"
)

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
	if group == nil {
		h.StateManager.Delete(telegramID)
		return nil, user, nil
	}

	university, err := h.UniversityService.GetByID(ctx, group.UniversityID)
	if err != nil {
		return nil, user, err
	}
	if university == nil {
		return nil, user, nil
	}

	state := &dto.UserState{
		UniversityID: group.UniversityID,
		University:   university.Name,
		SearchType:   dto.SearchTypeGroup,
		Query:        group.Name,
		GroupID:      group.ID,
		GroupActive:  group.IsActive && university.IsActive,
		Step:         "done",
	}
	h.StateManager.Set(telegramID, state)
	return state, user, nil
}

func (h *Handler) readyState(ctx context.Context, telegramID int64) (*dto.UserState, error) {
	restored, _, err := h.restoreProfile(ctx, telegramID)
	return restored, err
}
