package main

import (
	"os"
	"testing"
	"time"
)

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		expected Config
	}{
		{
			name:    "uses defaults when env vars not set",
			envVars: map[string]string{},
			expected: Config{
				RedisHost:        defaultEnv.RedisHost,
				RedisPort:        defaultEnv.RedisPort,
				ServerPort:       defaultEnv.ServerPort,
				LogFormat:        "",
				BaseURL:          defaultEnv.BaseURL,
				CacheTimeout:     defaultEnv.CacheTimeout,
				RedisDB:          defaultEnv.RedisDB,
				RedisPrefix:      defaultEnv.RedisPrefix,
				InfluxDSN:        defaultEnv.InfluxDSN,
				InfluxSampleRate: defaultEnv.InfluxSampleRate,
			},
		},
		{
			name: "uses environment variables when set",
			envVars: map[string]string{
				"REDIS_HOST":          "custom-redis",
				"REDIS_PORT":          "6380",
				"SERVER_PORT":         "8081",
				"LOG_FORMAT":          "gcp",
				"BASE_URL":            "https://custom-maps.example.com",
				"CACHE_TIMEOUT_HOURS": "48",
				"REDIS_DB":            "2",
				"REDIS_PREFIX":        "prod",
				"INFLUX_DSN":          "http://influxdb:8086?org=test&bucket=cache&token=abc",
				"INFLUX_SAMPLE_RATE":  "0.25",
			},
			expected: Config{
				RedisHost:        "custom-redis",
				RedisPort:        "6380",
				ServerPort:       "8081",
				LogFormat:        "gcp",
				BaseURL:          "https://custom-maps.example.com",
				CacheTimeout:     48 * time.Hour,
				RedisDB:          2,
				RedisPrefix:      "prod",
				InfluxDSN:        "http://influxdb:8086?org=test&bucket=cache&token=abc",
				InfluxSampleRate: 0.25,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment before each test
			os.Clearenv()

			// Set test environment variables
			for k, v := range tt.envVars {
				os.Setenv(k, v)
			}

			config := LoadConfig()

			if config.RedisHost != tt.expected.RedisHost {
				t.Errorf("RedisHost = %v, want %v", config.RedisHost, tt.expected.RedisHost)
			}
			if config.RedisPort != tt.expected.RedisPort {
				t.Errorf("RedisPort = %v, want %v", config.RedisPort, tt.expected.RedisPort)
			}
			if config.ServerPort != tt.expected.ServerPort {
				t.Errorf("ServerPort = %v, want %v", config.ServerPort, tt.expected.ServerPort)
			}
			if config.LogFormat != tt.expected.LogFormat {
				t.Errorf("LogFormat = %v, want %v", config.LogFormat, tt.expected.LogFormat)
			}
			if config.BaseURL != tt.expected.BaseURL {
				t.Errorf("BaseURL = %v, want %v", config.BaseURL, tt.expected.BaseURL)
			}
			if config.CacheTimeout != tt.expected.CacheTimeout {
				t.Errorf("CacheTimeout = %v, want %v", config.CacheTimeout, tt.expected.CacheTimeout)
			}
			if config.RedisDB != tt.expected.RedisDB {
				t.Errorf("RedisDB = %v, want %v", config.RedisDB, tt.expected.RedisDB)
			}
			if config.RedisPrefix != tt.expected.RedisPrefix {
				t.Errorf("RedisPrefix = %v, want %v", config.RedisPrefix, tt.expected.RedisPrefix)
			}
			if config.InfluxDSN != tt.expected.InfluxDSN {
				t.Errorf("InfluxDSN = %v, want %v", config.InfluxDSN, tt.expected.InfluxDSN)
			}
			if config.InfluxSampleRate != tt.expected.InfluxSampleRate {
				t.Errorf("InfluxSampleRate = %v, want %v", config.InfluxSampleRate, tt.expected.InfluxSampleRate)
			}
		})
	}
}
