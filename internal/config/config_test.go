package config

import "testing"

func TestInitConfigReadsEnvironmentWithoutDotEnv(t *testing.T) {
	t.Setenv("BOT_TOKEN", "test-token")
	t.Setenv("DATABASE_HOST", "postgres")
	t.Setenv("DATABASE_PORT", "5432")
	t.Setenv("DATABASE_USER", "scheduler")
	t.Setenv("DATABASE_PASSWORD", "secret")
	t.Setenv("DATABASE_NAME", "scheduler")
	t.Setenv("ADMIN_PORT", "8080")
	t.Setenv("ADMIN_ACCESS_TOKEN", "admin-secret")
	t.Setenv("ADMIN_PUBLIC_URL", "https://admin.example.test")

	cfg, err := InitConfig()
	if err != nil {
		t.Fatalf("init config from environment: %v", err)
	}
	if cfg.Database.Host != "postgres" || cfg.Database.Port != "5432" {
		t.Fatalf("database address = %s:%s", cfg.Database.Host, cfg.Database.Port)
	}
	if cfg.Admin.Port != "8080" || cfg.Admin.AccessToken != "admin-secret" {
		t.Fatalf("admin config = %+v", cfg.Admin)
	}
}
