package handlers

import (
	"context"
	"fmt"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/searchtext"
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
	if user.Role == domain.RoleTeacher {
		if user.TeacherID == "" {
			return nil, user, nil
		}
		teacher, loadErr := h.ProfileService.GetTeacher(ctx, user.TeacherID)
		if loadErr != nil || teacher == nil {
			return nil, user, loadErr
		}
		university, loadErr := h.UniversityService.GetByID(ctx, teacher.UniversityID)
		if loadErr != nil || university == nil {
			return nil, user, loadErr
		}
		names, loadErr := h.ScheduleService.FindTeachers(ctx, teacher.UniversityID, "")
		if loadErr != nil {
			return nil, user, loadErr
		}
		active := false
		for _, name := range names {
			if searchtext.TokenKey(name) == teacher.NameKey {
				active = true
				break
			}
		}
		state := &dto.UserState{Role: domain.RoleTeacher, TeacherID: teacher.ID, TeacherName: teacher.Name, UniversityID: university.ID, University: university.Name, Query: teacher.Name, GroupActive: active && university.IsActive, Step: "done"}
		h.StateManager.Set(telegramID, state)
		return state, user, nil
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
		Role:         domain.RoleStudent,
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
