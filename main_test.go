package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
)

func TestSetupServer(t *testing.T) {
	logger := NewLogger(false)
	config := Config{
		RedisHost:    defaultEnv.RedisHost,
		RedisPort:    defaultEnv.RedisPort,
		BaseURL:      defaultEnv.BaseURL,
		CacheTimeout: defaultEnv.CacheTimeout,
	}
	rdb, err := setupRedis(config)
	if err != nil {
		t.Skip("Skipping test as Redis is not available:", err)
	}

	mux := setupServer(logger, rdb, config)

	// Test health endpoint
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Health check failed. Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestSetupRedis(t *testing.T) {
	// Start miniredis
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("Failed to start miniredis: %v", err)
	}
	defer mr.Close()

	tests := []struct {
		name        string
		config      Config
		shouldError bool
	}{
		{
			name: "valid config",
			config: Config{
				RedisHost: mr.Host(),
				RedisPort: mr.Port(),
			},
			shouldError: false,
		},
		{
			name: "invalid host",
			config: Config{
				RedisHost: "nonexistent-host",
				RedisPort: "12345",
			},
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := setupRedis(tt.config)
			if tt.shouldError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.shouldError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
			if client != nil {
				client.Close()
			}
		})
	}
}
