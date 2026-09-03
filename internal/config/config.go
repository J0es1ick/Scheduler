package config

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	DeploymentEnvironment    string         `mapstructure:"DEPLOYMENT_ENV"`
	BotToken                 string         `mapstructure:"BOT_TOKEN"`
	BotUsername              string         `mapstructure:"BOT_USERNAME"`
	BotTelegramAPIURL        string         `mapstructure:"BOT_TELEGRAM_API_URL"`
	BotTelegramAPIInsecure   bool           `mapstructure:"BOT_TELEGRAM_API_ALLOW_INSECURE"`
	BotHealthPort            string         `mapstructure:"BOT_HEALTH_PORT"`
	BotMaxConcurrentHandlers int            `mapstructure:"BOT_MAX_CONCURRENT_HANDLERS"`
	BotMaxPendingPerSender   int            `mapstructure:"BOT_MAX_PENDING_PER_SENDER"`
	BotStateTTLMinutes       int            `mapstructure:"BOT_STATE_TTL_MINUTES"`
	NotificationBatchSize    int            `mapstructure:"BOT_NOTIFICATION_BATCH_SIZE"`
	NotificationMaxBatches   int            `mapstructure:"BOT_NOTIFICATION_MAX_BATCHES"`
	NotificationPollSeconds  int            `mapstructure:"BOT_NOTIFICATION_POLL_SECONDS"`
	WorkerHealthPort         string         `mapstructure:"WORKER_HEALTH_PORT"`
	ProjectURL               string         `mapstructure:"PROJECT_URL"`
	BotPublicURL             string         `mapstructure:"BOT_PUBLIC_URL"`
	Database                 DatabaseConfig `mapstructure:",squash"`
	Admin                    AdminConfig    `mapstructure:",squash"`
	Site                     SiteConfig     `mapstructure:",squash"`
}

type DatabaseConfig struct {
	Host                    string `mapstructure:"DATABASE_HOST"`
	Port                    string `mapstructure:"DATABASE_PORT"`
	User                    string `mapstructure:"DATABASE_USER"`
	Password                string `mapstructure:"DATABASE_PASSWORD"`
	Name                    string `mapstructure:"DATABASE_NAME"`
	SSLMode                 string `mapstructure:"DATABASE_SSLMODE"`
	MaxOpenConnections      int    `mapstructure:"DATABASE_MAX_OPEN_CONNECTIONS"`
	MaxIdleConnections      int    `mapstructure:"DATABASE_MAX_IDLE_CONNECTIONS"`
	ConnectTimeoutSeconds   int    `mapstructure:"DATABASE_CONNECT_TIMEOUT_SECONDS"`
	StatementTimeoutSeconds int    `mapstructure:"DATABASE_STATEMENT_TIMEOUT_SECONDS"`
}

type AdminConfig struct {
	Port                  string `mapstructure:"ADMIN_PORT"`
	AccessToken           string `mapstructure:"ADMIN_ACCESS_TOKEN"`
	AccessKeyLoginEnabled bool   `mapstructure:"ADMIN_ACCESS_LOGIN_ENABLED"`
	CookieSecure          bool   `mapstructure:"ADMIN_COOKIE_SECURE"`
	PublicURL             string `mapstructure:"ADMIN_PUBLIC_URL"`
	TrustedProxyCIDRs     string `mapstructure:"ADMIN_TRUSTED_PROXY_CIDRS"`
	MetricsToken          string `mapstructure:"ADMIN_METRICS_TOKEN"`
}

type SiteConfig struct {
	Port string `mapstructure:"SITE_PORT"`
}

func InitConfig() (*Config, error) {
	return initConfig(true)
}

func InitAdminConfig() (*Config, error) {
	cfg, err := initConfig(true)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.Admin.MetricsToken) == "" {
		return nil, errors.New("config: validation: missing required env vars: ADMIN_METRICS_TOKEN")
	}
	if isPlaceholderSecret(cfg.Admin.MetricsToken) {
		return nil, errors.New("config: validation: ADMIN_METRICS_TOKEN still contains a placeholder value")
	}
	if len(cfg.Admin.MetricsToken) < 32 {
		return nil, errors.New("config: validation: ADMIN_METRICS_TOKEN must contain at least 32 characters")
	}
	return cfg, nil
}

func InitSiteConfig() (*Config, error) {
	return initConfig(false)
}

func InitWorkerConfig() (*Config, error) {
	return initConfig(false)
}

func initConfig(requireBotToken bool) (*Config, error) {
	reader := viper.New()
	reader.SetConfigFile(".env")
	reader.SetConfigType("env")
	reader.AutomaticEnv()
	reader.SetDefault("DEPLOYMENT_ENV", "development")
	reader.SetDefault("ADMIN_PORT", "18080")
	reader.SetDefault("DATABASE_SSLMODE", "disable")
	reader.SetDefault("DATABASE_MAX_OPEN_CONNECTIONS", 15)
	reader.SetDefault("DATABASE_MAX_IDLE_CONNECTIONS", 5)
	reader.SetDefault("DATABASE_CONNECT_TIMEOUT_SECONDS", 10)
	reader.SetDefault("DATABASE_STATEMENT_TIMEOUT_SECONDS", 30)
	reader.SetDefault("BOT_HEALTH_PORT", "18082")
	reader.SetDefault("BOT_MAX_CONCURRENT_HANDLERS", 32)
	reader.SetDefault("BOT_MAX_PENDING_PER_SENDER", 8)
	reader.SetDefault("BOT_STATE_TTL_MINUTES", 30)
	reader.SetDefault("BOT_NOTIFICATION_BATCH_SIZE", 250)
	reader.SetDefault("BOT_NOTIFICATION_MAX_BATCHES", 4)
	reader.SetDefault("BOT_NOTIFICATION_POLL_SECONDS", 1)
	reader.SetDefault("WORKER_HEALTH_PORT", "18083")
	reader.SetDefault("BOT_TELEGRAM_API_ALLOW_INSECURE", false)
	reader.SetDefault("ADMIN_ACCESS_LOGIN_ENABLED", false)
	reader.SetDefault("ADMIN_COOKIE_SECURE", true)
	reader.SetDefault("ADMIN_TRUSTED_PROXY_CIDRS", "127.0.0.1/32,::1/128")
	reader.SetDefault("SITE_PORT", "18081")
	reader.SetDefault("PROJECT_URL", "https://github.com/J0es1ick/Scheduler")
	reader.SetDefault("BOT_PUBLIC_URL", "https://t.me/schedule_free_bot")
	for _, key := range []string{
		"DEPLOYMENT_ENV",
		"BOT_TOKEN",
		"BOT_USERNAME",
		"BOT_TELEGRAM_API_URL",
		"BOT_TELEGRAM_API_ALLOW_INSECURE",
		"BOT_HEALTH_PORT",
		"BOT_MAX_CONCURRENT_HANDLERS",
		"BOT_MAX_PENDING_PER_SENDER",
		"BOT_STATE_TTL_MINUTES",
		"BOT_NOTIFICATION_BATCH_SIZE",
		"BOT_NOTIFICATION_MAX_BATCHES",
		"BOT_NOTIFICATION_POLL_SECONDS",
		"WORKER_HEALTH_PORT",
		"PROJECT_URL",
		"BOT_PUBLIC_URL",
		"DATABASE_HOST",
		"DATABASE_PORT",
		"DATABASE_USER",
		"DATABASE_PASSWORD",
		"DATABASE_NAME",
		"DATABASE_SSLMODE",
		"DATABASE_MAX_OPEN_CONNECTIONS",
		"DATABASE_MAX_IDLE_CONNECTIONS",
		"DATABASE_CONNECT_TIMEOUT_SECONDS",
		"DATABASE_STATEMENT_TIMEOUT_SECONDS",
		"ADMIN_PORT",
		"ADMIN_ACCESS_TOKEN",
		"ADMIN_ACCESS_LOGIN_ENABLED",
		"ADMIN_COOKIE_SECURE",
		"ADMIN_PUBLIC_URL",
		"ADMIN_TRUSTED_PROXY_CIDRS",
		"ADMIN_METRICS_TOKEN",
		"SITE_PORT",
	} {
		if err := reader.BindEnv(key); err != nil {
			return nil, fmt.Errorf("config: bind %s: %w", key, err)
		}
	}

	if err := reader.ReadInConfig(); err != nil {
		if !isNotFoundErr(err) {
			return nil, fmt.Errorf("config: read .env: %w", err)
		}
	}

	var cfg Config
	if err := reader.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("config: unmarshal: %w", err)
	}

	if err := cfg.validate(requireBotToken); err != nil {
		return nil, fmt.Errorf("config: validation: %w", err)
	}

	return &cfg, nil
}

func (c *Config) validate(requireBotToken bool) error {
	c.DeploymentEnvironment = strings.ToLower(strings.TrimSpace(c.DeploymentEnvironment))
	if c.DeploymentEnvironment != "development" && c.DeploymentEnvironment != "test" && c.DeploymentEnvironment != "production" {
		return errors.New("DEPLOYMENT_ENV must be one of development, test, production")
	}
	var missing []string
	if requireBotToken && c.BotToken == "" {
		missing = append(missing, "BOT_TOKEN")
	}
	if c.Database.Host == "" {
		missing = append(missing, "DATABASE_HOST")
	}
	if c.Database.Port == "" {
		missing = append(missing, "DATABASE_PORT")
	}
	if c.Database.User == "" {
		missing = append(missing, "DATABASE_USER")
	}
	if c.Database.Password == "" {
		missing = append(missing, "DATABASE_PASSWORD")
	}
	if c.Database.Name == "" {
		missing = append(missing, "DATABASE_NAME")
	}
	if c.Admin.AccessKeyLoginEnabled && c.Admin.AccessToken == "" {
		missing = append(missing, "ADMIN_ACCESS_TOKEN (required when ADMIN_ACCESS_LOGIN_ENABLED=true)")
	}
	if strings.TrimSpace(c.BotHealthPort) == "" {
		missing = append(missing, "BOT_HEALTH_PORT")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required env vars: %s", strings.Join(missing, ", "))
	}
	if requireBotToken && isPlaceholderSecret(c.BotToken) {
		return errors.New("BOT_TOKEN still contains a placeholder value")
	}
	if c.BotTelegramAPIURL != "" {
		parsed, err := url.Parse(c.BotTelegramAPIURL)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return errors.New("BOT_TELEGRAM_API_URL must be an absolute HTTP(S) URL")
		}
		if parsed.Scheme == "http" && !c.BotTelegramAPIInsecure {
			return errors.New("BOT_TELEGRAM_API_URL may use HTTP only when BOT_TELEGRAM_API_ALLOW_INSECURE=true")
		}
		if c.DeploymentEnvironment == "production" && parsed.Scheme != "https" {
			return errors.New("BOT_TELEGRAM_API_URL must use HTTPS in production")
		}
		c.BotTelegramAPIURL = strings.TrimRight(c.BotTelegramAPIURL, "/")
	}
	if isPlaceholderSecret(c.Database.Password) {
		return errors.New("DATABASE_PASSWORD still contains a placeholder value")
	}
	if len(c.Database.Password) < 16 {
		return errors.New("DATABASE_PASSWORD must contain at least 16 characters")
	}
	sslMode := strings.ToLower(strings.TrimSpace(c.Database.SSLMode))
	allowedSSLModes := map[string]bool{
		"disable":     true,
		"allow":       true,
		"prefer":      true,
		"require":     true,
		"verify-ca":   true,
		"verify-full": true,
	}
	if !allowedSSLModes[sslMode] {
		return fmt.Errorf("DATABASE_SSLMODE must be one of disable, allow, prefer, require, verify-ca, verify-full")
	}
	c.Database.SSLMode = sslMode
	if c.DeploymentEnvironment == "production" {
		if c.Admin.AccessKeyLoginEnabled {
			return errors.New("ADMIN_ACCESS_LOGIN_ENABLED must be false in production")
		}
		if sslMode == "disable" || sslMode == "allow" || sslMode == "prefer" {
			return errors.New("DATABASE_SSLMODE must be require, verify-ca, or verify-full in production")
		}
		if !c.Admin.CookieSecure {
			return errors.New("ADMIN_COOKIE_SECURE must be true in production")
		}
		if strings.TrimSpace(c.Admin.PublicURL) != "" {
			publicURL, err := url.Parse(c.Admin.PublicURL)
			if err != nil || publicURL.Scheme != "https" || publicURL.Host == "" {
				return errors.New("ADMIN_PUBLIC_URL must be an absolute HTTPS URL in production")
			}
		}
	}
	if err := validatePort("DATABASE_PORT", c.Database.Port); err != nil {
		return err
	}
	if err := validatePort("ADMIN_PORT", c.Admin.Port); err != nil {
		return err
	}
	if err := validatePort("BOT_HEALTH_PORT", c.BotHealthPort); err != nil {
		return err
	}
	if err := validatePort("SITE_PORT", c.Site.Port); err != nil {
		return err
	}
	if err := validatePort("WORKER_HEALTH_PORT", c.WorkerHealthPort); err != nil {
		return err
	}
	if c.Database.MaxOpenConnections < 1 || c.Database.MaxOpenConnections > 200 {
		return errors.New("DATABASE_MAX_OPEN_CONNECTIONS must be between 1 and 200")
	}
	if c.Database.MaxIdleConnections < 0 || c.Database.MaxIdleConnections > c.Database.MaxOpenConnections {
		return errors.New("DATABASE_MAX_IDLE_CONNECTIONS must be between 0 and DATABASE_MAX_OPEN_CONNECTIONS")
	}
	if c.Database.ConnectTimeoutSeconds < 1 || c.Database.ConnectTimeoutSeconds > 120 {
		return errors.New("DATABASE_CONNECT_TIMEOUT_SECONDS must be between 1 and 120")
	}
	if c.Database.StatementTimeoutSeconds < 1 || c.Database.StatementTimeoutSeconds > 600 {
		return errors.New("DATABASE_STATEMENT_TIMEOUT_SECONDS must be between 1 and 600")
	}
	if c.Admin.PublicURL != "" {
		parsed, err := url.Parse(c.Admin.PublicURL)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return errors.New("ADMIN_PUBLIC_URL must be an absolute HTTP(S) URL")
		}
		if parsed.Scheme == "https" && !c.Admin.CookieSecure {
			return errors.New("ADMIN_COOKIE_SECURE must be true when ADMIN_PUBLIC_URL uses HTTPS")
		}
	}
	if c.Admin.AccessKeyLoginEnabled && isPlaceholderSecret(c.Admin.AccessToken) {
		return errors.New("ADMIN_ACCESS_TOKEN still contains a placeholder value")
	}
	if c.Admin.AccessKeyLoginEnabled && len(c.Admin.AccessToken) < 32 {
		return errors.New("ADMIN_ACCESS_TOKEN must contain at least 32 characters when access-key login is enabled")
	}
	if c.BotMaxConcurrentHandlers <= 0 {
		return errors.New("BOT_MAX_CONCURRENT_HANDLERS must be greater than zero")
	}
	if c.BotMaxPendingPerSender < 0 {
		return errors.New("BOT_MAX_PENDING_PER_SENDER must not be negative")
	}
	if c.BotStateTTLMinutes <= 0 {
		return errors.New("BOT_STATE_TTL_MINUTES must be greater than zero")
	}
	if c.NotificationBatchSize < 1 || c.NotificationBatchSize > 1000 {
		return errors.New("BOT_NOTIFICATION_BATCH_SIZE must be between 1 and 1000")
	}
	if c.NotificationMaxBatches < 1 || c.NotificationMaxBatches > 20 {
		return errors.New("BOT_NOTIFICATION_MAX_BATCHES must be between 1 and 20")
	}
	if c.NotificationPollSeconds < 1 || c.NotificationPollSeconds > 60 {
		return errors.New("BOT_NOTIFICATION_POLL_SECONDS must be between 1 and 60")
	}
	return nil
}

func validatePort(name, value string) error {
	port, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("%s must be a number between 1 and 65535", name)
	}
	return nil
}

func isPlaceholderSecret(value string) bool {
	upper := strings.ToUpper(strings.TrimSpace(value))
	return strings.Contains(upper, "CHANGE_ME") || strings.Contains(upper, "PASTE_")
}

func isNotFoundErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, fs.ErrNotExist) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "not found") || strings.Contains(msg, "no such file")
}
