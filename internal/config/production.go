package config

import (
	"crypto/x509"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
)

func ProductionPreflight(getenv func(string) string) error {
	var failures []error
	require := func(ok bool, message string) {
		if !ok {
			failures = append(failures, errors.New(message))
		}
	}
	require(getenv("DEPLOYMENT_ENV") == "production", "DEPLOYMENT_ENV must be production")
	require(getenv("DATABASE_SSLMODE") == "verify-full", "DATABASE_SSLMODE must be verify-full")
	require(getenv("ADMIN_COOKIE_SECURE") == "true", "ADMIN_COOKIE_SECURE must be true")
	require(getenv("ADMIN_ACCESS_LOGIN_ENABLED") == "false", "emergency access-key login must be disabled")
	require(getenv("BOT_TELEGRAM_API_ALLOW_INSECURE") != "true", "insecure Telegram API must be disabled")
	for _, key := range []string{"ADMIN_PUBLIC_URL", "SITE_PUBLIC_URL"} {
		parsed, err := url.Parse(getenv(key))
		require(err == nil && parsed.Scheme == "https" && parsed.Hostname() != "" && parsed.User == nil, key+" must be an absolute HTTPS URL without credentials")
	}
	for _, key := range []string{"BOT_TOKEN", "ADMIN_METRICS_TOKEN"} {
		require(len(getenv(key)) >= 32 && !isPlaceholderSecret(getenv(key)), key+" must contain a real secret of at least 32 characters")
	}
	roleName := regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,62}$`)
	superuser := strings.TrimSpace(getenv("POSTGRES_SUPERUSER"))
	superuserPassword := getenv("POSTGRES_SUPERUSER_PASSWORD")
	require(roleName.MatchString(superuser), "POSTGRES_SUPERUSER must contain a valid PostgreSQL role name")
	require(len(superuserPassword) >= 24 && !isPlaceholderSecret(superuserPassword), "POSTGRES_SUPERUSER_PASSWORD must contain a real secret of at least 24 characters")
	roles, passwords := map[string]bool{superuser: true, "scheduler_public_reader": true}, map[string]bool{superuserPassword: true}
	for _, role := range []string{"MIGRATOR", "BOT", "PARSER", "PRIVACY", "ADMIN", "SITE", "BACKUP", "RESTORE"} {
		userKey, passwordKey := "DATABASE_"+role+"_USER", "DATABASE_"+role+"_PASSWORD"
		user, password := strings.TrimSpace(getenv(userKey)), getenv(passwordKey)
		require(roleName.MatchString(user) && !roles[user], userKey+" must be a valid role distinct from the configured superuser and every reserved role")
		require(len(password) >= 24 && !isPlaceholderSecret(password) && !passwords[password], passwordKey+" must be a distinct secret of at least 24 characters")
		roles[user], passwords[password] = true, true
	}
	require(getenv("DATABASE_HOST") != "" && getenv("DATABASE_NAME") != "", "database host and name are required")
	if err := validatePort("DATABASE_PORT", getenv("DATABASE_PORT")); err != nil {
		failures = append(failures, err)
	}
	ca, err := os.ReadFile(getenv("PGSSLROOTCERT"))
	pool := x509.NewCertPool()
	require(err == nil && pool.AppendCertsFromPEM(ca), "PGSSLROOTCERT must point to a readable PEM CA certificate")
	if len(failures) != 0 {
		return fmt.Errorf("production preflight: %w", errors.Join(failures...))
	}
	return nil
}
