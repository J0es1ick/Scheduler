package site

import "time"

type PublicInfo struct {
	Universities    int       `json:"universities" db:"universities"`
	Groups          int       `json:"groups" db:"groups"`
	Lessons         int       `json:"lessons" db:"lessons"`
	Users           int       `json:"users" db:"users"`
	Subscriptions   int       `json:"subscriptions" db:"subscriptions"`
	UniversityNames []string  `json:"university_names"`
	ProjectURL      string    `json:"project_url"`
	BotURL          string    `json:"bot_url"`
	UpdatedAt       time.Time `json:"updated_at"`
}
