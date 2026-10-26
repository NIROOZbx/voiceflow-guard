package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config contains the process configuration loaded from environment variables.
type Config struct {
	Host        string
	Port        int
	DatabaseURL string
}

func Load() (Config, error) {
	port := 8080
	if value := os.Getenv("PORT"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 65535 {
			return Config{}, fmt.Errorf("PORT must be an integer from 1 to 65535")
		}
		port = parsed
	}

	host := os.Getenv("HOST")
	if host == "" {
		host = "0.0.0.0"
	}

	return Config{
		Host:        host,
		Port:        port,
		DatabaseURL: os.Getenv("DATABASE_URL"),
	}, nil
}

func (c Config) HTTPAddress() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}
