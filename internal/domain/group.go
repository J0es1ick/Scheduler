package domain

import "time"

type Group struct {
	ID               string    `db:"id"`
	UniversityID     string    `db:"university_id"`
	Name             string    `db:"name"`
	IsActive         bool      `db:"is_active"`
	SourceActive     bool      `db:"source_active"`
	ManuallyDisabled bool      `db:"manually_disabled"`
	CreatedAt        time.Time `db:"created_at"`
	UpdatedAt        time.Time `db:"updated_at"`
}

type GroupSourceIdentityMapping struct {
	DataSourceID    string    `db:"data_source_id" json:"data_source_id"`
	ExternalGroupID string    `db:"external_group_id" json:"external_group_id"`
	GroupID         string    `db:"group_id" json:"group_id"`
	ExpectedName    string    `db:"expected_name" json:"expected_name"`
	CreatedAt       time.Time `db:"created_at" json:"created_at"`
	UpdatedAt       time.Time `db:"updated_at" json:"updated_at"`
}

type GroupIdentityConflict struct {
	ID                string     `db:"id" json:"id"`
	DataSourceID      string     `db:"data_source_id" json:"data_source_id"`
	UniversityID      string     `db:"university_id" json:"university_id"`
	ExternalGroupID   string     `db:"external_group_id" json:"external_group_id"`
	ExistingGroupID   string     `db:"existing_group_id" json:"existing_group_id"`
	ExistingName      string     `db:"existing_name" json:"existing_name"`
	IncomingName      string     `db:"incoming_name" json:"incoming_name"`
	Status            string     `db:"status" json:"status"`
	Resolution        string     `db:"resolution" json:"resolution"`
	ResolvedGroupID   *string    `db:"resolved_group_id" json:"resolved_group_id,omitempty"`
	ResolvedBy        string     `db:"resolved_by" json:"resolved_by"`
	FirstSeenAt       time.Time  `db:"first_seen_at" json:"first_seen_at"`
	LastSeenAt        time.Time  `db:"last_seen_at" json:"last_seen_at"`
	ResolvedAt        *time.Time `db:"resolved_at" json:"resolved_at,omitempty"`
	Occurrences       int        `db:"occurrences" json:"occurrences"`
	SubscriptionCount int        `db:"subscription_count" json:"subscription_count"`
	DefaultGroupCount int        `db:"default_group_count" json:"default_group_count"`
	ChatCount         int        `db:"chat_count" json:"chat_count"`
	LessonCount       int        `db:"lesson_count" json:"lesson_count"`
}
