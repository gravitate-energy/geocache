package main

import (
	"context"
	"fmt"
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

func setupServer(logger *Logger, rdb *redis.Client, config Config) *http.ServeMux {
	mux := http.NewServeMux()
	server := NewServer(logger, rdb, config, nil)

	mux.Handle("/health", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf("ok\nversion: %s\n", apiConfig.Version)))
	}))

	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("/", server.logMiddleware(http.HandlerFunc(server.query)))
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
