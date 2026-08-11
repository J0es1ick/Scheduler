package v1

import "time"

const SchemaVersion = "1.0"

const (
	RecurrenceEvery = "every"
	RecurrenceOdd   = "odd"
	RecurrenceEven  = "even"
	RecurrenceDate  = "date"
	RecurrenceCycle = "cycle"
)

type Snapshot struct {
	SchemaVersion string            `json:"schema_version"`
	SnapshotID    string            `json:"snapshot_id"`
	GeneratedAt   time.Time         `json:"generated_at"`
	Institution   Institution       `json:"institution"`
	Term          Term              `json:"term"`
	Groups        []Group           `json:"groups"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

type Institution struct {
	ExternalID  string `json:"external_id"`
	Name        string `json:"name"`
	FullName    string `json:"full_name,omitempty"`
	ScheduleURL string `json:"schedule_url,omitempty"`
	Timezone    string `json:"timezone"`
	Locale      string `json:"locale,omitempty"`
}

type Term struct {
	ExternalID   string `json:"external_id"`
	Name         string `json:"name"`
	AcademicYear string `json:"academic_year,omitempty"`
	StartsOn     string `json:"starts_on"`
	EndsOn       string `json:"ends_on"`
}

type Group struct {
	ExternalID string            `json:"external_id"`
	Name       string            `json:"name"`
	Lessons    []Lesson          `json:"lessons"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type Lesson struct {
	ExternalID string            `json:"external_id"`
	Subject    string            `json:"subject"`
	Type       string            `json:"type"`
	Teachers   []string          `json:"teachers,omitempty"`
	Rooms      []string          `json:"rooms,omitempty"`
	Subgroup   int               `json:"subgroup,omitempty"`
	Schedule   Schedule          `json:"schedule"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type Schedule struct {
	DayOfWeek  int        `json:"day_of_week,omitempty"`
	Date       string     `json:"date,omitempty"`
	StartsAt   string     `json:"starts_at"`
	EndsAt     string     `json:"ends_at"`
	Recurrence Recurrence `json:"recurrence"`
}

type Recurrence struct {
	Kind        string `json:"kind"`
	ValidFrom   string `json:"valid_from,omitempty"`
	ValidTo     string `json:"valid_to,omitempty"`
	CycleLength int    `json:"cycle_length,omitempty"`
	CycleWeeks  []int  `json:"cycle_weeks,omitempty"`
}

type SubmissionResponse struct {
	RunID      string `json:"run_id"`
	Status     string `json:"status"`
	SnapshotID string `json:"snapshot_id"`
	StatusURL  string `json:"status_url"`
	Duplicate  bool   `json:"duplicate,omitempty"`
}

type RunStatus struct {
	RunID            string     `json:"run_id"`
	ConnectorID      string     `json:"connector_id"`
	ExternalSnapshot string     `json:"external_snapshot_id"`
	Status           string     `json:"status"`
	GroupCount       int        `json:"group_count"`
	LessonCount      int        `json:"lesson_count"`
	Error            string     `json:"error,omitempty"`
	ParserSnapshotID string     `json:"parser_snapshot_id,omitempty"`
	ReceivedAt       time.Time  `json:"received_at"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
}

type HeartbeatResponse struct {
	Status      string    `json:"status"`
	ConnectorID string    `json:"connector_id"`
	ServerTime  time.Time `json:"server_time"`
}
