package domain

import "time"

type University struct {
	ID          string    `db:"id"`           // slug: "igktu"
	Name        string    `db:"name"`         // "ИГХТУ"
	FullName    string    `db:"full_name"`    // "Ивановский государственный..."
	ScheduleURL string    `db:"schedule_url"` // откуда парсить расписание
	IsActive    bool      `db:"is_active"`    // включить/выключить без деплоя
	CreatedAt   time.Time `db:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"`
}
