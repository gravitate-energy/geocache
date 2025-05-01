package main

import (
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
