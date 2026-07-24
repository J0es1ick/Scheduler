package config

import "testing"

func TestInitConfigReadsEnvironmentWithoutDotEnv(t *testing.T) {
	t.Setenv("BOT_TOKEN", "test-token")
	t.Setenv("DATABASE_HOST", "postgres")
	t.Setenv("DATABASE_PORT", "5432")
	t.Setenv("DATABASE_USER", "scheduler")
	t.Setenv("DATABASE_PASSWORD", "secret")
	t.Setenv("DATABASE_NAME", "scheduler")
	t.Setenv("ADMIN_PORT", "18080")
	t.Setenv("ADMIN_ACCESS_TOKEN", "admin-secret")
	t.Setenv("ADMIN_PUBLIC_URL", "https://admin.example.test")

	cfg, err := InitConfig()
	if err != nil {
		t.Fatalf("init config from environment: %v", err)
	}
	if cfg.Database.Host != "postgres" || cfg.Database.Port != "5432" {
		t.Fatalf("database address = %s:%s", cfg.Database.Host, cfg.Database.Port)
	}
	if cfg.Admin.Port != "18080" || cfg.Admin.AccessToken != "admin-secret" {
		t.Fatalf("admin config = %+v", cfg.Admin)
	}
	if cfg.ProjectURL == "" || cfg.BotPublicURL == "" {
		t.Fatalf("public links are not configured: project=%q bot=%q", cfg.ProjectURL, cfg.BotPublicURL)
	}
}

func TestInitSiteConfigDoesNotRequireBotToken(t *testing.T) {
	t.Setenv("BOT_TOKEN", "")
	t.Setenv("DATABASE_HOST", "postgres")
	t.Setenv("DATABASE_PORT", "5432")
	t.Setenv("DATABASE_USER", "scheduler")
	t.Setenv("DATABASE_PASSWORD", "secret")
	t.Setenv("DATABASE_NAME", "scheduler")
	t.Setenv("SITE_PORT", "18081")

	cfg, err := InitSiteConfig()
	if err != nil {
		t.Fatalf("init site config without bot token: %v", err)
	}
	if cfg.Site.Port != "18081" {
		t.Fatalf("site port = %q", cfg.Site.Port)
	}
}
