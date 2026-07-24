package config

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	BotToken     string         `mapstructure:"BOT_TOKEN"`
	ProjectURL   string         `mapstructure:"PROJECT_URL"`
	BotPublicURL string         `mapstructure:"BOT_PUBLIC_URL"`
	Database     DatabaseConfig `mapstructure:",squash"`
	Admin        AdminConfig    `mapstructure:",squash"`
	Site         SiteConfig     `mapstructure:",squash"`
}

type DatabaseConfig struct {
	Host     string `mapstructure:"DATABASE_HOST"`
	Port     string `mapstructure:"DATABASE_PORT"`
	User     string `mapstructure:"DATABASE_USER"`
	Password string `mapstructure:"DATABASE_PASSWORD"`
	Name     string `mapstructure:"DATABASE_NAME"`
}

type AdminConfig struct {
	Port        string `mapstructure:"ADMIN_PORT"`
	AccessToken string `mapstructure:"ADMIN_ACCESS_TOKEN"`
	PublicURL   string `mapstructure:"ADMIN_PUBLIC_URL"`
}

type SiteConfig struct {
	Port string `mapstructure:"SITE_PORT"`
}

func InitConfig() (*Config, error) {
	return initConfig(true)
}

func InitSiteConfig() (*Config, error) {
	return initConfig(false)
}

func initConfig(requireBotToken bool) (*Config, error) {
	reader := viper.New()
	reader.SetConfigFile(".env")
	reader.SetConfigType("env")
	reader.AutomaticEnv()
	reader.SetDefault("ADMIN_PORT", "18080")
	reader.SetDefault("SITE_PORT", "18081")
	reader.SetDefault("PROJECT_URL", "https://github.com/J0es1ick/Scheduler")
	reader.SetDefault("BOT_PUBLIC_URL", "https://t.me/schedule_free_bot")
	for _, key := range []string{
		"BOT_TOKEN",
		"PROJECT_URL",
		"BOT_PUBLIC_URL",
		"DATABASE_HOST",
		"DATABASE_PORT",
		"DATABASE_USER",
		"DATABASE_PASSWORD",
		"DATABASE_NAME",
		"ADMIN_PORT",
		"ADMIN_ACCESS_TOKEN",
		"ADMIN_PUBLIC_URL",
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
	if len(missing) > 0 {
		return fmt.Errorf("missing required env vars: %s", strings.Join(missing, ", "))
	}
	return nil
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
