package database

import (
	"net/url"
	"testing"

	"github.com/J0es1ick/Scheduler/internal/config"
)

func TestConnectionStringEscapesCredentialsAndSupportsIPv6(t *testing.T) {
	raw := connectionString(config.DatabaseConfig{
		Host:     "::1",
		Port:     "5432",
		User:     "schedule user",
		Password: "p@ss:word",
		Name:     "scheduler",
		SSLMode:  "verify-full",
	})
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	password, _ := parsed.User.Password()
	if parsed.Host != "[::1]:5432" || parsed.User.Username() != "schedule user" || password != "p@ss:word" {
		t.Fatalf("connection string did not preserve address or credentials: %q", raw)
	}
	if parsed.Query().Get("sslmode") != "verify-full" {
		t.Fatalf("sslmode = %q", parsed.Query().Get("sslmode"))
	}
}
