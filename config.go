package main

import (
	"os"
	"time"
)

type Environment struct {
	RedisHost  string
	RedisPort  string
	LogFormat  string
	ServerPort string
}

type APIConfig struct {
	BaseURL      string
	CacheTimeout time.Duration
	Version      string
}

var (
	defaultEnv = Environment{
		RedisHost:  "redis",
		RedisPort:  "6379",
		ServerPort: "80",
	}

	apiConfig = APIConfig{
		BaseURL:      "https://maps.googleapis.com",
		CacheTimeout: 720 * time.Hour,
		Version:      "1.0.0",
	}
)

type Config struct {
	RedisHost  string
	RedisPort  string
	ServerPort string
	LogFormat  string
}

func LoadConfig() Config {
	return Config{
		RedisHost:  getEnvOrDefault("REDIS_HOST", defaultEnv.RedisHost),
		RedisPort:  getEnvOrDefault("REDIS_PORT", defaultEnv.RedisPort),
		ServerPort: getEnvOrDefault("SERVER_PORT", defaultEnv.ServerPort),
		LogFormat:  os.Getenv("LOG_FORMAT"),
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
