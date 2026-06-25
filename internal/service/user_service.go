// internal/service/user_service.go
package service

import (
	"context"
	"fmt"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
)

type UserService struct {
	userRepo *repository.UserRepository
}

func NewUserService(userRepo *repository.UserRepository) *UserService {
	return &UserService{userRepo: userRepo}
}

// RegisterOrGetUser регистрирует пользователя, если его нет, и возвращает его.
func (s *UserService) RegisterOrGetUser(ctx context.Context, telegramID, username string) (*domain.User, error) {
	user, err := s.userRepo.GetUserByID(ctx, telegramID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	if user != nil {
		return user, nil
	}

	// Новый пользователь – не админ
	_, err = s.userRepo.CreateUser(ctx, telegramID, username, false)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}
	return s.userRepo.GetUserByID(ctx, telegramID)
}

// GetUser возвращает пользователя по Telegram ID.
func (s *UserService) GetUser(ctx context.Context, userID string) (*domain.User, error) {
	return s.userRepo.GetUserByID(ctx, userID)
}

// IsAdmin проверяет, является ли пользователь администратором.
func (s *UserService) IsAdmin(ctx context.Context, userID string) (bool, error) {
	user, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil || user == nil {
		return false, err
	}
	return user.IsAdmin, nil
}

// SetAdmin устанавливает флаг администратора (только для служебных команд).
func (s *UserService) SetAdmin(ctx context.Context, userID string, isAdmin bool) error {
	user, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil || user == nil {
		return fmt.Errorf("user not found")
	}
	return s.userRepo.UpdateUser(ctx, userID, user.Username, isAdmin)
}