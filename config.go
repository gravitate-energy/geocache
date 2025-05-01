package main

import (
	"os"
	"strconv"
	"time"
)

type Environment struct {
	RedisHost    string
	RedisPort    string
	LogFormat    string
	ServerPort   string
	BaseURL      string
	CacheTimeout time.Duration
	RedisDB      int
	RedisPrefix  string
}

type APIConfig struct {
	Version string
}

var (
	defaultEnv = Environment{
		RedisHost:    "redis",
		RedisPort:    "6379",
		ServerPort:   "80",
		BaseURL:      "https://maps.googleapis.com",
		CacheTimeout: 720 * time.Hour,
		RedisDB:      0,
		RedisPrefix:  "",
	}

	apiConfig = APIConfig{
		Version: "1.0.0",
	}
)

type Config struct {
	RedisHost    string
	RedisPort    string
	ServerPort   string
	LogFormat    string
	BaseURL      string
	CacheTimeout time.Duration
	RedisDB      int
	RedisPrefix  string
}

func LoadConfig() Config {
	cacheTimeoutHours, _ := strconv.ParseInt(getEnvOrDefault("CACHE_TIMEOUT_HOURS", "720"), 10, 64)
	redisDB, _ := strconv.Atoi(getEnvOrDefault("REDIS_DB", "0"))

	return Config{
		RedisHost:    getEnvOrDefault("REDIS_HOST", defaultEnv.RedisHost),
		RedisPort:    getEnvOrDefault("REDIS_PORT", defaultEnv.RedisPort),
		ServerPort:   getEnvOrDefault("SERVER_PORT", defaultEnv.ServerPort),
		LogFormat:    os.Getenv("LOG_FORMAT"),
		BaseURL:      getEnvOrDefault("BASE_URL", defaultEnv.BaseURL),
		CacheTimeout: time.Duration(cacheTimeoutHours) * time.Hour,
		RedisDB:      redisDB,
		RedisPrefix:  getEnvOrDefault("REDIS_PREFIX", defaultEnv.RedisPrefix),
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
