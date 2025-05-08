package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
)

var (
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)
	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Duration of HTTP requests",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)
	redisLatency = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "redis_latency_seconds",
			Help:    "Redis round-trip latency in seconds",
			Buckets: prometheus.DefBuckets,
		},
	)
	redisUp = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "redis_up",
			Help: "Whether Redis is up (1) or down (0)",
		},
	)
)

func init() {
	prometheus.MustRegister(httpRequestsTotal)
	prometheus.MustRegister(httpRequestDuration)
	prometheus.MustRegister(redisLatency)
	prometheus.MustRegister(redisUp)
}

type Server struct {
	logger     *Logger
	redis      *redis.Client
	config     Config
	httpClient *http.Client
	influx     influxdb2.Client
	bucket     string
	org        string
	token      string
	influxURL  string
}

type cacheStatusResponseWriter struct {
	statusResponseWriter
	cacheStatus string
}

func newCacheStatusResponseWriter(w http.ResponseWriter) *cacheStatusResponseWriter {
	return &cacheStatusResponseWriter{
		statusResponseWriter: *newStatusResponseWriter(w),
	}
}

func NewServer(logger *Logger, redis *redis.Client, config Config, httpClient *http.Client) *Server {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	var influx influxdb2.Client
	var bucket, org, token, influxURL string
	if config.InfluxDSN != "" && config.InfluxSampleRate > 0 {
		dsn, err := url.Parse(config.InfluxDSN)
		if err == nil {
			influxURL = dsn.Scheme + "://" + dsn.Host
			q := dsn.Query()
			bucket = q.Get("bucket")
			org = q.Get("org")
			if org == "" {
				org = "ignored"
			}
			token = q.Get("token")
			if influxURL != "" && token != "" && bucket != "" {
				influx = influxdb2.NewClient(influxURL, token)
				writeAPI := influx.WriteAPI(org, bucket)
				go func() {
					for err := range writeAPI.Errors() {
						if logger != nil {
							logger.log(LogWarning, "InfluxDB write error: %v", err)
						} else {
							fmt.Println("InfluxDB write error:", err)
						}
					}
				}()
			}
		}
	}

	return &Server{
		logger:     logger,
		redis:      redis,
		config:     config,
		httpClient: httpClient,
		influx:     influx,
		bucket:     bucket,
		org:        org,
		token:      token,
		influxURL:  influxURL,
	}
}

func (s *Server) recordCacheEvent(event string, r *http.Request, cacheKey string) {
	if s.influx == nil || s.config.InfluxSampleRate <= 0 {
		return
	}
	if rand.Float64() > s.config.InfluxSampleRate {
		return
	}
	apiKey := extractAPIKey(r)
	obfuscatedKey := obfuscateAPIKey(apiKey)
	if obfuscatedKey == "" {
		return
	}
	writeAPI := s.influx.WriteAPIBlocking(s.org, s.bucket)
	p := influxdb2.NewPoint(
		"cache_event",
		map[string]string{"event": event},
		map[string]interface{}{
			"api":       r.URL.Path,
			"api_key":   obfuscatedKey,
			"cache_key": cacheKey,
		},
		time.Now(),
	)
	_ = writeAPI.WritePoint(context.Background(), p)
}

func extractAPIKey(r *http.Request) string {
	key := r.Header.Get("X-Maps-API-Key")
	if key != "" {
		return key
	}
	q := r.URL.Query()
	if k := q.Get("key"); k != "" {
		return k
	}
	return ""
}

func obfuscateAPIKey(key string) string {
	if len(key) <= 8 {
		return key
	}
	return key[:4] + "..." + key[len(key)-4:]
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

func prometheusMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := newStatusResponseWriter(w)
		next.ServeHTTP(sw, r)
		duration := time.Since(start).Seconds()
		httpRequestsTotal.WithLabelValues(r.Method, r.URL.Path, fmt.Sprintf("%d", sw.statusCode)).Inc()
		httpRequestDuration.WithLabelValues(r.Method, r.URL.Path).Observe(duration)
	})
}

func (s *Server) query(w http.ResponseWriter, r *http.Request) {
	cacheKey := getCacheKey(r, s.config.RedisPrefix)

	redisStart := time.Now()
	cachedResponse, err := s.redis.Get(context.Background(), cacheKey).Result()
	redisLatency.Observe(time.Since(redisStart).Seconds())
	if err == nil {
		redisUp.Set(1)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Cache", "HIT")
		w.Write([]byte(cachedResponse))
		s.recordCacheEvent("hit", r, cacheKey)
		if csw, ok := w.(*cacheStatusResponseWriter); ok {
			csw.cacheStatus = "HIT"
		}
		return
	} else {
		redisUp.Set(0)
	}

	googleMapsAPIKey := r.Header.Get("X-Maps-API-Key")
	ruri := r.URL.RequestURI()

	if googleMapsAPIKey != "" && !strings.Contains(ruri, "key=") {
		ruri += "&key=" + googleMapsAPIKey
	}

	if s.config.VerboseLogging {
		headers := make(map[string]string)
		for k, v := range r.Header {
			headers[k] = strings.Join(v, ",")
		}
		s.logger.log(LogInfo, "Proxying request to backend: uri=%s headers=%v", s.config.BaseURL+ruri, headers)
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

	redisSetStart := time.Now()
	if err := s.redis.Set(context.Background(), cacheKey, body, s.config.CacheTimeout).Err(); err != nil {
		redisUp.Set(0)
		s.logger.log(LogWarning, "Failed to cache response: %v", err)
	} else {
		redisUp.Set(1)
	}
	redisLatency.Observe(time.Since(redisSetStart).Seconds())

	w.Header().Set("Content-Type", resp.Header.Get("content-type"))
	w.Header().Set("Date", resp.Header.Get("date"))
	w.Header().Set("Expires", resp.Header.Get("expires"))
	w.Header().Set("Alt-Svc", resp.Header.Get("alt-svc"))
	w.Header().Set("X-Cache", "MISS")
	w.Write(body)
	s.recordCacheEvent("miss", r, cacheKey)
	if csw, ok := w.(*cacheStatusResponseWriter); ok {
		csw.cacheStatus = "MISS"
	}
}

func (s *Server) logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			ip := r.Header.Get("X-Forwarded-For")
			if ip == "" {
				ip = r.RemoteAddr
			}

			csw := newCacheStatusResponseWriter(w)
			next.ServeHTTP(csw, r)

			entry := logEntry{
				Message:     fmt.Sprintf("%s %s", r.Method, r.URL.Path),
				Severity:    LogInfo,
				Timestamp:   time.Now(),
				IP:          ip,
				Method:      r.Method,
				Path:        r.URL.Path,
				StatusCode:  csw.statusCode,
				CacheStatus: csw.cacheStatus,
			}

			if s.logger.useGCP {
				if b, err := json.Marshal(entry); err == nil {
					fmt.Println(string(b))
				}
			} else {
				log.Printf("%s [%s] %s - %d - cache:%s", ip, r.Method, r.URL.Path, csw.statusCode, csw.cacheStatus)
			}
			return
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
