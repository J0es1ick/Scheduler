package domain

import (
	"encoding/json"
	"time"
)

type User struct {
	ID                   string    `db:"id" json:"id"`
	Username             string    `db:"username" json:"username"`
	IsAdmin              bool      `db:"is_admin" json:"is_admin"`
	DefaultGroupID       string    `db:"default_group_id" json:"default_group_id"`
	NotificationsEnabled bool      `db:"notifications_enabled" json:"notifications_enabled"`
	ReminderEnabled      bool      `db:"reminder_enabled" json:"reminder_enabled"`
	ReminderMinutes      int       `db:"reminder_minutes" json:"reminder_minutes"`
	QuietHoursEnabled    bool      `db:"quiet_hours_enabled" json:"quiet_hours_enabled"`
	QuietHoursStart      string    `db:"quiet_hours_start" json:"quiet_hours_start"`
	QuietHoursEnd        string    `db:"quiet_hours_end" json:"quiet_hours_end"`
	CreatedAt            time.Time `db:"created_at" json:"created_at"`
	UpdatedAt            time.Time `db:"updated_at" json:"updated_at"`
}

type UserDataExport struct {
	ExportedAt      time.Time               `json:"exported_at"`
	User            User                    `json:"user"`
	Subscriptions   []Subscription          `json:"subscriptions"`
	SupportRequests []SupportRequest        `json:"support_requests"`
	AuditRecords    []PersonalAuditRecord   `json:"audit_records"`
	AdminSessions   []PersonalAdminSession  `json:"admin_sessions"`
	References      []PersonalDataReference `json:"references"`
}

type PersonalAuditRecord struct {
	ID         string          `db:"id" json:"id"`
	ActorName  string          `db:"actor_name" json:"actor_name"`
	Action     string          `db:"action" json:"action"`
	ObjectType string          `db:"object_type" json:"object_type"`
	ObjectID   string          `db:"object_id" json:"object_id"`
	Details    json.RawMessage `db:"details" json:"details"`
	IPAddress  string          `db:"ip_address" json:"ip_address"`
	CreatedAt  time.Time       `db:"created_at" json:"created_at"`
}

type PersonalAdminSession struct {
	Name       string    `db:"name" json:"name"`
	AuthMethod string    `db:"auth_method" json:"auth_method"`
	AdminRole  string    `db:"admin_role" json:"admin_role"`
	ExpiresAt  time.Time `db:"expires_at" json:"expires_at"`
	CreatedAt  time.Time `db:"created_at" json:"created_at"`
	LastSeenAt time.Time `db:"last_seen_at" json:"last_seen_at"`
}

type PersonalDataReference struct {
	Category     string    `db:"category" json:"category"`
	ObjectID     string    `db:"object_id" json:"object_id,omitempty"`
	Relationship string    `db:"relationship" json:"relationship"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
}
