package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestSetupServer(t *testing.T) {
	// Start miniredis for a mock Redis server
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("Failed to start miniredis: %v", err)
	}
	defer mr.Close()

	logger := NewLogger(false)
	config := Config{
		RedisHost:    mr.Host(),
		RedisPort:    mr.Port(),
		BaseURL:      "https://maps.googleapis.com",
		CacheTimeout: 720 * time.Hour,
	}

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
		DB:   0,
	})
	defer rdb.Close()

	mux := setupServer(logger, rdb, config)

	tests := []struct {
		name           string
		path           string
		method         string
		expectedStatus int
		headers        map[string]string
	}{
		{
			name:           "health check",
			path:           "/health",
			method:         "GET",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "query endpoint without API key",
			path:           "/maps/api/geocode/json?address=test",
			method:         "GET",
			expectedStatus: http.StatusOK, // Server forwards request to Google Maps API without validation
		},
		{
			name:           "query endpoint with API key header",
			path:           "/maps/api/geocode/json?address=test",
			method:         "GET",
			expectedStatus: http.StatusOK,
			headers: map[string]string{
				"X-Maps-API-Key": "test-key",
			},
		},
		{
			name:           "CORS preflight request",
			path:           "/maps/api/geocode/json",
			method:         "OPTIONS",
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status code %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.method == "OPTIONS" {
				if w.Header().Get("Access-Control-Allow-Origin") == "" {
					t.Error("Expected CORS headers in response")
				}
			}
		})
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
