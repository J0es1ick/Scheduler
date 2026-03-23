package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

type Config struct {
	ServerPort string `mapstructure:"SERVER_PORT"`;
	BotToken   string `mapstructure:"BOT_TOKEN"`;
	Database string   `mapstructure:",squash"`;
}

type DatabaseConfig struct {
	Host string 	`mapstructure:"DB_HOST"`;
	Port string 	`mapstructure:"DB_PORT"`;
	Name string 	`mapstructure:"DB_NAME"`;
	User string 	`mapstructure:"DB_USER"`;
	Password string `mapstructure:"DB_PASSWORD"`;
}

func InitConfig() (*Config, error) {
	exePath, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	rootPath := filepath.Dir(filepath.Dir(exePath))

	viper.SetConfigName(".env")
	viper.SetConfigType("env")
	viper.AddConfigPath(rootPath)

	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &cfg, nil;
}