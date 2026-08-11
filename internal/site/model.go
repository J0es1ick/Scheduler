package site

import "time"

type PublicInfo struct {
	Universities    int                  `json:"universities" db:"universities"`
	Groups          int                  `json:"groups" db:"groups"`
	Lessons         int                  `json:"lessons" db:"lessons"`
	Users           int                  `json:"users" db:"users"`
	Subscriptions   int                  `json:"subscriptions" db:"subscriptions"`
	UniversityNames []string             `json:"university_names"`
	Sources         []PublicSourceStatus `json:"sources"`
	ProjectURL      string               `json:"project_url"`
	BotURL          string               `json:"bot_url"`
	UpdatedAt       time.Time            `json:"updated_at"`
}

type PublicSourceStatus struct {
	UniversityName string     `json:"university_name" db:"university_name"`
	ScheduleURL    string     `json:"schedule_url" db:"schedule_url"`
	State          string     `json:"state" db:"state"`
	Secure         bool       `json:"secure" db:"secure"`
	LastSuccessAt  *time.Time `json:"last_success_at" db:"last_success_at"`
}
