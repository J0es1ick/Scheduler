package config

import "testing"

func TestInitConfigReadsEnvironmentWithoutDotEnv(t *testing.T) {
	t.Setenv("BOT_TOKEN", "test-token")
	t.Setenv("BOT_USERNAME", "schedule_free_bot")
	t.Setenv("DATABASE_HOST", "postgres")
	t.Setenv("DATABASE_PORT", "5432")
	t.Setenv("DATABASE_USER", "scheduler")
	t.Setenv("DATABASE_PASSWORD", "secret")
	t.Setenv("DATABASE_NAME", "scheduler")
	t.Setenv("DATABASE_SSLMODE", "verify-full")
	t.Setenv("ADMIN_PORT", "18080")
	t.Setenv("ADMIN_ACCESS_TOKEN", "admin-secret")
	t.Setenv("ADMIN_ACCESS_LOGIN_ENABLED", "true")
	t.Setenv("ADMIN_COOKIE_SECURE", "true")
	t.Setenv("ADMIN_PUBLIC_URL", "https://admin.example.test")
	t.Setenv("ADMIN_TRUSTED_PROXY_CIDRS", "10.0.0.0/8")
	t.Setenv("ADMIN_METRICS_TOKEN", "metrics-secret")

	cfg, err := InitConfig()
	if err != nil {
		t.Fatalf("init config from environment: %v", err)
	}
	if cfg.Database.Host != "postgres" || cfg.Database.Port != "5432" {
		t.Fatalf("database address = %s:%s", cfg.Database.Host, cfg.Database.Port)
	}
	if cfg.Database.SSLMode != "verify-full" {
		t.Fatalf("database SSL mode = %q", cfg.Database.SSLMode)
	}
	if cfg.Admin.Port != "18080" || cfg.Admin.AccessToken != "admin-secret" || !cfg.Admin.AccessKeyLoginEnabled || !cfg.Admin.CookieSecure {
		t.Fatalf("admin config = %+v", cfg.Admin)
	}
	if cfg.ProjectURL == "" || cfg.BotPublicURL == "" {
		t.Fatalf("public links are not configured: project=%q bot=%q", cfg.ProjectURL, cfg.BotPublicURL)
	}
	if cfg.BotUsername != "schedule_free_bot" {
		t.Fatalf("bot username = %q", cfg.BotUsername)
	}
	if cfg.BotMaxConcurrentHandlers != 32 || cfg.BotMaxPendingPerSender != 8 {
		t.Fatalf(
			"bot handler limits = concurrent:%d pending-per-sender:%d",
			cfg.BotMaxConcurrentHandlers,
			cfg.BotMaxPendingPerSender,
		)
	}
}

func TestInitConfigRejectsUnknownDatabaseSSLMode(t *testing.T) {
	t.Setenv("BOT_TOKEN", "test-token")
	t.Setenv("DATABASE_HOST", "postgres")
	t.Setenv("DATABASE_PORT", "5432")
	t.Setenv("DATABASE_USER", "scheduler")
	t.Setenv("DATABASE_PASSWORD", "secret")
	t.Setenv("DATABASE_NAME", "scheduler")
	t.Setenv("DATABASE_SSLMODE", "unsafe")

	if _, err := InitConfig(); err == nil {
		t.Fatal("unknown database SSL mode must be rejected")
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

func TestInitAdminConfigRequiresMetricsToken(t *testing.T) {
	t.Setenv("BOT_TOKEN", "test-token")
	t.Setenv("DATABASE_HOST", "postgres")
	t.Setenv("DATABASE_PORT", "5432")
	t.Setenv("DATABASE_USER", "scheduler")
	t.Setenv("DATABASE_PASSWORD", "secret")
	t.Setenv("DATABASE_NAME", "scheduler")
	t.Setenv("ADMIN_METRICS_TOKEN", "")
	if _, err := InitAdminConfig(); err == nil {
		t.Fatal("admin config without metrics token must be rejected")
	}
}

func TestInitConfigRejectsNegativeSenderQueueLimit(t *testing.T) {
	t.Setenv("BOT_TOKEN", "test-token")
	t.Setenv("DATABASE_HOST", "postgres")
	t.Setenv("DATABASE_PORT", "5432")
	t.Setenv("DATABASE_USER", "scheduler")
	t.Setenv("DATABASE_PASSWORD", "secret")
	t.Setenv("DATABASE_NAME", "scheduler")
	t.Setenv("BOT_MAX_PENDING_PER_SENDER", "-1")

	if _, err := InitConfig(); err == nil {
		t.Fatal("negative per-sender queue limit must be rejected")
	}
}
