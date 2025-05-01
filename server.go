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
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type Server struct {
	logger     *Logger
	redis      *redis.Client
	config     Config
	httpClient *http.Client
}

func NewServer(logger *Logger, redis *redis.Client, config Config, httpClient *http.Client) *Server {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &Server{
		logger:     logger,
		redis:      redis,
		config:     config,
		httpClient: httpClient,
	}
}

func getCacheKey(r *http.Request, prefix string) string {
	h := sha256.New()
	h.Write([]byte(r.URL.RequestURI()))
	key := hex.EncodeToString(h.Sum(nil))
	if prefix != "" {
		return prefix + ":" + key
	}
	return key
}

func (s *Server) query(w http.ResponseWriter, r *http.Request) {
	cacheKey := getCacheKey(r, s.config.RedisPrefix)

	if cachedResponse, err := s.redis.Get(context.Background(), cacheKey).Result(); err == nil {
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

	resp, err := s.httpClient.Get(s.config.BaseURL + ruri)
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

	if err := s.redis.Set(context.Background(), cacheKey, body, s.config.CacheTimeout).Err(); err != nil {
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

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Maps-API-Key")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
