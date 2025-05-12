package main

import (
	"encoding/json"
	"fmt"
	"log"
	"time"
)

type LogSeverity string

const (
	LogInfo     LogSeverity = "INFO"
	LogWarning  LogSeverity = "WARNING"
	LogError    LogSeverity = "ERROR"
	LogCritical LogSeverity = "CRITICAL"
)

type Logger struct {
	useGCP bool
}

type logEntry struct {
	Message     string      `json:"message"`
	Severity    LogSeverity `json:"severity"`
	Timestamp   time.Time   `json:"timestamp"`
	IP          string      `json:"ip,omitempty"`
	Method      string      `json:"method,omitempty"`
	Path        string      `json:"path,omitempty"`
	Error       string      `json:"error,omitempty"`
	StatusCode  int         `json:"status_code,omitempty"`
	CacheStatus string      `json:"cache_status,omitempty"`
	Referrer    string      `json:"referrer,omitempty"`
}

func NewLogger(useGCP bool) *Logger {
	return &Logger{useGCP: useGCP}
}

func (l *Logger) log(severity LogSeverity, format string, v ...interface{}) {
	entry := logEntry{
		Message:   fmt.Sprintf(format, v...),
		Severity:  severity,
		Timestamp: time.Now(),
	}

	if l.useGCP {
		if b, err := json.Marshal(entry); err == nil {
			fmt.Println(string(b))
			return
		}
	}

	log.Printf(format, v...)
}

func (l *Logger) logWithReferrer(severity LogSeverity, format string, referrer string, cacheStatus string, statusCode int, v ...interface{}) {
	entry := logEntry{
		Message:     fmt.Sprintf(format, v...),
		Severity:    severity,
		Timestamp:   time.Now(),
		Referrer:    referrer,
		CacheStatus: cacheStatus,
		StatusCode:  statusCode,
	}

	if l.useGCP {
		if b, err := json.Marshal(entry); err == nil {
			fmt.Println(string(b))
			return
		}
	}

	log.Printf(format, v...)
}
