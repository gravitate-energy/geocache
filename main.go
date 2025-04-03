package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	cache "github.com/victorspringer/http-cache"
	"github.com/victorspringer/http-cache/adapter/memory"
)

// The google maps api endpoint url
var googleMapsURL = "https://maps.googleapis.com"

// The google maps api key
var googleMapsAPIKey string
var capacity int
var lifetime time.Duration

// Programs main function
func main() {

	err := godotenv.Load()

	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}

	if os.Getenv("GOOGLE_MAPS_API_KEY") != "" {
		googleMapsAPIKey = os.Getenv("GOOGLE_MAPS_API_KEY")
	}

	if os.Getenv("CACHE_CAPACITY") != "" {
		capacity, err = strconv.Atoi(os.Getenv("CACHE_CAPACITY"))
	} else {
		capacity = 10000000
	}
	log.Println("capacity: " + fmt.Sprint(capacity))

	memcached, err := memory.NewAdapter(
		memory.AdapterWithAlgorithm(memory.LRU),
		memory.AdapterWithCapacity(capacity),
	)

	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if os.Getenv("CACHE_LIFETIME") != "" {
		lifetime, _ = time.ParseDuration(os.Getenv("CACHE_LIFETIME"))
	}
	log.Println("lifetime: " + fmt.Sprint(lifetime))

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
	http.Handle("/", cacheClient.Middleware(corsMiddleware(handler)))
	http.ListenAndServe(":8080", nil)
}

// CORS middleware to add CORS headers
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

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

	ruri := r.URL.RequestURI()

	if googleMapsAPIKey != "" && !strings.Contains(ruri, "key=") {
		ruri += "&key=" + googleMapsAPIKey
	}

	resp, err := http.Get(googleMapsURL + ruri)

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
