package domain

import "time"

type DataSource struct {
	ID           	string    `db:"id"`
	UniversityID 	string    `db:"university_id"`
	AdapterType  	string    `db:"adapter_type"`
	Config       	string 	  `db:"config"`
	UpdateInterval  int       `db:"update_interval"` // in seconds
	LastRunAt	 	time.Time `db:"last_run_at"`
	LastError   	string    `db:"last_error"`
	CreatedAt    	time.Time `db:"created_at"`
	UpdatedAt    	time.Time `db:"updated_at"`
}