package service

import (
	"context"
	"fmt"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/google/uuid"
)

type GroupService struct {
	groupRepo *repository.GroupRepository
}

func NewGroupService(groupRepo *repository.GroupRepository) *GroupService {
	return &GroupService{groupRepo: groupRepo}
}

// GetGroupsByUniversity возвращает все группы университета.
func (s *GroupService) GetGroupsByUniversity(ctx context.Context, universityID string) ([]domain.Group, error) {
	return s.groupRepo.GetGroupsByUniversityID(ctx, universityID)
}

// FindOrCreateGroup находит группу по имени или создаёт новую.
func (s *GroupService) FindOrCreateGroup(ctx context.Context, universityID, groupName string, isActive bool) (*domain.Group, error) {
	group, err := s.groupRepo.GetGroupByName(ctx, universityID, groupName)
	if err != nil {
		return nil, err
	}
	if group != nil {
		return group, nil
	}

	// Генерируем ID для новой группы
	id := uuid.New().String()
	_, err = s.groupRepo.CreateGroup(ctx, id, universityID, groupName, isActive)
	if err != nil {
		return nil, fmt.Errorf("failed to create group: %w", err)
	}
	return s.groupRepo.GetGroupByID(ctx, id)
}

// GetGroupByID возвращает группу по ID.
func (s *GroupService) GetGroupByID(ctx context.Context, groupID string) (*domain.Group, error) {
	return s.groupRepo.GetGroupByID(ctx, groupID)
}