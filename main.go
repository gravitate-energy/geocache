package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

func setupRedis(config Config) (*redis.Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%s", config.RedisHost, config.RedisPort),
		DB:   0,
	})

	if err := rdb.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %v", err)
	}
	return rdb, nil
}

func isIPAllowed(remoteAddr string, cidrs []string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr // fallback if not in host:port format
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, cidr := range cidrs {
		_, network, err := net.ParseCIDR(cidr)
		if err == nil && network.Contains(ip) {
			return true
		}
	}
	return false
}

func setupServer(logger *Logger, rdb *redis.Client, config Config) *http.ServeMux {
	mux := http.NewServeMux()
	server := NewServer(logger, rdb, config, nil)

	mux.Handle("/health", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf("ok\nversion: %s\n", apiConfig.Version)))
	}))

	metricsHandler := promhttp.Handler()
	mux.Handle("/metrics", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(config.AllowedMetricsCIDRs) > 0 && !isIPAllowed(r.RemoteAddr, config.AllowedMetricsCIDRs) {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte("Forbidden\n"))
			return
		}
		metricsHandler.ServeHTTP(w, r)
	}))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Google Maps Proxy\nThis service proxies requests to Google Maps and caches responses.\nStatus: alive\n"))
			return
		}
		server.logMiddleware(http.HandlerFunc(server.query)).ServeHTTP(w, r)
	})

	return mux
}

func main() {
	config := LoadConfig()
	logger := NewLogger(config.LogFormat == "gcp")

	rdb, err := setupRedis(config)
	if err != nil {
		logger.log(LogCritical, err.Error())
		os.Exit(1)
	}

	mux := setupServer(logger, rdb, config)

	addr := fmt.Sprintf(":%s", config.ServerPort)
	logger.log(LogInfo, "Starting server on %s", addr)
	if err := http.ListenAndServe(addr, corsMiddleware(prometheusMiddleware(mux))); err != nil {
		logger.log(LogCritical, "Server failed: %v", err)
		os.Exit(1)
	}
}
