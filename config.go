package main

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Environment struct {
	RedisHost        string
	RedisPort        string
	LogFormat        string
	ServerPort       string
	BaseURL          string
	CacheTimeout     time.Duration
	RedisDB          int
	RedisPrefix      string
	InfluxDSN        string
	InfluxSampleRate float64
}

type APIConfig struct {
	Version string
}

var (
	defaultEnv = Environment{
		RedisHost:        "redis",
		RedisPort:        "6379",
		ServerPort:       "80",
		BaseURL:          "https://maps.googleapis.com",
		CacheTimeout:     720 * time.Hour,
		RedisDB:          0,
		RedisPrefix:      "",
		InfluxDSN:        "",
		InfluxSampleRate: 0.0,
	}

	apiConfig = APIConfig{
		Version: "1.0.0",
	}
)

type Config struct {
	RedisHost           string
	RedisPort           string
	ServerPort          string
	LogFormat           string
	BaseURL             string
	CacheTimeout        time.Duration
	RedisDB             int
	RedisPrefix         string
	InfluxDSN           string
	InfluxSampleRate    float64
	AllowedMetricsCIDRs []string
	VerboseLogging      bool
}

func LoadConfig() Config {
	cacheTimeoutHours, _ := strconv.ParseInt(getEnvOrDefault("CACHE_TIMEOUT_HOURS", "720"), 10, 64)
	redisDB, _ := strconv.Atoi(getEnvOrDefault("REDIS_DB", "0"))
	influxSampleRate, _ := strconv.ParseFloat(getEnvOrDefault("INFLUX_SAMPLE_RATE", "0.0"), 64)

	cidrs := []string{}
	if cidrEnv := os.Getenv("ALLOWED_METRICS_CIDRS"); cidrEnv != "" {
		for _, cidr := range strings.Split(cidrEnv, ",") {
			trimmed := strings.TrimSpace(cidr)
			if trimmed != "" {
				cidrs = append(cidrs, trimmed)
			}
		}
	}

	verboseLogging := false
	if v := os.Getenv("VERBOSE_LOGGING"); v != "" {
		verboseLogging = v == "1" || strings.ToLower(v) == "true"
	}

	return Config{
		RedisHost:           getEnvOrDefault("REDIS_HOST", defaultEnv.RedisHost),
		RedisPort:           getEnvOrDefault("REDIS_PORT", defaultEnv.RedisPort),
		ServerPort:          getEnvOrDefault("SERVER_PORT", defaultEnv.ServerPort),
		LogFormat:           os.Getenv("LOG_FORMAT"),
		BaseURL:             getEnvOrDefault("BASE_URL", defaultEnv.BaseURL),
		CacheTimeout:        time.Duration(cacheTimeoutHours) * time.Hour,
		RedisDB:             redisDB,
		RedisPrefix:         getEnvOrDefault("REDIS_PREFIX", defaultEnv.RedisPrefix),
		InfluxDSN:           getEnvOrDefault("INFLUX_DSN", defaultEnv.InfluxDSN),
		InfluxSampleRate:    influxSampleRate,
		AllowedMetricsCIDRs: cidrs,
		VerboseLogging:      verboseLogging,
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
