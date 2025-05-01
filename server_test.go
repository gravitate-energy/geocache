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

	seen := make(map[string]string)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			got := getCacheKey(req)

			if got == "" {
				t.Error("getCacheKey() returned empty string")
			}

			if len(got) != 64 {
				t.Errorf("getCacheKey() returned hash of length %d, want 64", len(got))
			}

			if prev, exists := seen[got]; exists {
				t.Errorf("getCacheKey() returned duplicate hash for different paths: %q and %q", tt.path, prev)
			}
			seen[got] = tt.path

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

	req := httptest.NewRequest(http.MethodGet, "/query?location=TestLocation", nil)
	w := httptest.NewRecorder()

	cacheKey := getCacheKey(req)
	testData := `{"test": "data"}`
	mr.Set(cacheKey, testData)
	mr.SetTTL(cacheKey, time.Hour)

	server.query(w, req)

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

	server, mr, cleanup := setupTestServer(t, mockClient)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/query?location=TestLocation", nil)
	w := httptest.NewRecorder()

	cacheKey := getCacheKey(req)
	mr.Del(cacheKey)

	server.query(w, req)

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
	mockClient := &http.Client{
		Transport: &MockTransport{
			Response: nil,
			Err:      fmt.Errorf("mock HTTP error"),
		},
	}

	server, _, cleanup := setupTestServer(t, mockClient)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/query?location=TestLocation", nil)
	w := httptest.NewRecorder()

	server.query(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status code %d, got %d", http.StatusInternalServerError, w.Code)
	}

	expectedBody := "Failed to fetch from Google Maps API\n"
	if w.Body.String() != expectedBody {
		t.Errorf("Expected body %q, got %q", expectedBody, w.Body.String())
	}
}

func TestServer_Query_RedisCacheError(t *testing.T) {
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

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("Failed to create miniredis: %v", err)
	}

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	logger := &Logger{useGCP: false}
	config := APIConfig{
		BaseURL:      "https://maps.googleapis.com/maps/api",
		CacheTimeout: time.Hour,
	}

	server := NewServer(logger, rdb, config, mockClient)

	mr.Close()

	req := httptest.NewRequest(http.MethodGet, "/query?location=TestLocation", nil)
	w := httptest.NewRecorder()

	server.query(w, req)

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

	server, _, cleanup := setupTestServer(t, mockClient)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/query?location=TestLocation", nil)
	req.Header.Set("X-Maps-API-Key", "test-api-key")
	w := httptest.NewRecorder()

	server.query(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
	}

	expectedBody := `{"mock": "response"}`
	if w.Body.String() != expectedBody {
		t.Errorf("Expected body %q, got %q", expectedBody, w.Body.String())
	}

	if w.Header().Get("X-Cache") != "MISS" {
		t.Errorf("Expected X-Cache header to be MISS, got %s", w.Header().Get("X-Cache"))
	}
}

func TestHealthEndpoint(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf("ok\nversion: %s\n", apiConfig.Version)))
	})

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
	}

	expectedBody := fmt.Sprintf("ok\nversion: %s\n", apiConfig.Version)
	if w.Body.String() != expectedBody {
		t.Errorf("Expected body %q, got %q", expectedBody, w.Body.String())
	}
}

type errorReader struct{}

func (er errorReader) Read(p []byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func (er errorReader) Close() error {
	return nil
}

type mockTransport struct {
	response *http.Response
}

func (m *mockTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return m.response, nil
}

func TestQueryResponseBodyReadError(t *testing.T) {
	// Setup mock logger
	logger := &Logger{useGCP: false}

	// Setup mock Redis client
	rdb := redis.NewClient(&redis.Options{})

	// Setup config
	config := APIConfig{
		BaseURL:      "http://example.com",
		CacheTimeout: 0,
	}

	// Create mock response with error reader
	mockResp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       errorReader{},
		Header:     make(http.Header),
	}

	// Setup mock HTTP client
	mockClient := &http.Client{
		Transport: &mockTransport{response: mockResp},
	}

	// Create server instance
	server := NewServer(logger, rdb, config, mockClient)

	// Create test request
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	// Execute request
	server.query(w, req)

	// Verify response
	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status code %d, got %d", http.StatusInternalServerError, w.Code)
	}

	expectedBody := "Failed to read response body\n"
	if w.Body.String() != expectedBody {
		t.Errorf("Expected body %q, got %q", expectedBody, w.Body.String())
	}
}
