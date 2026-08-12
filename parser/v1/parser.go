package v1

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	connector "github.com/J0es1ick/Scheduler/connector/v1"
)

const ContractVersion = "1.0"

var parserIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}$`)

type Manifest struct {
	ContractVersion string                `json:"contract_version"`
	ParserID        string                `json:"parser_id"`
	Version         string                `json:"version"`
	DisplayName     string                `json:"display_name"`
	Description     string                `json:"description"`
	Institution     connector.Institution `json:"institution"`
	MaintainerName  string                `json:"maintainer_name,omitempty"`
	MaintainerURL   string                `json:"maintainer_url,omitempty"`
	UpdateInterval  time.Duration         `json:"-"`
	UpdateSeconds   int                   `json:"update_interval"`
}

type Group struct {
	ExternalID string            `json:"external_id"`
	Name       string            `json:"name"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type Lesson = connector.Lesson

type Parser interface {
	Manifest() Manifest
	FetchGroups(ctx context.Context) ([]Group, error)
	FetchSchedule(ctx context.Context, externalGroupID string) ([]Lesson, error)
}

type Factory func() Parser

func ValidateManifest(manifest Manifest) error {
	if manifest.ContractVersion != ContractVersion {
		return fmt.Errorf("managed parser contract_version must be %q", ContractVersion)
	}
	if !parserIDPattern.MatchString(strings.TrimSpace(manifest.ParserID)) {
		return fmt.Errorf("managed parser_id %q is invalid", manifest.ParserID)
	}
	if strings.TrimSpace(manifest.Version) == "" || strings.TrimSpace(manifest.DisplayName) == "" {
		return fmt.Errorf("managed parser version and display_name are required")
	}
	if strings.TrimSpace(manifest.Institution.ExternalID) == "" || strings.TrimSpace(manifest.Institution.Name) == "" {
		return fmt.Errorf("managed parser institution identity is required")
	}
	if _, err := time.LoadLocation(manifest.Institution.Timezone); err != nil {
		return fmt.Errorf("managed parser institution timezone: %w", err)
	}
	interval := manifest.UpdateInterval
	if interval == 0 && manifest.UpdateSeconds > 0 {
		interval = time.Duration(manifest.UpdateSeconds) * time.Second
	}
	if interval < 5*time.Minute || interval > 7*24*time.Hour {
		return fmt.Errorf("managed parser update interval must be between 5 minutes and 7 days")
	}
	return nil
}

func NormalizeManifest(manifest Manifest) Manifest {
	if manifest.UpdateInterval == 0 && manifest.UpdateSeconds > 0 {
		manifest.UpdateInterval = time.Duration(manifest.UpdateSeconds) * time.Second
	}
	if manifest.UpdateSeconds == 0 && manifest.UpdateInterval > 0 {
		manifest.UpdateSeconds = int(manifest.UpdateInterval / time.Second)
	}
	return manifest
}
