package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/redis/go-redis/v9"
)

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

	serverPort := os.Getenv("SERVER_PORT")
	if serverPort == "" {
		serverPort = defaultEnv.ServerPort
	}

	rdb := redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%s", redisHost, redisPort),
		DB:   0,
	})

	if err := rdb.Ping(context.Background()).Err(); err != nil {
		logger.log(LogCritical, "Failed to connect to Redis: %v", err)
		os.Exit(1)
	}

	server := NewServer(logger, rdb, apiConfig, nil)

	http.Handle("/health", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf("ok\nversion: %s\n", apiConfig.Version)))
	}))

	http.Handle("/", server.logMiddleware(corsMiddleware(http.HandlerFunc(server.query))))

	addr := fmt.Sprintf(":%s", serverPort)
	logger.log(LogInfo, "Starting server on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		logger.log(LogCritical, "Server failed: %v", err)
		os.Exit(1)
	}
}
