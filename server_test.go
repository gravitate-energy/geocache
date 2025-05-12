package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/prometheus/client_golang/prometheus/testutil"
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

	config := Config{
		BaseURL:      "https://maps.googleapis.com/maps/api",
		CacheTimeout: time.Hour,
		RedisDB:      0,
		RedisPrefix:  "test",
	}

	// Create a Redis client connected to the mock server
	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
		DB:   config.RedisDB,
	})

	logger := &Logger{useGCP: false}

	server := NewServer(logger, rdb, config, mockClient)

	cleanup := func() {
		mr.Close()
		rdb.Close()
	}

	return server, mr, cleanup
}

func TestGetCacheKey(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		prefix string
	}{
		{
			name:   "simple path no prefix",
			path:   "/query?location=NewYork",
			prefix: "",
		},
		{
			name:   "simple path with prefix",
			path:   "/query?location=NewYork",
			prefix: "test",
		},
		{
			name:   "empty path with prefix",
			path:   "/",
			prefix: "prod",
		},
		{
			name:   "path with multiple params and prefix",
			path:   "/query?location=NewYork&radius=10&type=restaurant",
			prefix: "staging",
		},
		{
			name:   "path with key param",
			path:   "/query?location=NewYork&key=abc123",
			prefix: "",
		},
		{
			name:   "path with key param in different position",
			path:   "/query?key=abc123&location=NewYork",
			prefix: "",
		},
		{
			name:   "path with same params different order",
			path:   "/query?radius=10&type=restaurant&location=NewYork",
			prefix: "staging",
		},
	}

	seen := make(map[string]string)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			got := getCacheKey(req, tt.prefix)

			if got == "" {
				t.Error("getCacheKey() returned empty string")
			}

			if tt.prefix != "" && !strings.HasPrefix(got, tt.prefix+":") {
				t.Errorf("getCacheKey() = %q, want prefix %q:", got, tt.prefix)
			}

			if prev, exists := seen[got]; exists {
				// Only allow duplicate hash if the paths are equivalent after removing 'key' and sorting params
				if !equivalentPaths(tt.path, prev) {
					t.Errorf("getCacheKey() returned duplicate hash for different paths: %q and %q", tt.path, prev)
				}
			}
			seen[got] = tt.path

			got2 := getCacheKey(req, tt.prefix)
			if got != got2 {
				t.Errorf("getCacheKey() not consistent for same input. First call: %v, Second call: %v", got, got2)
			}
		})
	}

	// Explicitly test that key param and param order do not affect the cache key
	req1 := httptest.NewRequest(http.MethodGet, "/query?location=NewYork&key=abc123", nil)
	req2 := httptest.NewRequest(http.MethodGet, "/query?key=def456&location=NewYork", nil)
	key1 := getCacheKey(req1, "")
	key2 := getCacheKey(req2, "")
	if key1 != key2 {
		t.Errorf("Cache key should be the same when only the 'key' param or its value changes. Got %q and %q", key1, key2)
	}

	req3 := httptest.NewRequest(http.MethodGet, "/query?radius=10&type=restaurant&location=NewYork", nil)
	req4 := httptest.NewRequest(http.MethodGet, "/query?location=NewYork&type=restaurant&radius=10", nil)
	key3 := getCacheKey(req3, "staging")
	key4 := getCacheKey(req4, "staging")
	if key3 != key4 {
		t.Errorf("Cache key should be the same for same params in different order. Got %q and %q", key3, key4)
	}

	// Additional tests for endpoint-specific whitelisting
	directionsTests := []struct {
		name        string
		path        string
		prefix      string
		shouldMatch bool
	}{
		{
			name:        "directions: only origin and destination matter",
			path:        "/maps/api/directions/json?origin=30.1,40.2&destination=31.1,41.2&foo=bar",
			prefix:      "",
			shouldMatch: true,
		},
		{
			name:        "directions: extra param ignored",
			path:        "/maps/api/directions/json?destination=31.1,41.2&origin=30.1,40.2&baz=qux",
			prefix:      "",
			shouldMatch: true,
		},
		{
			name:        "directions: different origin",
			path:        "/maps/api/directions/json?origin=32.1,42.2&destination=31.1,41.2",
			prefix:      "",
			shouldMatch: false,
		},
	}

	directionsKey := ""
	for i, tt := range directionsTests {
		req := httptest.NewRequest(http.MethodGet, tt.path, nil)
		key := getCacheKey(req, tt.prefix)
		if i == 0 {
			directionsKey = key
		} else if tt.shouldMatch {
			if key != directionsKey {
				t.Errorf("Directions cache key mismatch: got %q, want %q", key, directionsKey)
			}
		} else {
			if key == directionsKey {
				t.Errorf("Directions cache key should differ for different origin/destination: %q", key)
			}
		}
	}

	distMatrixTests := []struct {
		name        string
		path        string
		prefix      string
		shouldMatch bool
	}{
		{
			name:        "distancematrix: only origins and destinations matter",
			path:        "/maps/api/distancematrix/json?origins=30.1,40.2&destinations=31.1,41.2&foo=bar",
			prefix:      "",
			shouldMatch: true,
		},
		{
			name:        "distancematrix: extra param ignored",
			path:        "/maps/api/distancematrix/json?destinations=31.1,41.2&origins=30.1,40.2&baz=qux",
			prefix:      "",
			shouldMatch: true,
		},
		{
			name:        "distancematrix: different origins",
			path:        "/maps/api/distancematrix/json?origins=32.1,42.2&destinations=31.1,41.2",
			prefix:      "",
			shouldMatch: false,
		},
	}

	distMatrixKey := ""
	for i, tt := range distMatrixTests {
		req := httptest.NewRequest(http.MethodGet, tt.path, nil)
		key := getCacheKey(req, tt.prefix)
		if i == 0 {
			distMatrixKey = key
		} else if tt.shouldMatch {
			if key != distMatrixKey {
				t.Errorf("DistanceMatrix cache key mismatch: got %q, want %q", key, distMatrixKey)
			}
		} else {
			if key == distMatrixKey {
				t.Errorf("DistanceMatrix cache key should differ for different origins/destinations: %q", key)
			}
		}
	}
}

// equivalentPaths returns true if two paths are equivalent after removing 'key' param and sorting params
func equivalentPaths(a, b string) bool {
	parse := func(s string) (string, []string) {
		u, _ := url.Parse(s)
		q := u.Query()
		q.Del("key")
		keys := make([]string, 0, len(q))
		for k := range q {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		params := make([]string, 0, len(keys))
		for _, k := range keys {
			for _, v := range q[k] {
				params = append(params, k+"="+v)
			}
		}
		return u.Path, params
	}
	pa, qa := parse(a)
	pb, qb := parse(b)
	if pa != pb {
		return false
	}
	if len(qa) != len(qb) {
		return false
	}
	for i := range qa {
		if qa[i] != qb[i] {
			return false
		}
	}
	return true
}

func TestServer_Query_CacheHit(t *testing.T) {
	server, mr, cleanup := setupTestServer(t, nil)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/query?location=TestLocation", nil)
	w := httptest.NewRecorder()

	cacheKey := getCacheKey(req, server.config.RedisPrefix)
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

	cacheKey := getCacheKey(req, server.config.RedisPrefix)
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

	config := Config{
		BaseURL:      "https://maps.googleapis.com/maps/api",
		CacheTimeout: time.Hour,
		RedisDB:      0,
		RedisPrefix:  "test",
	}

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
		DB:   config.RedisDB,
	})

	logger := &Logger{useGCP: false}

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
	config := Config{
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

func TestPrometheusMetrics_AreUpdated(t *testing.T) {
	server, mr, cleanup := setupTestServer(t, nil)
	defer cleanup()

	// Set up a cache hit
	cacheKey := getCacheKey(httptest.NewRequest(http.MethodGet, "/query?location=TestLocation", nil), server.config.RedisPrefix)
	testData := `{"test": "data"}`
	mr.Set(cacheKey, testData)
	mr.SetTTL(cacheKey, time.Hour)

	req := httptest.NewRequest(http.MethodGet, "/query?location=TestLocation", nil)
	w := httptest.NewRecorder()

	before := testutil.ToFloat64(httpRequestsTotal.WithLabelValues("GET", "/query", "200"))
	handler := prometheusMiddleware(http.HandlerFunc(server.query))
	handler.ServeHTTP(w, req)
	after := testutil.ToFloat64(httpRequestsTotal.WithLabelValues("GET", "/query", "200"))

	if after-before != 1 {
		t.Errorf("Expected httpRequestsTotal to increment by 1, got %v", after-before)
	}

	up := testutil.ToFloat64(redisUp)
	if up != 1 {
		t.Errorf("Expected redisUp to be 1 after successful Redis get, got %v", up)
	}
}

func TestLog_CacheHitAndMiss(t *testing.T) {
	var buf bytes.Buffer
	origOutput := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(origOutput)

	server, mr, cleanup := setupTestServer(t, nil)
	defer cleanup()

	// Cache miss: ensure key is not present
	cacheKey := getCacheKey(httptest.NewRequest(http.MethodGet, "/query?location=TestLocation", nil), server.config.RedisPrefix)
	mr.Del(cacheKey)

	reqMiss := httptest.NewRequest(http.MethodGet, "/query?location=TestLocation", nil)
	wMiss := httptest.NewRecorder()
	// Use a mock HTTP client to avoid real network call
	mockResp := &http.Response{
		Status:     "200 OK",
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"mock": "response"}`)),
		Header:     make(http.Header),
	}
	mockResp.Header.Set("content-type", "application/json")
	server.httpClient = &http.Client{Transport: &MockTransport{Response: mockResp}}

	// Wrap with logMiddleware
	handler := server.logMiddleware(http.HandlerFunc(server.query))
	handler.ServeHTTP(wMiss, reqMiss)

	if !strings.Contains(buf.String(), "cache:MISS") {
		t.Errorf("Expected log to contain cache:MISS, got: %s", buf.String())
	}
	buf.Reset()

	// Cache hit: set the value in Redis
	mr.Set(cacheKey, `{"mock": "response"}`)
	mr.SetTTL(cacheKey, time.Hour)
	reqHit := httptest.NewRequest(http.MethodGet, "/query?location=TestLocation", nil)
	wHit := httptest.NewRecorder()
	handler.ServeHTTP(wHit, reqHit)

	if !strings.Contains(buf.String(), "cache:HIT") {
		t.Errorf("Expected log to contain cache:HIT, got: %s", buf.String())
	}
}

func TestLogMiddleware_ReferrerSuffixStripping(t *testing.T) {
	var buf bytes.Buffer
	origOutput := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(origOutput)

	server, _, cleanup := setupTestServer(t, nil)
	defer cleanup()

	cases := []struct {
		name         string
		referer      string
		wantReferrer string
	}{
		{"strips .bb.gravitate.energy", "https://foo.bb.gravitate.energy/some/path", "foo"},
		{"keeps other domains", "https://bar.example.com/some/path", "bar.example.com"},
		{"keeps subdomain with similar ending", "https://baz.bb.gravitate.energy.com/path", "baz.bb.gravitate.energy.com"},
	}

	handler := server.logMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			buf.Reset()
			req := httptest.NewRequest(http.MethodGet, "/maps/api/directions/json", nil)
			req.Header.Set("Referer", tc.referer)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if !strings.Contains(buf.String(), "referrer:"+tc.wantReferrer) {
				t.Errorf("Expected log to contain referrer:%s, got: %s", tc.wantReferrer, buf.String())
			}
		})
	}

	// Test with GCP logger
	server.logger.useGCP = true
	for _, tc := range cases {
		t.Run(tc.name+"_GCP", func(t *testing.T) {
			buf.Reset()
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w
			req := httptest.NewRequest(http.MethodGet, "/maps/api/directions/json", nil)
			req.Header.Set("Referer", tc.referer)
			respW := httptest.NewRecorder()
			handler.ServeHTTP(respW, req)
			w.Close()
			os.Stdout = oldStdout
			out, _ := io.ReadAll(r)
			r.Close()
			if !strings.Contains(string(out), tc.wantReferrer) {
				t.Errorf("Expected GCP log to contain referrer %s, got: %s", tc.wantReferrer, string(out))
			}
		})
	}
}
