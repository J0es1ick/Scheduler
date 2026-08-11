package admin

import (
	"encoding/json"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
)

type AdminIdentity struct {
	ID         string `json:"id" db:"id"`
	Name       string `json:"name" db:"name"`
	AuthMethod string `json:"auth_method" db:"auth_method"`
	Role       string `json:"role" db:"role"`
	CSRFToken  string `json:"csrf_token,omitempty" db:"csrf_token"`
}

type Dashboard struct {
	Stats        DashboardStats        `json:"stats"`
	Sources      []SourceView          `json:"sources"`
	RecentLogs   []ParseLogView        `json:"recent_logs"`
	Trend        []TrendPoint          `json:"trend"`
	Universities []UniversityBreakdown `json:"universities"`
	Operations   OperationalHealth     `json:"operations"`
}

type DashboardStats struct {
	Universities  int     `json:"universities" db:"universities"`
	Groups        int     `json:"groups" db:"groups"`
	Lessons       int     `json:"lessons" db:"lessons"`
	Users         int     `json:"users" db:"users"`
	Subscriptions int     `json:"subscriptions" db:"subscriptions"`
	SuccessRate   float64 `json:"success_rate" db:"success_rate"`
}

type SourceView struct {
	ID                        string     `json:"id" db:"id"`
	UniversityID              string     `json:"university_id" db:"university_id"`
	UniversityName            string     `json:"university_name" db:"university_name"`
	UniversityFullName        string     `json:"university_full_name" db:"university_full_name"`
	ScheduleURL               string     `json:"schedule_url" db:"schedule_url"`
	AdapterType               string     `json:"adapter_type" db:"adapter_type"`
	LifecycleStatus           string     `json:"lifecycle_status" db:"lifecycle_status"`
	ArchivedAt                *time.Time `json:"archived_at" db:"archived_at"`
	AllowEmpty                bool       `json:"allow_empty" db:"allow_empty"`
	InsecureTransport         bool       `json:"insecure_transport" db:"insecure_transport"`
	IsEnabled                 bool       `json:"is_enabled" db:"is_enabled"`
	UpdateInterval            int        `json:"update_interval" db:"update_interval"`
	LastRunAt                 *time.Time `json:"last_run_at" db:"last_run_at"`
	LastSuccessAt             *time.Time `json:"last_success_at" db:"last_success_at"`
	NextRunAt                 *time.Time `json:"next_run_at"`
	LastError                 string     `json:"last_error" db:"last_error"`
	ConsecutiveFailures       int        `json:"consecutive_failures" db:"consecutive_failures"`
	NextRetryAt               *time.Time `json:"next_retry_at" db:"next_retry_at"`
	CurrentSnapshotID         string     `json:"current_snapshot_id" db:"current_snapshot_id"`
	QuarantinedCount          int        `json:"quarantined_count" db:"quarantined_count"`
	LatestStatus              string     `json:"latest_status" db:"latest_status"`
	LatestStartedAt           *time.Time `json:"latest_started_at" db:"latest_started_at"`
	LatestFinishedAt          *time.Time `json:"latest_finished_at" db:"latest_finished_at"`
	LatestRecords             int        `json:"latest_records" db:"latest_records"`
	GroupCount                int        `json:"group_count" db:"group_count"`
	LessonCount               int        `json:"lesson_count" db:"lesson_count"`
	DiagnosticID              string     `json:"diagnostic_id" db:"diagnostic_id"`
	DiagnosticCategory        string     `json:"diagnostic_category" db:"diagnostic_category"`
	DiagnosticSummary         string     `json:"diagnostic_summary" db:"diagnostic_summary"`
	DiagnosticGroupID         string     `json:"diagnostic_group_id" db:"diagnostic_group_id"`
	DiagnosticHTTPStatus      int        `json:"diagnostic_http_status" db:"diagnostic_http_status"`
	DiagnosticContentType     string     `json:"diagnostic_content_type" db:"diagnostic_content_type"`
	DiagnosticResponseSize    int        `json:"diagnostic_response_size" db:"diagnostic_response_size"`
	DiagnosticResponseSHA256  string     `json:"diagnostic_response_sha256" db:"diagnostic_response_sha256"`
	DiagnosticResponsePreview string     `json:"diagnostic_response_preview" db:"diagnostic_response_preview"`
	DiagnosticOccurrences     int        `json:"diagnostic_occurrences" db:"diagnostic_occurrences"`
	DiagnosticCreatedAt       *time.Time `json:"diagnostic_created_at" db:"diagnostic_created_at"`
	Running                   bool       `json:"running"`
	Health                    string     `json:"health"`
}

type OperationalHealth struct {
	Status                string              `json:"status"`
	Database              bool                `json:"database"`
	SourcesTotal          int                 `json:"sources_total"`
	SourcesHealthy        int                 `json:"sources_healthy"`
	SourcesRunning        int                 `json:"sources_running"`
	SourcesStale          int                 `json:"sources_stale"`
	SourcesError          int                 `json:"sources_error"`
	SourcesQuarantined    int                 `json:"sources_quarantined"`
	SourcesDisabled       int                 `json:"sources_disabled"`
	PendingNotifications  int                 `json:"pending_notifications" db:"pending_notifications"`
	FailedNotifications   int                 `json:"failed_notifications" db:"failed_notifications"`
	PendingOutbox         int                 `json:"pending_outbox" db:"pending_outbox"`
	FailedOutbox          int                 `json:"failed_outbox" db:"failed_outbox"`
	PendingConnectorRuns  int                 `json:"pending_connector_runs" db:"pending_connector_runs"`
	FailedConnectorRuns   int                 `json:"failed_connector_runs" db:"failed_connector_runs"`
	OldestPendingSeconds  int64               `json:"oldest_pending_seconds" db:"oldest_pending_seconds"`
	LastSuccessfulParseAt *time.Time          `json:"last_successful_parse_at" db:"last_successful_parse_at"`
	ReminderWorker        domain.WorkerStatus `json:"reminder_worker"`
	CheckedAt             time.Time           `json:"checked_at"`
}

type ParseLogView struct {
	ID             string     `json:"id" db:"id"`
	DataSourceID   string     `json:"data_source_id" db:"data_source_id"`
	UniversityName string     `json:"university_name" db:"university_name"`
	StartedAt      time.Time  `json:"started_at" db:"started_at"`
	FinishedAt     *time.Time `json:"finished_at" db:"finished_at"`
	Status         string     `json:"status" db:"status"`
	RecordsFetched int        `json:"records_fetched" db:"records_fetched"`
	ErrorMessage   string     `json:"error_message" db:"error_message"`
	DurationMS     int64      `json:"duration_ms" db:"duration_ms"`
}

type TrendPoint struct {
	Date    time.Time `json:"date" db:"date"`
	Records int       `json:"records" db:"records"`
	Success int       `json:"success" db:"success"`
	Failed  int       `json:"failed" db:"failed"`
}

type UniversityBreakdown struct {
	ID      string `json:"id" db:"id"`
	Name    string `json:"name" db:"name"`
	Groups  int    `json:"groups" db:"groups"`
	Lessons int    `json:"lessons" db:"lessons"`
}

type UniversityOption struct {
	ID          string `json:"id" db:"id"`
	Name        string `json:"name" db:"name"`
	FullName    string `json:"full_name" db:"full_name"`
	ScheduleURL string `json:"schedule_url" db:"schedule_url"`
	IsActive    bool   `json:"is_active" db:"is_active"`
}

type GroupView struct {
	ID             string    `json:"id" db:"id"`
	Name           string    `json:"name" db:"name"`
	UniversityID   string    `json:"university_id" db:"university_id"`
	UniversityName string    `json:"university_name" db:"university_name"`
	IsActive       bool      `json:"is_active" db:"is_active"`
	LessonCount    int       `json:"lesson_count" db:"lesson_count"`
	UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
}

type LessonView struct {
	ID             string     `json:"id" db:"id"`
	UniversityName string     `json:"university_name" db:"university_name"`
	GroupID        string     `json:"group_id" db:"group_id"`
	GroupName      string     `json:"group_name" db:"group_name"`
	Subject        string     `json:"subject" db:"subject"`
	Type           string     `json:"type" db:"type"`
	Teacher        string     `json:"teacher" db:"teacher"`
	Room           string     `json:"room" db:"room"`
	DayOfWeek      int        `json:"day_of_week" db:"day_of_week"`
	SpecialDate    *time.Time `json:"special_date" db:"special_date"`
	TimeStart      string     `json:"time_start" db:"time_start"`
	TimeEnd        string     `json:"time_end" db:"time_end"`
	WeekType       string     `json:"week_type" db:"week_type"`
	Subgroup       int        `json:"subgroup" db:"subgroup"`
	ValidFrom      *time.Time `json:"valid_from" db:"valid_from"`
	ValidTo        *time.Time `json:"valid_to" db:"valid_to"`
}

type UserView struct {
	ID                   string    `json:"id" db:"id"`
	Username             string    `json:"username" db:"username"`
	IsAdmin              bool      `json:"is_admin" db:"is_admin"`
	AdminRole            string    `json:"admin_role" db:"admin_role"`
	Subscriptions        int       `json:"subscriptions" db:"subscriptions"`
	DefaultGroupID       string    `json:"default_group_id" db:"default_group_id"`
	DefaultGroupName     string    `json:"default_group_name" db:"default_group_name"`
	NotificationsEnabled bool      `json:"notifications_enabled" db:"notifications_enabled"`
	CreatedAt            time.Time `json:"created_at" db:"created_at"`
	UpdatedAt            time.Time `json:"updated_at" db:"updated_at"`
}

type SupportRequestView struct {
	ID          string     `json:"id" db:"id"`
	UserID      string     `json:"user_id" db:"user_id"`
	Username    string     `json:"username" db:"username"`
	RequestType string     `json:"request_type" db:"request_type"`
	Details     string     `json:"details" db:"details"`
	Status      string     `json:"status" db:"status"`
	ReviewNote  string     `json:"review_note" db:"review_note"`
	ReviewedBy  string     `json:"reviewed_by" db:"reviewed_by"`
	ReviewedAt  *time.Time `json:"reviewed_at" db:"reviewed_at"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
}

type AuditLogView struct {
	ID         string         `json:"id" db:"id"`
	ActorID    string         `json:"actor_id" db:"actor_id"`
	ActorName  string         `json:"actor_name" db:"actor_name"`
	Action     string         `json:"action" db:"action"`
	ObjectType string         `json:"object_type" db:"object_type"`
	ObjectID   string         `json:"object_id" db:"object_id"`
	Details    map[string]any `json:"details" db:"-"`
	DetailsRaw []byte         `json:"-" db:"details"`
	IPAddress  string         `json:"ip_address" db:"ip_address"`
	CreatedAt  time.Time      `json:"created_at" db:"created_at"`
}

type Pagination struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
	Total    int `json:"total"`
}

type Page[T any] struct {
	Items      []T        `json:"items"`
	Pagination Pagination `json:"pagination"`
}

func (p Page[T]) MarshalJSON() ([]byte, error) {
	type pageJSON Page[T]
	if p.Items == nil {
		p.Items = []T{}
	}
	return json.Marshal(pageJSON(p))
}

type EditorGroup struct {
	ID             string    `json:"id" db:"id"`
	Name           string    `json:"name" db:"name"`
	UniversityID   string    `json:"university_id" db:"university_id"`
	UniversityName string    `json:"university_name" db:"university_name"`
	UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
}

type SemesterOption struct {
	ID        string    `json:"id" db:"id"`
	Name      string    `json:"name" db:"name"`
	StartDate time.Time `json:"start_date" db:"start_date"`
	EndDate   time.Time `json:"end_date" db:"end_date"`
}

type EditorLesson struct {
	ID           string     `json:"id" db:"id"`
	UniversityID string     `json:"university_id" db:"university_id"`
	SemesterID   string     `json:"semester_id" db:"semester_id"`
	DayOfWeek    int        `json:"day_of_week" db:"day_of_week"`
	SpecialDate  *time.Time `json:"special_date" db:"special_date"`
	TimeStart    string     `json:"time_start" db:"time_start"`
	TimeEnd      string     `json:"time_end" db:"time_end"`
	WeekType     string     `json:"week_type" db:"week_type"`
	Subject      string     `json:"subject" db:"subject"`
	Type         string     `json:"type" db:"type"`
	Teacher      string     `json:"teacher" db:"teacher"`
	Room         string     `json:"room" db:"room"`
	GroupID      string     `json:"group_id" db:"group_id"`
	Subgroup     int        `json:"subgroup" db:"subgroup"`
	ValidFrom    *time.Time `json:"valid_from" db:"valid_from"`
	ValidTo      *time.Time `json:"valid_to" db:"valid_to"`
	UpdatedAt    time.Time  `json:"updated_at" db:"updated_at"`
	Origin       string     `json:"origin" db:"origin"`
	BaseLessonID *string    `json:"base_lesson_id" db:"base_lesson_id"`
	Version      int64      `json:"version" db:"version"`
	Deleted      bool       `json:"deleted" db:"is_deleted"`
}

type EditorSchedule struct {
	Group          EditorGroup      `json:"group"`
	Semesters      []SemesterOption `json:"semesters"`
	Lessons        []EditorLesson   `json:"lessons"`
	DeletedLessons []EditorLesson   `json:"deleted_lessons"`
}

type LessonMutation struct {
	GroupID     string
	SemesterID  string
	DayOfWeek   int
	SpecialDate *time.Time
	TimeStart   string
	TimeEnd     string
	WeekType    string
	Subject     string
	Type        string
	Teacher     string
	Room        string
	Subgroup    int
	ValidFrom   *time.Time
	ValidTo     *time.Time
}
