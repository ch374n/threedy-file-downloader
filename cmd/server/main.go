package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/ch374n/file-downloader/internal/cache"
	"github.com/ch374n/file-downloader/internal/config"
	"github.com/ch374n/file-downloader/internal/storage"
)

var (
	fileCache   *cache.RedisCache
	fileStorage *storage.R2Client
)

type Response struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
}

func main() {
	cfg := config.Load()

	// Initialize Redis cache (optional - service works without it)
	var err error
	fileCache, err = cache.NewRedisCache(
		cfg.Redis.Addr,
		cfg.Redis.Password,
		cfg.Redis.DB,
		cfg.Redis.CacheTTL,
	)
	if err != nil {
		log.Printf("WARNING: Redis unavailable, running without cache: %v", err)
		fileCache = nil // Explicitly set to nil for clarity
	} else {
		defer func() {
			if err := fileCache.Close(); err != nil {
				log.Printf("Failed to close Redis cache: %v", err)
			}
		}()
		log.Printf("Connected to Redis at %s", cfg.Redis.Addr)
	}

	// Initialize R2 storage
	fileStorage, err = storage.NewR2Client(
		cfg.R2.AccountID,
		cfg.R2.AccessKeyID,
		cfg.R2.SecretAccessKey,
		cfg.R2.BucketName,
	)
	if err != nil {
		log.Fatalf("Failed to initialize R2 client: %v", err)
	}
	log.Printf("Connected to R2 bucket: %s", cfg.R2.BucketName)

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

	// Add timeout for the entire request (30 seconds)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// Check cache only if Redis is available
	if fileCache != nil {
		data, found, err := fileCache.Get(ctx, filename)
		if err != nil {
			log.Printf("Cache error for %s: %v", filename, err)
		}

		if found {
			log.Printf("Cache HIT for file: %s", filename)
			writeFileResponse(w, filename, data)
			return
		}

		log.Printf("Cache MISS for file: %s", filename)
	} else {
		log.Printf("Cache disabled, fetching from R2: %s", filename)
	}

	data, err := fileStorage.GetObject(ctx, filename)
	if err != nil {
		log.Printf("R2 error for %s: %v", filename, err)

		// Check if it's a timeout error
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			writeJSON(w, http.StatusGatewayTimeout, Response{
				Success: false,
				Message: "Request timeout",
			})
			return
		}

		if isNotFoundError(err) {
			writeJSON(w, http.StatusNotFound, Response{
				Success: false,
				Message: "File not found",
			})
			return
		}
		writeJSON(w, http.StatusInternalServerError, Response{
			Success: false,
			Message: "Failed to retrieve file",
		})
		return
	}

	// Cache the file only if Redis is available
	if fileCache != nil {
		go func() {
			// Use background context since HTTP request context will be cancelled
			// after response is sent
			bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := fileCache.Set(bgCtx, filename, data); err != nil {
				log.Printf("Failed to cache file %s: %v", filename, err)
			} else {
				log.Printf("Cached file: %s", filename)
			}
		}()
	}

	writeFileResponse(w, filename, data)
}

func writeFileResponse(w http.ResponseWriter, filename string, data []byte) {
	contentType := mime.TypeByExtension(filepath.Ext(filename))
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", "inline; filename=\""+filename+"\"")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}

	msg := err.Error()
	return strings.Contains(msg, "NoSuchKey") || strings.Contains(msg, "not found")
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Error encoding JSON response: %v", err)
	}
}
