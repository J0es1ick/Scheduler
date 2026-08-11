package domain

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

const (
	IntegrationModeManagedParser = "managed_parser"
	IntegrationModeDeclarative   = "declarative_pull"
	IntegrationModeExternalPush  = "external_push"

	ConnectorStatusDraft         = "draft"
	ConnectorStatusTesting       = "testing"
	ConnectorStatusPendingReview = "pending_review"
	ConnectorStatusActive        = "active"
	ConnectorStatusSuspended     = "suspended"
	ConnectorStatusArchived      = "archived"

	IngestionStatusReceived    = "received"
	IngestionStatusProcessing  = "processing"
	IngestionStatusStaged      = "staged"
	IngestionStatusQuarantined = "quarantined"
	IngestionStatusPublished   = "published"
	IngestionStatusRejected    = "rejected"
	IngestionStatusFailed      = "failed"
)

type ConnectorClient struct {
	ID                 string              `db:"id" json:"id"`
	DataSourceID       string              `db:"data_source_id" json:"data_source_id"`
	UniversityID       string              `db:"university_id" json:"university_id"`
	UniversityName     string              `db:"university_name" json:"university_name"`
	DisplayName        string              `db:"display_name" json:"display_name"`
	IntegrationMode    string              `db:"integration_mode" json:"integration_mode"`
	ParserID           string              `db:"parser_id" json:"parser_id"`
	Description        string              `db:"description" json:"description"`
	MaintainerName     string              `db:"maintainer_name" json:"maintainer_name"`
	MaintainerURL      string              `db:"maintainer_url" json:"maintainer_url"`
	KeyID              string              `db:"key_id" json:"key_id"`
	PublicKey          []byte              `db:"public_key" json:"-"`
	Status             string              `db:"status" json:"status"`
	RateLimitPerMinute int                 `db:"rate_limit_per_minute" json:"rate_limit_per_minute"`
	MaxPayloadBytes    int                 `db:"max_payload_bytes" json:"max_payload_bytes"`
	LastSeenAt         *time.Time          `db:"last_seen_at" json:"last_seen_at"`
	LastSnapshotAt     *time.Time          `db:"last_snapshot_at" json:"last_snapshot_at"`
	CreatedBy          string              `db:"created_by" json:"created_by"`
	CreatedAt          time.Time           `db:"created_at" json:"created_at"`
	UpdatedAt          time.Time           `db:"updated_at" json:"updated_at"`
	QualityPolicy      SourceQualityPolicy `db:"quality_policy" json:"quality_policy"`
}

type ConnectorIngestionRun struct {
	ID                 string          `db:"id" json:"run_id"`
	ConnectorID        string          `db:"connector_id" json:"connector_id"`
	DataSourceID       string          `db:"data_source_id" json:"data_source_id,omitempty"`
	ExternalSnapshotID string          `db:"external_snapshot_id" json:"external_snapshot_id"`
	SchemaVersion      string          `db:"schema_version" json:"schema_version"`
	IdempotencyKey     string          `db:"idempotency_key" json:"-"`
	PayloadSHA256      string          `db:"payload_sha256" json:"payload_sha256"`
	Payload            json.RawMessage `db:"payload" json:"-"`
	Status             string          `db:"status" json:"status"`
	Attempts           int             `db:"attempts" json:"attempts"`
	ErrorMessage       string          `db:"error_message" json:"error,omitempty"`
	ParserSnapshotID   string          `db:"parser_snapshot_id" json:"parser_snapshot_id,omitempty"`
	GroupCount         int             `db:"group_count" json:"group_count"`
	LessonCount        int             `db:"lesson_count" json:"lesson_count"`
	NextAttemptAt      time.Time       `db:"next_attempt_at" json:"next_attempt_at"`
	ClaimedAt          *time.Time      `db:"claimed_at" json:"claimed_at,omitempty"`
	ReceivedAt         time.Time       `db:"received_at" json:"received_at"`
	CompletedAt        *time.Time      `db:"completed_at" json:"completed_at,omitempty"`
}

type SourceQualityPolicy struct {
	AllowEmpty               bool    `json:"allow_empty"`
	MinimumGroups            int     `json:"minimum_groups"`
	MinimumLessons           int     `json:"minimum_lessons"`
	MaximumGroupDropRatio    float64 `json:"maximum_group_drop_ratio"`
	MaximumGroupGrowthRatio  float64 `json:"maximum_group_growth_ratio"`
	MaximumLessonDropRatio   float64 `json:"maximum_lesson_drop_ratio"`
	MaximumLessonGrowthRatio float64 `json:"maximum_lesson_growth_ratio"`
}

func (p *SourceQualityPolicy) Scan(value any) error {
	if value == nil {
		*p = DefaultSourceQualityPolicy()
		return nil
	}
	var raw []byte
	switch typed := value.(type) {
	case []byte:
		raw = typed
	case string:
		raw = []byte(typed)
	default:
		return fmt.Errorf("scan source quality policy: unsupported %T", value)
	}
	if len(raw) == 0 {
		*p = DefaultSourceQualityPolicy()
		return nil
	}
	return json.Unmarshal(raw, p)
}

func (p SourceQualityPolicy) Value() (driver.Value, error) {
	return json.Marshal(p)
}

func DefaultSourceQualityPolicy() SourceQualityPolicy {
	return SourceQualityPolicy{
		MinimumGroups:            1,
		MaximumGroupDropRatio:    0.30,
		MaximumGroupGrowthRatio:  0.80,
		MaximumLessonDropRatio:   0.40,
		MaximumLessonGrowthRatio: 1.00,
	}
}
