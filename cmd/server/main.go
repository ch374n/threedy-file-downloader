package main

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/ch374n/file-downloader/internal/cache"
	"github.com/ch374n/file-downloader/internal/config"
)

var fileCache *cache.RedisCache

type Response struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
}

func main() {

	cfg := config.Load()

	var err error
	fileCache, err = cache.NewRedisCache(
		cfg.Redis.Addr,
		cfg.Redis.Password,
		cfg.Redis.DB,
		cfg.Redis.CacheTTL,
	)

	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	defer func(fileCache *cache.RedisCache) {
		err := fileCache.Close()
		if err != nil {
			log.Fatalf("Failed to close Redis cache: %v", err)
		}
	}(fileCache)

	log.Printf("Connected to Redis at %s", cfg.Redis.Addr)

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", healthHandler)

	mux.HandleFunc("GET /", rootHandler)

	mux.HandleFunc("GET /files/{name}", getFileHandler)

	server := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: mux,
	}

	log.Printf("Starting server on port %s", cfg.Port)

	if err = server.ListenAndServe(); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, Response{
		Success: true,
		Message: "Service is healthy",
	})
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, Response{
		Success: true,
		Message: "File Caching Service",
		Data: map[string]string{
			"version": "0.1.0",
		},
	})
}

func getFileHandler(w http.ResponseWriter, r *http.Request) {
	filename := r.PathValue("name")

	if filename == "" {
		writeJSON(w, http.StatusBadRequest, Response{
			Success: false,
			Message: "filename is required",
		})
		return
	}

	// TODO: Step 3 - Check Redis cache
	// TODO: Step 4 - If cache miss, fetch from R2
	// TODO: Step 5 - Cache the file in Redis

	log.Printf("File requested: %s", filename)

	writeJSON(w, http.StatusOK, Response{
		Success: true,
		Message: "File endpoint placeholder",
		Data: map[string]string{
			"filename": filename,
			"status":   "not_implemented",
		},
	})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Error encoding JSON response: %v", err)
	}
}
