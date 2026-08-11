package database

import (
	"context"
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
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Database.ConnectTimeoutSeconds)*time.Second)
	defer cancel()
	db, err := sqlx.ConnectContext(ctx, "pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("database: connect: %w", err)
	}

	db.SetMaxOpenConns(cfg.Database.MaxOpenConnections)
	db.SetMaxIdleConns(cfg.Database.MaxIdleConnections)
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
	query.Set("connect_timeout", fmt.Sprintf("%d", cfg.ConnectTimeoutSeconds))
	query.Set("statement_timeout", fmt.Sprintf("%d", cfg.StatementTimeoutSeconds*1000))
	query.Set("application_name", "scheduler")
	dsn.RawQuery = query.Encode()
	return dsn.String()
}

func (d *Database) Close() error {
	return d.DB.Close()
}
