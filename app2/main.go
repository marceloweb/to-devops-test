package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

type TimeResponse struct {
	Timezone    string `json:"timezone"`
	CurrentTime string `json:"current_time"`
	Cache       string `json:"cache,omitempty"`
}

type TextResponse struct {
	Message string `json:"message"`
	Cache   string `json:"cache,omitempty"`
}

var ctx = context.Background()
var rdb *redis.Client

var (
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	cacheHits = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cache_hits_total",
			Help: "Total cache hits",
		},
		[]string{"app"},
	)

	cacheMisses = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cache_misses_total",
			Help: "Total cache misses",
		},
		[]string{"app"},
	)
)

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func metricsMiddleware(path string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, statusCode: 200}

		next(rec, r)

		duration := time.Since(start).Seconds()

		httpRequestsTotal.WithLabelValues(
			r.Method,
			path,
			strconv.Itoa(rec.statusCode),
		).Inc()

		httpRequestDuration.WithLabelValues(r.Method, path).Observe(duration)
	}
}

func timeHandler(w http.ResponseWriter, r *http.Request) {
	cacheKey := "app2:time"

	cached, err := rdb.Get(ctx, cacheKey).Result()
	if err == nil {
		cacheHits.WithLabelValues("app2").Inc()

		var response TimeResponse
		if err := json.Unmarshal([]byte(cached), &response); err == nil {
			response.Cache = "hit"
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
			return
		}
	}

	cacheMisses.WithLabelValues("app2").Inc()

	location, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		http.Error(w, "failed to load timezone", http.StatusInternalServerError)
		return
	}

	now := time.Now().In(location)

	response := TimeResponse{
		Timezone:    "America/Sao_Paulo",
		CurrentTime: now.Format("2006-01-02 15:04:05"),
		Cache:       "miss",
	}

	raw, _ := json.Marshal(TimeResponse{
		Timezone:    response.Timezone,
		CurrentTime: response.CurrentTime,
	})

	_ = rdb.Set(ctx, cacheKey, raw, 10*time.Second).Err()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func textHandler(w http.ResponseWriter, r *http.Request) {
	cacheKey := "app2:text"

	cached, err := rdb.Get(ctx, cacheKey).Result()
	if err == nil {
		cacheHits.WithLabelValues("app2").Inc()

		var response TextResponse
		if err := json.Unmarshal([]byte(cached), &response); err == nil {
			response.Cache = "hit"
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
			return
		}
	}

	cacheMisses.WithLabelValues("app2").Inc()

	response := TextResponse{
		Message: "Microservice activated: Go",
		Cache:   "miss",
	}

	raw, _ := json.Marshal(TextResponse{
		Message: response.Message,
	})

	_ = rdb.Set(ctx, cacheKey, raw, time.Minute).Err()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func main() {
	prometheus.MustRegister(httpRequestsTotal)
	prometheus.MustRegister(httpRequestDuration)
	prometheus.MustRegister(cacheHits)
	prometheus.MustRegister(cacheMisses)

	redisHost := os.Getenv("REDIS_HOST")
	if redisHost == "" {
		redisHost = "localhost"
	}

	redisPort := os.Getenv("REDIS_PORT")
	if redisPort == "" {
		redisPort = "6379"
	}

	rdb = redis.NewClient(&redis.Options{
		Addr: redisHost + ":" + redisPort,
	})

	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("failed to connect to redis: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/time", metricsMiddleware("/time", timeHandler))
	mux.HandleFunc("/text", metricsMiddleware("/text", textHandler))
	mux.Handle("/metrics", promhttp.Handler())

	server := &http.Server{
		Addr:         ":8081",
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	log.Println("Server running on :8081")
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}