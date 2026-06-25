package database

import (
	"fmt"

	"github.com/J0es1ick/Scheduler/internal/config"
	"github.com/jmoiron/sqlx"
)

type Database struct {
	DB *sqlx.DB
}

func NewDatabase(cfg *config.Config) (*Database, error) {
	connString := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		cfg.Database.User, cfg.Database.Password, cfg.Database.Host, cfg.Database.Port, cfg.Database.Name)

	conn, err := sqlx.Connect("pgx", connString)
	if err != nil {
		return nil, fmt.Errorf("can't connect to pg instance, %v", err)
	}

	return &Database{DB: conn}, nil
}

func (d *Database) Close() error {
	return d.DB.Close()
}