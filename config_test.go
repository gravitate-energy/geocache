package main

import (
	"os"
	"testing"
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
				RedisHost:  defaultEnv.RedisHost,
				RedisPort:  defaultEnv.RedisPort,
				ServerPort: defaultEnv.ServerPort,
				LogFormat:  "",
			},
		},
		{
			name: "uses environment variables when set",
			envVars: map[string]string{
				"REDIS_HOST":  "custom-redis",
				"REDIS_PORT":  "6380",
				"SERVER_PORT": "8081",
				"LOG_FORMAT":  "gcp",
			},
			expected: Config{
				RedisHost:  "custom-redis",
				RedisPort:  "6380",
				ServerPort: "8081",
				LogFormat:  "gcp",
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
		})
	}
}
