package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/ch374n/file-downloader/internal/cache"
	"github.com/ch374n/file-downloader/internal/config"
	"github.com/ch374n/file-downloader/internal/logger"
	"github.com/ch374n/file-downloader/internal/metrics"
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

	// Initialize structured logger
	logger.Init(cfg.LogLevel)

	// Initialize Redis cache based on mode
	var err error
	switch cfg.Redis.Mode {
	case config.RedisModeDisabled:
		slog.Info("Redis caching disabled")
		fileCache = nil
	case config.RedisModeEnabled:
		fileCache, err = cache.NewRedisCache(cache.RedisConfig{
			Addr:         cfg.Redis.Addr,
			Password:     cfg.Redis.Password,
			DB:           cfg.Redis.DB,
			TTL:          cfg.Redis.CacheTTL,
			DialTimeout:  cfg.Redis.DialTimeout,
			ReadTimeout:  cfg.Redis.ReadTimeout,
			WriteTimeout: cfg.Redis.WriteTimeout,
		})
		if err != nil {
			slog.Warn("Redis unavailable, running without cache",
				"addr", cfg.Redis.Addr,
				"error", err,
			)
			fileCache = nil
		} else {
			defer func() {
				if err := fileCache.Close(); err != nil {
					slog.Error("Failed to close Redis cache", "error", err)
				}
			}()
			slog.Info("Connected to Redis", "addr", cfg.Redis.Addr)
		}
	}

	// Initialize R2 storage
	fileStorage, err = storage.NewR2Client(
		cfg.R2.AccountID,
		cfg.R2.AccessKeyID,
		cfg.R2.SecretAccessKey,
		cfg.R2.BucketName,
	)
	if err != nil {
		slog.Error("Failed to initialize R2 client", "error", err)
		panic(err)
	}
	slog.Info("Connected to R2 bucket", "bucket", cfg.R2.BucketName)

	mux := http.NewServeMux()

	// Endpoints
	mux.HandleFunc("GET /health", healthHandler)
	mux.HandleFunc("GET /", rootHandler)
	mux.HandleFunc("GET /files/{name}", metricsMiddleware(getFileHandler))

	// Prometheus metrics endpoint
	mux.Handle("GET /metrics", promhttp.Handler())

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	slog.Info("Starting server", "port", cfg.Port)

	if err = server.ListenAndServe(); err != nil {
		slog.Error("Server failed to start", "error", err)
		panic(err)
	}
}

// metricsMiddleware wraps a handler to record HTTP metrics
func metricsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next(wrapped, r)

		duration := time.Since(start).Seconds()
		path := r.URL.Path
		method := r.Method
		status := strconv.Itoa(wrapped.statusCode)

		metrics.HTTPRequestsTotal.WithLabelValues(method, path, status).Inc()
		metrics.HTTPRequestDuration.WithLabelValues(method, path).Observe(duration)

		slog.Info("Request completed",
			"method", method,
			"path", path,
			"status", wrapped.statusCode,
			"duration_ms", duration*1000,
		)
	}
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	health := map[string]string{
		"status": "healthy",
	}

	// Check Redis (optional - doesn't affect overall health)
	if fileCache != nil {
		if err := fileCache.Ping(ctx); err != nil {
			health["redis"] = "unhealthy: " + err.Error()
		} else {
			health["redis"] = "healthy"
		}
	} else {
		health["redis"] = "disabled"
	}

	// Check R2 (required - affects overall health)
	if err := fileStorage.HealthCheck(ctx); err != nil {
		health["status"] = "unhealthy"
		health["r2"] = "unhealthy: " + err.Error()
		writeJSON(w, http.StatusServiceUnavailable, Response{
			Success: false,
			Message: "Service is unhealthy",
			Data:    health,
		})
		return
	}
	health["r2"] = "healthy"

	writeJSON(w, http.StatusOK, Response{
		Success: true,
		Message: "Service is healthy",
		Data:    health,
	})
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, Response{
		Success: true,
		Message: "File Caching Service",
		Data: map[string]string{
			"version": "1.0.0",
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

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// Check cache only if Redis is available
	if fileCache != nil {
		start := time.Now()
		data, found, err := fileCache.Get(ctx, filename)
		metrics.CacheOperationDuration.WithLabelValues("get").Observe(time.Since(start).Seconds())

		if err != nil {
			slog.Error("Cache error", "filename", filename, "error", err)
		}

		if found {
			metrics.CacheHitsTotal.Inc()
			slog.Info("Cache HIT", "filename", filename)
			writeFileResponse(w, filename, data)
			return
		}

		metrics.CacheMissesTotal.Inc()
		slog.Info("Cache MISS", "filename", filename)
	} else {
		slog.Info("Cache disabled, fetching from R2", "filename", filename)
	}

	// Fetch from R2
	start := time.Now()
	data, err := fileStorage.GetObject(ctx, filename)
	duration := time.Since(start).Seconds()
	metrics.R2RequestDuration.WithLabelValues("get").Observe(duration)

	if err != nil {
		metrics.R2RequestsTotal.WithLabelValues("get", "error").Inc()
		slog.Error("R2 error", "filename", filename, "error", err)

		if ctx.Err() == context.DeadlineExceeded {
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

	metrics.R2RequestsTotal.WithLabelValues("get", "success").Inc()

	// Cache the file only if Redis is available
	if fileCache != nil {
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			start := time.Now()
			if err := fileCache.Set(bgCtx, filename, data); err != nil {
				slog.Error("Failed to cache file", "filename", filename, "error", err)
			} else {
				slog.Info("Cached file", "filename", filename)
			}
			metrics.CacheOperationDuration.WithLabelValues("set").Observe(time.Since(start).Seconds())
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
	return strings.Contains(err.Error(), "NoSuchKey") ||
		strings.Contains(err.Error(), "not found")
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("Error encoding JSON response", "error", err)
	}
}
