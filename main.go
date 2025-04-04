package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	cache "github.com/victorspringer/http-cache"
	"github.com/victorspringer/http-cache/adapter/memory"
)

// Constants
const GOOGLE_MAPS_URL = "https://maps.googleapis.com"
const CACHE_LIFETIME = "720h"
const CACHE_CAPACITY = 10_000_000

// Programs main function
func main() {

	log.Println("capacity: " + fmt.Sprintf("%d requests", CACHE_CAPACITY))

	memcached, err := memory.NewAdapter(
		memory.AdapterWithAlgorithm(memory.LRU),
		memory.AdapterWithCapacity(CACHE_CAPACITY),
	)

	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	lifetime, err := time.ParseDuration(CACHE_LIFETIME)

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
		w.Write([]byte("Server is healthy"))
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
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Google-Maps-API-Key")

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
	googleMapsAPIKey := r.Header.Get("x-google-maps-api-key")

	ruri := r.URL.RequestURI()

	if googleMapsAPIKey != "" && !strings.Contains(ruri, "key=") {
		ruri += "&key=" + googleMapsAPIKey
	}

	resp, err := http.Get(GOOGLE_MAPS_URL + ruri)

	if err != nil {
		log.Fatal(err)
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		log.Fatal(err)
	}

	w.Header().Add("content-type", resp.Header.Get("content-type"))
	w.Header().Add("date", resp.Header.Get("date"))
	w.Header().Add("expires", resp.Header.Get("expires"))
	w.Header().Add("alt-svc", resp.Header.Get("alt-svc"))
	w.Write(body)
}
