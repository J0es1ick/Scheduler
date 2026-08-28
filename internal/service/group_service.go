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

func (s *GroupService) GetGroupsByUniversity(ctx context.Context, universityID string) ([]domain.Group, error) {
	return s.groupRepo.GetGroupsByUniversityID(ctx, universityID)
}

func (s *GroupService) GetGroupByID(ctx context.Context, groupID string) (*domain.Group, error) {
	return s.groupRepo.GetGroupByID(ctx, groupID)
}

func (s *GroupService) GetGroupByName(ctx context.Context, universityID, groupName string) (*domain.Group, error) {
	return s.groupRepo.GetGroupByName(ctx, universityID, groupName)
}

func (s *GroupService) GetActiveGroupByToken(ctx context.Context, token string) (*domain.Group, error) {
	return s.groupRepo.GetActiveGroupByToken(ctx, token)
}

func (s *GroupService) FindActiveByName(
	ctx context.Context,
	universityID string,
	groupName string,
) ([]domain.Group, error) {
	return s.groupRepo.FindActiveByName(ctx, universityID, groupName)
}
