package domain

import "time"

const (
	SnapshotStatusStaged      = "staged"
	SnapshotStatusQuarantined = "quarantined"
	SnapshotStatusApproved    = "approved"
	SnapshotStatusPublished   = "published"
	SnapshotStatusRejected    = "rejected"
)

type SnapshotAnomaly struct {
	Code      string  `json:"code"`
	Message   string  `json:"message"`
	Current   int     `json:"current,omitempty"`
	Candidate int     `json:"candidate,omitempty"`
	Ratio     float64 `json:"ratio,omitempty"`
}

type SnapshotGroup struct {
	ID           string   `json:"id"`
	UniversityID string   `json:"university_id"`
	Name         string   `json:"name"`
	Lessons      []Lesson `json:"lessons"`
}

type SnapshotInstitutionMetadata struct {
	Name        string `json:"name,omitempty"`
	FullName    string `json:"full_name,omitempty"`
	ScheduleURL string `json:"schedule_url,omitempty"`
	Timezone    string `json:"timezone,omitempty"`
	Locale      string `json:"locale,omitempty"`
}

type SnapshotTermMetadata struct {
	ExternalID   string `json:"external_id,omitempty"`
	Name         string `json:"name,omitempty"`
	AcademicYear string `json:"academic_year,omitempty"`
}

type SnapshotMetadata struct {
	Institution SnapshotInstitutionMetadata `json:"institution"`
	Term        SnapshotTermMetadata        `json:"term"`
}

type ScheduleSnapshot struct {
	UniversityID string            `json:"university_id"`
	SemesterID   string            `json:"semester_id"`
	StartDate    time.Time         `json:"start_date"`
	EndDate      time.Time         `json:"end_date"`
	Groups       []SnapshotGroup   `json:"groups"`
	Metadata     *SnapshotMetadata `json:"metadata,omitempty"`
}

type ParserSnapshot struct {
	ID             string            `db:"id" json:"id"`
	DataSourceID   string            `db:"data_source_id" json:"data_source_id"`
	ParseLogID     string            `db:"parse_log_id" json:"parse_log_id"`
	Status         string            `db:"status" json:"status"`
	Publishable    bool              `db:"publishable" json:"publishable"`
	GroupCount     int               `db:"group_count" json:"group_count"`
	LessonCount    int               `db:"lesson_count" json:"lesson_count"`
	AnomalyReasons []SnapshotAnomaly `db:"-" json:"anomaly_reasons"`
	Payload        ScheduleSnapshot  `db:"-" json:"-"`
	ReviewedBy     string            `db:"reviewed_by" json:"reviewed_by"`
	ReviewNote     string            `db:"review_note" json:"review_note"`
	CreatedAt      time.Time         `db:"created_at" json:"created_at"`
	PublishedAt    *time.Time        `db:"published_at" json:"published_at"`
	ReviewedAt     *time.Time        `db:"reviewed_at" json:"reviewed_at"`
}

type SnapshotBaseline struct {
	GroupCount       int               `db:"group_count"`
	LessonCount      int               `db:"lesson_count"`
	LessonsByGroup   map[string]int    `db:"-"`
	CurrentSnapshot  string            `db:"current_snapshot"`
	TrustedSnapshot  *ScheduleSnapshot `db:"-"`
	HasExistingState bool              `db:"-"`
}
