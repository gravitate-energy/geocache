package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type Environment struct {
	RedisHost string
	RedisPort string
	LogFormat string
}

type APIConfig struct {
	BaseURL      string
	CacheTimeout time.Duration
	Version      string
}

var (
	defaultEnv = Environment{
		RedisHost: "redis",
		RedisPort: "6379",
	}

	apiConfig = APIConfig{
		BaseURL:      "https://maps.googleapis.com",
		CacheTimeout: 720 * time.Hour,
		Version:      "1.0.0",
	}

	ctx = context.Background()
	rdb *redis.Client
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
	Message   string      `json:"message"`
	Severity  LogSeverity `json:"severity"`
	Timestamp time.Time   `json:"timestamp"`
	IP        string      `json:"ip,omitempty"`
	Method    string      `json:"method,omitempty"`
	Path      string      `json:"path,omitempty"`
	Error     string      `json:"error,omitempty"`
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

type Server struct {
	logger *Logger
	redis  *redis.Client
	config APIConfig
}

func NewServer(logger *Logger, redis *redis.Client, config APIConfig) *Server {
	return &Server{
		logger: logger,
		redis:  redis,
		config: config,
	}
}

func getCacheKey(r *http.Request) string {
	// Create a unique cache key based on the full request URI
	h := sha256.New()
	h.Write([]byte(r.URL.RequestURI()))
	return hex.EncodeToString(h.Sum(nil))
}

func (s *Server) query(w http.ResponseWriter, r *http.Request) {
	cacheKey := getCacheKey(r)

	if cachedResponse, err := s.redis.Get(ctx, cacheKey).Result(); err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Cache", "HIT")
		w.Write([]byte(cachedResponse))
		return
	}

	googleMapsAPIKey := r.Header.Get("X-Maps-API-Key")
	ruri := r.URL.RequestURI()

	if googleMapsAPIKey != "" && !strings.Contains(ruri, "key=") {
		ruri += "&key=" + googleMapsAPIKey
	}

	resp, err := http.Get(s.config.BaseURL + ruri)
	if err != nil {
		s.logger.log(LogError, "Failed to fetch from Google Maps API: %v", err)
		http.Error(w, "Failed to fetch from Google Maps API", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		s.logger.log(LogError, "Failed to read response body: %v", err)
		http.Error(w, "Failed to read response body", http.StatusInternalServerError)
		return
	}

	if err := s.redis.Set(ctx, cacheKey, body, s.config.CacheTimeout).Err(); err != nil {
		s.logger.log(LogWarning, "Failed to cache response: %v", err)
	}

	w.Header().Set("Content-Type", resp.Header.Get("content-type"))
	w.Header().Set("Date", resp.Header.Get("date"))
	w.Header().Set("Expires", resp.Header.Get("expires"))
	w.Header().Set("Alt-Svc", resp.Header.Get("alt-svc"))
	w.Header().Set("X-Cache", "MISS")
	w.Write(body)
}

func (s *Server) logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			ip := r.Header.Get("X-Forwarded-For")
			if ip == "" {
				ip = r.RemoteAddr
			}

			entry := logEntry{
				Message:   fmt.Sprintf("%s %s", r.Method, r.URL.Path),
				Severity:  LogInfo,
				Timestamp: time.Now(),
				IP:        ip,
				Method:    r.Method,
				Path:      r.URL.Path,
			}

			if s.logger.useGCP {
				if b, err := json.Marshal(entry); err == nil {
					fmt.Println(string(b))
				}
			} else {
				log.Printf("%s [%s] %s", ip, r.Method, r.URL.Path)
			}
		}
		next.ServeHTTP(w, r)
	})
}

// CORS middleware to add CORS headers
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Maps-API-Key")

		// Handle preflight requests
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func main() {
	logger := NewLogger(os.Getenv("LOG_FORMAT") == "gcp")

	redisHost := os.Getenv("REDIS_HOST")
	if redisHost == "" {
		redisHost = defaultEnv.RedisHost
	}

	redisPort := os.Getenv("REDIS_PORT")
	if redisPort == "" {
		redisPort = defaultEnv.RedisPort
	}

	rdb = redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%s", redisHost, redisPort),
		DB:   0,
	})

	if err := rdb.Ping(ctx).Err(); err != nil {
		logger.log(LogCritical, "Failed to connect to Redis: %v", err)
		os.Exit(1)
	}

	server := NewServer(logger, rdb, apiConfig)

	http.Handle("/health", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf("ok\nversion: %s\n", apiConfig.Version)))
	}))

	http.Handle("/", server.logMiddleware(corsMiddleware(http.HandlerFunc(server.query))))

	logger.log(LogInfo, "Starting server on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		logger.log(LogCritical, "Server failed: %v", err)
		os.Exit(1)
	}
}
