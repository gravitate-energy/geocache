package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLogMiddleware(t *testing.T) {
	server, _, cleanup := setupTestServer(t, nil)
	defer cleanup()

	tests := []struct {
		name           string
		path           string
		method         string
		forwardedFor   string
		expectedStatus int
	}{
		{
			name:           "Regular path",
			path:           "/query",
			method:         http.MethodGet,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Health check path",
			path:           "/health",
			method:         http.MethodGet,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "With X-Forwarded-For",
			path:           "/query",
			method:         http.MethodGet,
			forwardedFor:   "192.168.1.1",
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if tt.forwardedFor != "" {
				req.Header.Set("X-Forwarded-For", tt.forwardedFor)
			}
			w := httptest.NewRecorder()

			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			wrappedHandler := server.logMiddleware(handler)
			wrappedHandler.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status code %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestCORSMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		headers        map[string]string
		expectedStatus int
	}{
		{
			name:           "OPTIONS request",
			method:         http.MethodOptions,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "GET request",
			method:         http.MethodGet,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "POST request",
			method:         http.MethodPost,
			expectedStatus: http.StatusOK,
		},
		{
			name:   "Request with custom headers",
			method: http.MethodGet,
			headers: map[string]string{
				"Authorization":  "Bearer token",
				"X-Maps-API-Key": "test-key",
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/query", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			w := httptest.NewRecorder()

			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			wrappedHandler := corsMiddleware(handler)
			wrappedHandler.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status code %d, got %d", tt.expectedStatus, w.Code)
			}

			expectedHeaders := map[string]string{
				"Access-Control-Allow-Origin":  "*",
				"Access-Control-Allow-Methods": "GET, POST, OPTIONS",
				"Access-Control-Allow-Headers": "Content-Type, Authorization, X-Maps-API-Key",
			}

			for header, expected := range expectedHeaders {
				if got := w.Header().Get(header); got != expected {
					t.Errorf("Expected header %s to be %s, got %s", header, expected, got)
				}
			}
		})
	}
}
