package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/J0es1ick/Scheduler/internal/domain"
)

func TestValidateSupportRequest(t *testing.T) {
	tests := []struct {
		name        string
		requestType string
		details     string
		wantErr     bool
	}{
		{"existing schedule", domain.SupportRequestUpdateExisting, strings.Repeat("я", 20), false},
		{"new institution", domain.SupportRequestNewInstitution, strings.Repeat("a", 4096), false},
		{"unknown type", "other", strings.Repeat("a", 20), true},
		{"too short", domain.SupportRequestUpdateExisting, strings.Repeat("a", 19), true},
		{"too long", domain.SupportRequestNewInstitution, strings.Repeat("a", 4097), true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateSupportRequest(test.requestType, test.details)
			if test.wantErr && !errors.Is(err, ErrInvalidSupportRequest) {
				t.Fatalf("expected ErrInvalidSupportRequest, got %v", err)
			}
			if !test.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
