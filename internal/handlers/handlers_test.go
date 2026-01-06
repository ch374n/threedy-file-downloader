package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ch374n/file-downloader/internal/handlers"
	"github.com/ch374n/file-downloader/internal/mocks"
)

type TestResponse struct {
	Success bool              `json:"success"`
	Message string            `json:"message"`
	Data    map[string]string `json:"data"`
}

func parseResponse(t *testing.T, body []byte) TestResponse {
	var resp TestResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	return resp
}

func TestRootHandler(t *testing.T) {
	mockCache := mocks.NewMockCache()
	mockStorage := mocks.NewMockStorage()
	handler := handlers.NewFileHandler(mockCache, mockStorage)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.Root(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
	}

	resp := parseResponse(t, rec.Body.Bytes())
	if !resp.Success {
		t.Error("Expected success to be true")
	}
	if resp.Message != "File Caching Service" {
		t.Errorf("Expected message 'File Caching Service', got '%s'", resp.Message)
	}
	if resp.Data["version"] != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got '%s'", resp.Data["version"])
	}
}

func TestHealthHandler_AllHealthy(t *testing.T) {
	mockCache := mocks.NewMockCache()
	mockStorage := mocks.NewMockStorage()
	handler := handlers.NewFileHandler(mockCache, mockStorage)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handler.Health(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
	}

	resp := parseResponse(t, rec.Body.Bytes())
	if !resp.Success {
		t.Error("Expected success to be true")
	}
	if resp.Data["status"] != "healthy" {
		t.Errorf("Expected status 'healthy', got '%s'", resp.Data["status"])
	}
	if resp.Data["redis"] != "healthy" {
		t.Errorf("Expected redis 'healthy', got '%s'", resp.Data["redis"])
	}
	if resp.Data["r2"] != "healthy" {
		t.Errorf("Expected r2 'healthy', got '%s'", resp.Data["r2"])
	}
}

func TestHealthHandler_CacheDisabled(t *testing.T) {
	mockStorage := mocks.NewMockStorage()
	handler := handlers.NewFileHandler(nil, mockStorage) // No cache

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handler.Health(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
	}

	resp := parseResponse(t, rec.Body.Bytes())
	if !resp.Success {
		t.Error("Expected success to be true")
	}
	if resp.Data["redis"] != "disabled" {
		t.Errorf("Expected redis 'disabled', got '%s'", resp.Data["redis"])
	}
}

func TestHealthHandler_CacheUnhealthy(t *testing.T) {
	mockCache := mocks.NewMockCache()
	mockCache.PingError = mocks.ErrCacheUnavailable
	mockStorage := mocks.NewMockStorage()
	handler := handlers.NewFileHandler(mockCache, mockStorage)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handler.Health(rec, req)

	// Service should still be healthy if only cache is down
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
	}

	resp := parseResponse(t, rec.Body.Bytes())
	if !resp.Success {
		t.Error("Expected success to be true (cache is optional)")
	}
	if resp.Data["status"] != "healthy" {
		t.Errorf("Expected status 'healthy', got '%s'", resp.Data["status"])
	}
}

func TestHealthHandler_StorageUnhealthy(t *testing.T) {
	mockCache := mocks.NewMockCache()
	mockStorage := mocks.NewMockStorage()
	mockStorage.HealthCheckError = mocks.ErrBucketNotFound
	handler := handlers.NewFileHandler(mockCache, mockStorage)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handler.Health(rec, req)

	// Service should be unhealthy if storage is down
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, rec.Code)
	}

	resp := parseResponse(t, rec.Body.Bytes())
	if resp.Success {
		t.Error("Expected success to be false")
	}
	if resp.Data["status"] != "unhealthy" {
		t.Errorf("Expected status 'unhealthy', got '%s'", resp.Data["status"])
	}
}

func TestGetFile_EmptyFilename(t *testing.T) {
	mockCache := mocks.NewMockCache()
	mockStorage := mocks.NewMockStorage()
	handler := handlers.NewFileHandler(mockCache, mockStorage)

	req := httptest.NewRequest(http.MethodGet, "/files/", nil)
	req.SetPathValue("name", "") // Empty filename
	rec := httptest.NewRecorder()

	handler.GetFile(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}

	resp := parseResponse(t, rec.Body.Bytes())
	if resp.Success {
		t.Error("Expected success to be false")
	}
	if resp.Message != "filename is required" {
		t.Errorf("Expected message 'filename is required', got '%s'", resp.Message)
	}
}

func TestGetFile_CacheHit(t *testing.T) {
	mockCache := mocks.NewMockCache()
	mockStorage := mocks.NewMockStorage()
	handler := handlers.NewFileHandler(mockCache, mockStorage)

	// Pre-populate cache
	testData := []byte("cached file content")
	mockCache.SetData("test.txt", testData)

	req := httptest.NewRequest(http.MethodGet, "/files/test.txt", nil)
	req.SetPathValue("name", "test.txt")
	rec := httptest.NewRecorder()

	handler.GetFile(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
	}

	// Verify response body
	if rec.Body.String() != string(testData) {
		t.Errorf("Expected body '%s', got '%s'", testData, rec.Body.String())
	}

	// Verify cache was checked
	if len(mockCache.GetCalls) != 1 {
		t.Errorf("Expected 1 cache get call, got %d", len(mockCache.GetCalls))
	}

	// Verify storage was NOT called (cache hit)
	if len(mockStorage.GetCalls) != 0 {
		t.Errorf("Expected 0 storage get calls, got %d", len(mockStorage.GetCalls))
	}
}

func TestGetFile_CacheMiss_StorageHit(t *testing.T) {
	mockCache := mocks.NewMockCache()
	mockStorage := mocks.NewMockStorage()
	handler := handlers.NewFileHandler(mockCache, mockStorage)

	// Pre-populate storage (not cache)
	testData := []byte("storage file content")
	mockStorage.SetObject("test.txt", testData)

	req := httptest.NewRequest(http.MethodGet, "/files/test.txt", nil)
	req.SetPathValue("name", "test.txt")
	rec := httptest.NewRecorder()

	handler.GetFile(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
	}

	// Verify response body
	if rec.Body.String() != string(testData) {
		t.Errorf("Expected body '%s', got '%s'", testData, rec.Body.String())
	}

	// Verify cache was checked first
	if len(mockCache.GetCalls) != 1 {
		t.Errorf("Expected 1 cache get call, got %d", len(mockCache.GetCalls))
	}

	// Verify storage was called
	if len(mockStorage.GetCalls) != 1 {
		t.Errorf("Expected 1 storage get call, got %d", len(mockStorage.GetCalls))
	}

}

func TestGetFile_NoCacheConfigured(t *testing.T) {
	mockStorage := mocks.NewMockStorage()
	handler := handlers.NewFileHandler(nil, mockStorage) // No cache

	testData := []byte("storage file content")
	mockStorage.SetObject("test.txt", testData)

	req := httptest.NewRequest(http.MethodGet, "/files/test.txt", nil)
	req.SetPathValue("name", "test.txt")
	rec := httptest.NewRecorder()

	handler.GetFile(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
	}

	// Verify response body
	if rec.Body.String() != string(testData) {
		t.Errorf("Expected body '%s', got '%s'", testData, rec.Body.String())
	}

	// Verify storage was called directly
	if len(mockStorage.GetCalls) != 1 {
		t.Errorf("Expected 1 storage get call, got %d", len(mockStorage.GetCalls))
	}
}

func TestGetFile_FileNotFound(t *testing.T) {
	mockCache := mocks.NewMockCache()
	mockStorage := mocks.NewMockStorage()
	handler := handlers.NewFileHandler(mockCache, mockStorage)

	// Don't add any files - storage is empty

	req := httptest.NewRequest(http.MethodGet, "/files/nonexistent.txt", nil)
	req.SetPathValue("name", "nonexistent.txt")
	rec := httptest.NewRecorder()

	handler.GetFile(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, rec.Code)
	}

	resp := parseResponse(t, rec.Body.Bytes())
	if resp.Success {
		t.Error("Expected success to be false")
	}
	if resp.Message != "File not found" {
		t.Errorf("Expected message 'File not found', got '%s'", resp.Message)
	}
}

func TestGetFile_StorageError(t *testing.T) {
	mockCache := mocks.NewMockCache()
	mockStorage := mocks.NewMockStorage()
	mockStorage.GetError = mocks.ErrStorageError
	handler := handlers.NewFileHandler(mockCache, mockStorage)

	req := httptest.NewRequest(http.MethodGet, "/files/test.txt", nil)
	req.SetPathValue("name", "test.txt")
	rec := httptest.NewRecorder()

	handler.GetFile(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("Expected status %d, got %d", http.StatusInternalServerError, rec.Code)
	}

	resp := parseResponse(t, rec.Body.Bytes())
	if resp.Success {
		t.Error("Expected success to be false")
	}
}

func TestGetFile_CacheErrorFallsBackToStorage(t *testing.T) {
	mockCache := mocks.NewMockCache()
	mockCache.GetError = mocks.ErrCacheUnavailable
	mockStorage := mocks.NewMockStorage()
	handler := handlers.NewFileHandler(mockCache, mockStorage)

	testData := []byte("storage file content")
	mockStorage.SetObject("test.txt", testData)

	req := httptest.NewRequest(http.MethodGet, "/files/test.txt", nil)
	req.SetPathValue("name", "test.txt")
	rec := httptest.NewRecorder()

	handler.GetFile(rec, req)

	// Should still succeed by falling back to storage
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
	}

	// Verify response body
	if rec.Body.String() != string(testData) {
		t.Errorf("Expected body '%s', got '%s'", testData, rec.Body.String())
	}
}

func TestGetFile_ContentType_PDF(t *testing.T) {
	mockStorage := mocks.NewMockStorage()
	handler := handlers.NewFileHandler(nil, mockStorage)

	mockStorage.SetObject("document.pdf", []byte("%PDF-1.4"))

	req := httptest.NewRequest(http.MethodGet, "/files/document.pdf", nil)
	req.SetPathValue("name", "document.pdf")
	rec := httptest.NewRecorder()

	handler.GetFile(rec, req)

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/pdf" {
		t.Errorf("Expected Content-Type 'application/pdf', got '%s'", contentType)
	}
}

func TestGetFile_ContentType_HTML(t *testing.T) {
	mockStorage := mocks.NewMockStorage()
	handler := handlers.NewFileHandler(nil, mockStorage)

	mockStorage.SetObject("page.html", []byte("<html></html>"))

	req := httptest.NewRequest(http.MethodGet, "/files/page.html", nil)
	req.SetPathValue("name", "page.html")
	rec := httptest.NewRecorder()

	handler.GetFile(rec, req)

	contentType := rec.Header().Get("Content-Type")
	if contentType != "text/html; charset=utf-8" {
		t.Errorf("Expected Content-Type 'text/html; charset=utf-8', got '%s'", contentType)
	}
}

func TestGetFile_ContentType_Unknown(t *testing.T) {
	mockStorage := mocks.NewMockStorage()
	handler := handlers.NewFileHandler(nil, mockStorage)

	mockStorage.SetObject("file.unknownext123", []byte("unknown content"))

	req := httptest.NewRequest(http.MethodGet, "/files/file.unknownext123", nil)
	req.SetPathValue("name", "file.unknownext123")
	rec := httptest.NewRecorder()

	handler.GetFile(rec, req)

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/octet-stream" {
		t.Errorf("Expected Content-Type 'application/octet-stream', got '%s'", contentType)
	}
}

func TestGetFile_ContentDisposition(t *testing.T) {
	mockStorage := mocks.NewMockStorage()
	handler := handlers.NewFileHandler(nil, mockStorage)

	mockStorage.SetObject("report.pdf", []byte("%PDF"))

	req := httptest.NewRequest(http.MethodGet, "/files/report.pdf", nil)
	req.SetPathValue("name", "report.pdf")
	rec := httptest.NewRecorder()

	handler.GetFile(rec, req)

	disposition := rec.Header().Get("Content-Disposition")
	expected := `inline; filename="report.pdf"`
	if disposition != expected {
		t.Errorf("Expected Content-Disposition '%s', got '%s'", expected, disposition)
	}
}

func TestGetFile_CacheSetError_StillSucceeds(t *testing.T) {
	mockCache := mocks.NewMockCache()
	mockCache.SetError = mocks.ErrCacheUnavailable
	mockStorage := mocks.NewMockStorage()
	handler := handlers.NewFileHandler(mockCache, mockStorage)

	testData := []byte("file content")
	mockStorage.SetObject("test.txt", testData)

	req := httptest.NewRequest(http.MethodGet, "/files/test.txt", nil)
	req.SetPathValue("name", "test.txt")
	rec := httptest.NewRecorder()

	handler.GetFile(rec, req)

	// Request should still succeed even if cache set fails
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
	}

	// Verify response body
	if rec.Body.String() != string(testData) {
		t.Errorf("Expected body '%s', got '%s'", testData, rec.Body.String())
	}
}

func BenchmarkGetFile_CacheHit(b *testing.B) {
	mockCache := mocks.NewMockCache()
	mockStorage := mocks.NewMockStorage()
	handler := handlers.NewFileHandler(mockCache, mockStorage)

	mockCache.SetData("test.txt", []byte("benchmark content"))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/files/test.txt", nil)
		req.SetPathValue("name", "test.txt")
		rec := httptest.NewRecorder()
		handler.GetFile(rec, req)
	}
}

func BenchmarkGetFile_CacheMiss(b *testing.B) {
	mockCache := mocks.NewMockCache()
	mockStorage := mocks.NewMockStorage()
	handler := handlers.NewFileHandler(mockCache, mockStorage)

	mockStorage.SetObject("test.txt", []byte("benchmark content"))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mockCache.ClearData() // Ensure cache miss
		req := httptest.NewRequest(http.MethodGet, "/files/test.txt", nil)
		req.SetPathValue("name", "test.txt")
		rec := httptest.NewRecorder()
		handler.GetFile(rec, req)
	}
}
