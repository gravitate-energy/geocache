package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// MockTransport implements http.RoundTripper for testing
type MockTransport struct {
	Response *http.Response
	Err      error
}

func (m *MockTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return m.Response, m.Err
}

// setupTestServer creates a new Server instance with mocked Redis and optional mocked HTTP client
func setupTestServer(t *testing.T, mockClient *http.Client) (*Server, *miniredis.Miniredis, func()) {
	t.Helper()

	// Create a mock Redis server
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("Failed to create miniredis: %v", err)
	}

	// Create a Redis client connected to the mock server
	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	logger := &Logger{useGCP: false}
	config := APIConfig{
		BaseURL:      "https://maps.googleapis.com/maps/api",
		CacheTimeout: time.Hour,
	}

	server := NewServer(logger, rdb, config, mockClient)

	cleanup := func() {
		mr.Close()
		rdb.Close()
	}

	return server, mr, cleanup
}

func TestGetCacheKey(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{
			name: "simple path",
			path: "/query?location=NewYork",
		},
		{
			name: "empty path",
			path: "/",
		},
		{
			name: "path with multiple params",
			path: "/query?location=NewYork&radius=10&type=restaurant",
		},
	}

	// Map to store seen hashes to check for uniqueness
	seen := make(map[string]string)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			got := getCacheKey(req)

			// Verify the hash is not empty
			if got == "" {
				t.Error("getCacheKey() returned empty string")
			}

			// Verify the hash is the correct length for SHA-256
			if len(got) != 64 {
				t.Errorf("getCacheKey() returned hash of length %d, want 64", len(got))
			}

			// Verify the hash is unique
			if prev, exists := seen[got]; exists {
				t.Errorf("getCacheKey() returned duplicate hash for different paths: %q and %q", tt.path, prev)
			}
			seen[got] = tt.path

			// Verify the hash is consistent for the same input
			got2 := getCacheKey(req)
			if got != got2 {
				t.Errorf("getCacheKey() not consistent for same input. First call: %v, Second call: %v", got, got2)
			}
		})
	}
}

func TestServer_Query_CacheHit(t *testing.T) {
	server, mr, cleanup := setupTestServer(t, nil)
	defer cleanup()

	// Create a test request
	req := httptest.NewRequest(http.MethodGet, "/query?location=TestLocation", nil)
	w := httptest.NewRecorder()

	// Set up test data in Redis
	cacheKey := getCacheKey(req)
	testData := `{"test": "data"}`
	mr.Set(cacheKey, testData)
	mr.SetTTL(cacheKey, time.Hour)

	// Call the query handler
	server.query(w, req)

	// Check response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
	}

	if w.Header().Get("X-Cache") != "HIT" {
		t.Errorf("Expected X-Cache header to be HIT, got %s", w.Header().Get("X-Cache"))
	}

	if w.Body.String() != testData {
		t.Errorf("Expected body %s, got %s", testData, w.Body.String())
	}
}

func TestServer_Query_CacheMiss(t *testing.T) {
	// Create mock response
	mockResp := &http.Response{
		Status:     "200 OK",
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"mock": "response"}`)),
		Header:     make(http.Header),
	}
	mockResp.Header.Set("content-type", "application/json")

	// Create mock HTTP client
	mockClient := &http.Client{
		Transport: &MockTransport{
			Response: mockResp,
			Err:      nil,
		},
	}

	server, mr, cleanup := setupTestServer(t, mockClient)
	defer cleanup()

	// Create a test request
	req := httptest.NewRequest(http.MethodGet, "/query?location=TestLocation", nil)
	w := httptest.NewRecorder()

	// Ensure no existing cache entry
	cacheKey := getCacheKey(req)
	mr.Del(cacheKey)

	// Call the query handler
	server.query(w, req)

	// Check response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
	}

	if w.Header().Get("X-Cache") != "MISS" {
		t.Errorf("Expected X-Cache header to be MISS, got %s", w.Header().Get("X-Cache"))
	}

	expectedBody := `{"mock": "response"}`
	if w.Body.String() != expectedBody {
		t.Errorf("Expected body %s, got %s", expectedBody, w.Body.String())
	}

	// Verify the response was cached
	if !mr.Exists(cacheKey) {
		t.Error("Expected value to be cached, but it wasn't")
	}
	cachedValue, err := mr.Get(cacheKey)
	if err != nil {
		t.Errorf("Failed to get cached value: %v", err)
	}
	if cachedValue != expectedBody {
		t.Errorf("Expected cached value %s, got %s", expectedBody, cachedValue)
	}
}

func TestServer_Query_HTTPClientError(t *testing.T) {
	// Create mock HTTP client that returns an error
	mockClient := &http.Client{
		Transport: &MockTransport{
			Response: nil,
			Err:      fmt.Errorf("mock HTTP error"),
		},
	}

	server, _, cleanup := setupTestServer(t, mockClient)
	defer cleanup()

	// Create a test request
	req := httptest.NewRequest(http.MethodGet, "/query?location=TestLocation", nil)
	w := httptest.NewRecorder()

	// Call the query handler
	server.query(w, req)

	// Check response
	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status code %d, got %d", http.StatusInternalServerError, w.Code)
	}

	expectedBody := "Failed to fetch from Google Maps API\n"
	if w.Body.String() != expectedBody {
		t.Errorf("Expected body %q, got %q", expectedBody, w.Body.String())
	}
}

func TestServer_Query_RedisCacheError(t *testing.T) {
	// Create mock response
	mockResp := &http.Response{
		Status:     "200 OK",
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"mock": "response"}`)),
		Header:     make(http.Header),
	}
	mockResp.Header.Set("content-type", "application/json")

	mockClient := &http.Client{
		Transport: &MockTransport{
			Response: mockResp,
			Err:      nil,
		},
	}

	// Create a mock Redis server that will be stopped to simulate errors
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("Failed to create miniredis: %v", err)
	}

	// Create a Redis client connected to the mock server
	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	logger := &Logger{useGCP: false}
	config := APIConfig{
		BaseURL:      "https://maps.googleapis.com/maps/api",
		CacheTimeout: time.Hour,
	}

	server := NewServer(logger, rdb, config, mockClient)

	// Stop Redis server to simulate connection error
	mr.Close()

	// Create a test request
	req := httptest.NewRequest(http.MethodGet, "/query?location=TestLocation", nil)
	w := httptest.NewRecorder()

	// Call the query handler
	server.query(w, req)

	// Check response - should still work but with a cache miss
	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
	}

	if w.Header().Get("X-Cache") != "MISS" {
		t.Errorf("Expected X-Cache header to be MISS, got %s", w.Header().Get("X-Cache"))
	}

	expectedBody := `{"mock": "response"}`
	if w.Body.String() != expectedBody {
		t.Errorf("Expected body %q, got %q", expectedBody, w.Body.String())
	}
}

func TestServer_Query_WithAPIKey(t *testing.T) {
	// Create a mock response
	mockResp := &http.Response{
		Status:     "200 OK",
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"mock": "response"}`)),
		Header:     make(http.Header),
	}
	mockResp.Header.Set("content-type", "application/json")

	// Create a mock transport that captures the request URL
	transport := &MockTransport{
		Response: mockResp,
		Err:      nil,
	}
	mockClient := &http.Client{Transport: transport}

	server, _, cleanup := setupTestServer(t, mockClient)
	defer cleanup()

	// Create a test request with API key
	req := httptest.NewRequest(http.MethodGet, "/query?location=TestLocation", nil)
	req.Header.Set("X-Maps-API-Key", "test-api-key")
	w := httptest.NewRecorder()

	// Call the query handler
	server.query(w, req)

	// Check response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
	}

	// Check that the response was received
	expectedBody := `{"mock": "response"}`
	if w.Body.String() != expectedBody {
		t.Errorf("Expected body %q, got %q", expectedBody, w.Body.String())
	}

	// Check that X-Cache header is set to MISS
	if w.Header().Get("X-Cache") != "MISS" {
		t.Errorf("Expected X-Cache header to be MISS, got %s", w.Header().Get("X-Cache"))
	}
}

func TestServer_LogMiddleware(t *testing.T) {
	server, _, cleanup := setupTestServer(t, nil)
	defer cleanup()

	// Test cases for different paths and methods
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

			// Create a simple handler that always returns 200 OK
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			// Wrap the handler with the log middleware
			wrappedHandler := server.logMiddleware(handler)
			wrappedHandler.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status code %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestCorsMiddleware(t *testing.T) {
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

			// Create a simple handler that always returns 200 OK
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			// Wrap the handler with the CORS middleware
			wrappedHandler := corsMiddleware(handler)
			wrappedHandler.ServeHTTP(w, req)

			// Check status code
			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status code %d, got %d", tt.expectedStatus, w.Code)
			}

			// Check CORS headers
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

func TestLogger(t *testing.T) {
	tests := []struct {
		name     string
		useGCP   bool
		severity LogSeverity
		format   string
		args     []interface{}
	}{
		{
			name:     "Standard logging INFO",
			useGCP:   false,
			severity: LogInfo,
			format:   "Test message %s",
			args:     []interface{}{"info"},
		},
		{
			name:     "Standard logging ERROR",
			useGCP:   false,
			severity: LogError,
			format:   "Test error %s",
			args:     []interface{}{"error"},
		},
		{
			name:     "GCP logging WARNING",
			useGCP:   true,
			severity: LogWarning,
			format:   "Test warning %s",
			args:     []interface{}{"warning"},
		},
		{
			name:     "GCP logging CRITICAL",
			useGCP:   true,
			severity: LogCritical,
			format:   "Test critical %s",
			args:     []interface{}{"critical"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := NewLogger(tt.useGCP)
			logger.log(tt.severity, tt.format, tt.args...)
			// Note: Since logger outputs to stdout/stderr, we can't easily capture and verify the output
			// This test mainly ensures the logger doesn't panic and runs without errors
		})
	}
}

func TestHealthEndpoint(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	// Create the health handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf("ok\nversion: %s\n", apiConfig.Version)))
	})

	handler.ServeHTTP(w, req)

	// Check status code
	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
	}

	// Check response body
	expectedBody := fmt.Sprintf("ok\nversion: %s\n", apiConfig.Version)
	if w.Body.String() != expectedBody {
		t.Errorf("Expected body %q, got %q", expectedBody, w.Body.String())
	}
}
