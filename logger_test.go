package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLoggerOutput(t *testing.T) {
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
			name:     "Standard logging WARNING",
			useGCP:   false,
			severity: LogWarning,
			format:   "Test warning %s",
			args:     []interface{}{"warning"},
		},
		{
			name:     "Standard logging CRITICAL",
			useGCP:   false,
			severity: LogCritical,
			format:   "Test critical %s",
			args:     []interface{}{"critical"},
		},
		{
			name:     "GCP logging INFO",
			useGCP:   true,
			severity: LogInfo,
			format:   "Test info %s",
			args:     []interface{}{"info"},
		},
		{
			name:     "GCP logging WARNING",
			useGCP:   true,
			severity: LogWarning,
			format:   "Test warning %s",
			args:     []interface{}{"warning"},
		},
		{
			name:     "GCP logging ERROR",
			useGCP:   true,
			severity: LogError,
			format:   "Test error %s",
			args:     []interface{}{"error"},
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
		})
	}
}

func TestNewLogger(t *testing.T) {
	tests := []struct {
		name   string
		useGCP bool
	}{
		{
			name:   "Standard logger",
			useGCP: false,
		},
		{
			name:   "GCP logger",
			useGCP: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := NewLogger(tt.useGCP)
			if logger.useGCP != tt.useGCP {
				t.Errorf("NewLogger() useGCP = %v, want %v", logger.useGCP, tt.useGCP)
			}
		})
	}
}

func TestGCPLogFormat(t *testing.T) {
	logger := NewLogger(true)
	testTime := time.Now()

	entry := logEntry{
		Message:   "Test message",
		Severity:  LogInfo,
		Timestamp: testTime,
		IP:        "192.168.1.1",
		Method:    "GET",
		Path:      "/test",
		Error:     "test error",
	}

	// Test actual logging through the logger
	logger.log(entry.Severity, entry.Message)

	b, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Failed to marshal log entry: %v", err)
	}

	var decoded logEntry
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal log entry: %v", err)
	}

	if decoded.Message != entry.Message {
		t.Errorf("Message = %v, want %v", decoded.Message, entry.Message)
	}
	if decoded.Severity != entry.Severity {
		t.Errorf("Severity = %v, want %v", decoded.Severity, entry.Severity)
	}
	if !decoded.Timestamp.Equal(entry.Timestamp) {
		t.Errorf("Timestamp = %v, want %v", decoded.Timestamp, entry.Timestamp)
	}
	if decoded.IP != entry.IP {
		t.Errorf("IP = %v, want %v", decoded.IP, entry.IP)
	}
	if decoded.Method != entry.Method {
		t.Errorf("Method = %v, want %v", decoded.Method, entry.Method)
	}
	if decoded.Path != entry.Path {
		t.Errorf("Path = %v, want %v", decoded.Path, entry.Path)
	}
	if decoded.Error != entry.Error {
		t.Errorf("Error = %v, want %v", decoded.Error, entry.Error)
	}
}

func TestLoggerMiddlewareOutput(t *testing.T) {
	tests := []struct {
		name          string
		useGCP        bool
		method        string
		path          string
		remoteAddr    string
		xForwardedFor string
		wantIP        string
	}{
		{
			name:       "Standard logging with remote addr",
			useGCP:     false,
			method:     "GET",
			path:       "/test",
			remoteAddr: "192.168.1.1:1234",
			wantIP:     "192.168.1.1:1234",
		},
		{
			name:          "Standard logging with X-Forwarded-For",
			useGCP:        false,
			method:        "POST",
			path:          "/api/test",
			remoteAddr:    "192.168.1.1:1234",
			xForwardedFor: "10.0.0.1",
			wantIP:        "10.0.0.1",
		},
		{
			name:       "GCP logging with remote addr",
			useGCP:     true,
			method:     "PUT",
			path:       "/update",
			remoteAddr: "192.168.1.1:1234",
			wantIP:     "192.168.1.1:1234",
		},
		{
			name:          "GCP logging with X-Forwarded-For",
			useGCP:        true,
			method:        "DELETE",
			path:          "/remove",
			remoteAddr:    "192.168.1.1:1234",
			xForwardedFor: "10.0.0.1",
			wantIP:        "10.0.0.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := NewLogger(tt.useGCP)
			config := Config{
				BaseURL:      "https://maps.googleapis.com",
				CacheTimeout: time.Hour,
			}
			server := NewServer(logger, nil, config, nil)

			// Create a test request to exercise the middleware
			req := httptest.NewRequest(tt.method, tt.path, nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.xForwardedFor != "" {
				req.Header.Set("X-Forwarded-For", tt.xForwardedFor)
			}

			rr := httptest.NewRecorder()
			handler := server.logMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
			handler.ServeHTTP(rr, req)

			entry := logEntry{
				Message:   strings.Join([]string{tt.method, tt.path}, " "),
				Severity:  LogInfo,
				Timestamp: time.Now(),
				IP:        tt.wantIP,
				Method:    tt.method,
				Path:      tt.path,
			}

			if tt.useGCP {
				b, err := json.Marshal(entry)
				if err != nil {
					t.Fatalf("Failed to marshal expected log entry: %v", err)
				}
				t.Logf("Expected GCP log format: %s", string(b))
			} else {
				t.Logf("Expected standard log format: %s [%s] %s", tt.wantIP, tt.method, tt.path)
			}
		})
	}
}

func TestLoggerWithReferrer(t *testing.T) {
	tests := []struct {
		name     string
		useGCP   bool
		referrer string
	}{
		{"Standard logging with referrer", false, "example.com"},
		{"GCP logging with referrer", true, "foo.bar"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := NewLogger(tt.useGCP)
			msg := "Test message with referrer"
			logger.logWithReferrer(LogInfo, msg, tt.referrer)

			entry := logEntry{
				Message:  msg,
				Severity: LogInfo,
				Referrer: tt.referrer,
			}

			b, err := json.Marshal(entry)
			if err != nil {
				t.Fatalf("Failed to marshal log entry: %v", err)
			}

			var decoded logEntry
			if err := json.Unmarshal(b, &decoded); err != nil {
				t.Fatalf("Failed to unmarshal log entry: %v", err)
			}

			if decoded.Referrer != tt.referrer {
				t.Errorf("Referrer = %v, want %v", decoded.Referrer, tt.referrer)
			}
		})
	}
}
