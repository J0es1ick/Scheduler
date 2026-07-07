package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	BotToken string         `mapstructure:"BOT_TOKEN"`
	Database DatabaseConfig `mapstructure:",squash"`
}

type DatabaseConfig struct {
	Host     string `mapstructure:"DATABASE_HOST"`
	Port     string `mapstructure:"DATABASE_PORT"`
	User     string `mapstructure:"DATABASE_USER"`
	Password string `mapstructure:"DATABASE_PASSWORD"`
	Name     string `mapstructure:"DATABASE_NAME"`
}

func InitConfig() (*Config, error) {
	viper.SetConfigFile(".env")
	viper.SetConfigType("env")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		if !isNotFoundErr(err) {
			return nil, fmt.Errorf("config: read .env: %w", err)
		}
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("config: unmarshal: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("config: validation: %w", err)
	}

	return &cfg, nil
}

func (c *Config) validate() error {
	var missing []string
	if c.BotToken == "" {
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
	msg := err.Error()
	return strings.Contains(msg, "not found") || strings.Contains(msg, "no such file")
}