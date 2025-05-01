package main

import "time"

type Environment struct {
	RedisHost string
	RedisPort string
	LogFormat string
}

type APIConfig struct {
	BaseURL      string
	CacheTimeout time.Duration
	Version      string
}

var (
	defaultEnv = Environment{
		RedisHost: "redis",
		RedisPort: "6379",
	}

	apiConfig = APIConfig{
		BaseURL:      "https://maps.googleapis.com",
		CacheTimeout: 720 * time.Hour,
		Version:      "1.0.0",
	}
)
