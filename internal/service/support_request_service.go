package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/google/uuid"
)

var ErrInvalidSupportRequest = errors.New("invalid support request")

type SupportRequestService struct {
	repository *repository.SupportRequestRepository
}

func NewSupportRequestService(repository *repository.SupportRequestRepository) *SupportRequestService {
	return &SupportRequestService{repository: repository}
}

func (s *SupportRequestService) Submit(ctx context.Context, userID, requestType, details string) (string, error) {
	details = strings.TrimSpace(details)
	if err := validateSupportRequest(requestType, details); err != nil {
		return "", err
	}
	id := uuid.NewString()
	if err := s.repository.Create(ctx, id, userID, requestType, details); err != nil {
		return "", err
	}
	return id, nil
}

func validateSupportRequest(requestType, details string) error {
	if requestType != domain.SupportRequestUpdateExisting && requestType != domain.SupportRequestNewInstitution {
		return fmt.Errorf("%w: unsupported type", ErrInvalidSupportRequest)
	}
	length := utf8.RuneCountInString(details)
	if length < 20 || length > 4096 {
		return fmt.Errorf("%w: details length is %d", ErrInvalidSupportRequest, length)
	}
	return nil
}
