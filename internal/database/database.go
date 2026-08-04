package database

import (
	"fmt"
	"net"
	"net/url"
	"time"

	"github.com/J0es1ick/Scheduler/internal/config"
	"github.com/jmoiron/sqlx"
)

type Database struct {
	DB *sqlx.DB
}

func NewDatabase(cfg *config.Config) (*Database, error) {
	dsn := connectionString(cfg.Database)

	db, err := sqlx.Connect("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("database: connect: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(2 * time.Minute)

	return &Database{DB: db}, nil
}

func connectionString(cfg config.DatabaseConfig) string {
	dsn := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(cfg.User, cfg.Password),
		Host:   net.JoinHostPort(cfg.Host, cfg.Port),
		Path:   "/" + cfg.Name,
	}
	query := dsn.Query()
	query.Set("sslmode", cfg.SSLMode)
	dsn.RawQuery = query.Encode()
	return dsn.String()
}

func (d *Database) Close() error {
	return d.DB.Close()
}
