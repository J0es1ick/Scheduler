package service

import (
	"context"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
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

// GetGroupByID возвращает группу по ID.
func (s *GroupService) GetGroupByID(ctx context.Context, groupID string) (*domain.Group, error) {
	return s.groupRepo.GetGroupByID(ctx, groupID)
}

// GetGroupByName ищет группу по имени, не создаёт если не найдена.
func (s *GroupService) GetGroupByName(ctx context.Context, universityID, groupName string) (*domain.Group, error) {
	return s.groupRepo.GetGroupByName(ctx, universityID, groupName)
}
