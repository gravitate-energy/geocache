package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	cache "github.com/victorspringer/http-cache"
	"github.com/victorspringer/http-cache/adapter/memory"
)

// Constants
const googleMapsUrl = "https://maps.googleapis.com"
const cacheLifetime = "720h"
const cacheCapacity = 10_000_000
const appVersion = "1.0.0"

// Programs main function
func main() {

	log.Println("capacity: " + fmt.Sprintf("%d requests", cacheCapacity))

	memcached, err := memory.NewAdapter(
		memory.AdapterWithAlgorithm(memory.LRU),
		memory.AdapterWithCapacity(cacheCapacity),
	)

	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	lifetime, err := time.ParseDuration(cacheLifetime)

	if err != nil {
		fmt.Println("Failed to parse CACHE_LIFETIME: " + err.Error())
		os.Exit(1)
	}

	cacheClient, err := cache.NewClient(
		cache.ClientWithAdapter(memcached),
		cache.ClientWithTTL(lifetime),
		cache.ClientWithRefreshKey("opn"),
	)

	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	handler := http.HandlerFunc(query)

	// Wrap the handler with CORS middleware
	// http.Handle("/", cacheClient.Middleware(corsMiddleware(handler)))
	http.Handle("/health", logger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf("ok\nversion: %s\n", appVersion)))
	})))

	http.Handle("/", cacheClient.Middleware(logger(corsMiddleware(handler))))
	http.ListenAndServe(":8080", nil)
}

func logger(next http.Handler) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		ip := r.Header.Get("X-Forwarded-For")
		if ip == "" {
			ip = r.RemoteAddr
		}

		log.Printf("%s [%s] %s", ip, r.Method, r.URL.Path)
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

// Send a query against the google maps api
func query(w http.ResponseWriter, r *http.Request) {

	// get googleMapsAPIKey from request header
	googleMapsAPIKey := r.Header.Get("X-Maps-API-Key")

	// get maps key from query parameter

	ruri := r.URL.RequestURI()

	if googleMapsAPIKey != "" && !strings.Contains(ruri, "key=") {
		ruri += "&key=" + googleMapsAPIKey
	}

	resp, err := http.Get(googleMapsUrl + ruri)

	if err != nil {
		log.Fatal(err)
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)

	if err != nil {
		log.Fatal(err)
	}

	w.Header().Add("content-type", resp.Header.Get("content-type"))
	w.Header().Add("date", resp.Header.Get("date"))
	w.Header().Add("expires", resp.Header.Get("expires"))
	w.Header().Add("alt-svc", resp.Header.Get("alt-svc"))
	w.Write(body)
}
