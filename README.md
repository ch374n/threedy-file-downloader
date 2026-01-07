# File Caching Service

A production-ready HTTP file caching service built with Go that fetches files from Cloudflare R2 object storage and caches them in Redis. Designed to reduce latency and external API calls by serving frequently accessed files from an in-memory cache.

## What This Does

The service acts as a caching proxy between clients and R2 storage:

1. Client requests a file via `GET /files/{filename}`
2. Service checks Redis cache first
3. On cache hit: serves file from Redis (fast)
4. On cache miss: fetches from R2, stores in Redis, then serves to client
5. Subsequent requests for the same file are served from cache

This reduces R2 API calls and improves response times for frequently accessed files.

## Architecture

```
Client -> Service -> Redis (cache)
                  -> R2 Storage (origin)
```

The service includes:
- Redis caching with configurable TTL
- Cloudflare R2 integration for object storage
- Prometheus metrics for monitoring
- Health checks for readiness/liveness probes
- Structured JSON logging
- Security hardening (non-root user, read-only filesystem)

## Prerequisites

- Go 1.21 or later
- Docker and Docker Compose
- Kubernetes cluster (for K8s deployment)
- Helm 3 (for Kubernetes deployment)
- Make (optional, for using Makefile targets)

## Environment Variables

The service is configured via environment variables:

### Application
- `PORT` - HTTP server port (default: `8080`)
- `LOG_LEVEL` - Logging level: debug, info, warn, error (default: `info`)

### Redis Configuration
- `REDIS_MODE` - Cache mode: `enabled` or `disabled` (default: `enabled`)
- `REDIS_ADDR` - Redis server address (default: `localhost:6379`)
- `REDIS_PASSWORD` - Redis password (optional)
- `REDIS_DB` - Redis database number (default: `0`)
- `CACHE_TTL` - Cache entry TTL (default: `1h`, examples: `30m`, `2h`, `24h`)

### R2 Storage Configuration
- `R2_ACCOUNT_ID` - Cloudflare account ID (required)
- `R2_ACCESS_KEY_ID` - R2 API access key (required)
- `R2_SECRET_ACCESS_KEY` - R2 API secret key (required)
- `R2_BUCKET_NAME` - R2 bucket name (required)

## API Endpoints

### `GET /health`
Health check endpoint for liveness and readiness probes.

Returns:
- `200 OK` - Service is healthy
- Response includes Redis and R2 connection status

Example:
```bash
curl http://localhost:8080/health
```

### `GET /files/{filename}`
Fetch a file from cache or R2 storage.

Returns:
- `200 OK` - File content with appropriate Content-Type header
- `404 Not Found` - File doesn't exist in R2
- `500 Internal Server Error` - Service error

Example:
```bash
curl http://localhost:8080/files/document.pdf -o document.pdf
```

### `GET /metrics`
Prometheus metrics endpoint.

Metrics include:
- HTTP request rate, duration, and status codes
- Cache hit/miss rates
- Redis and R2 operation metrics

### `GET /`
Root endpoint returning service info.

## Running Locally

### Option 1: Using Go Directly

1. Copy the example environment file:
```bash
cp .env.example .env.local
```

2. Edit `.env.local` with your R2 credentials:
```bash
R2_ACCOUNT_ID=your-account-id
R2_ACCESS_KEY_ID=your-access-key
R2_SECRET_ACCESS_KEY=your-secret-key
R2_BUCKET_NAME=your-bucket-name
```

3. Start Redis (or set `REDIS_MODE=disabled`):
```bash
docker run -d -p 6379:6379 redis:7-alpine
```

4. Load environment and run:
```bash
export $(cat .env.local | xargs)
go run cmd/server/main.go
```

The service will be available at `http://localhost:8080`.

### Option 2: Using Docker Compose

Docker Compose runs the full stack including the app, Redis, and observability tools (Prometheus, Grafana, Loki).

1. Create `.env.docker` with your R2 credentials:
```bash
R2_ACCOUNT_ID=your-account-id
R2_ACCESS_KEY_ID=your-access-key
R2_SECRET_ACCESS_KEY=your-secret-key
R2_BUCKET_NAME=your-bucket-name
```

2. Start everything:
```bash
docker-compose -f docker-compose.observability.yml up -d
```

This starts:
- `app` - File caching service on port 8080
- `redis` - Redis cache on port 6379
- `prometheus` - Metrics collection on port 9090
- `grafana` - Visualization dashboard on port 3000
- `loki` - Log aggregation on port 3100
- `promtail` - Log shipping agent

3. Access the services:
- Application: http://localhost:8080
- Grafana: http://localhost:3000 (admin/admin)
- Prometheus: http://localhost:9090

4. Stop everything:
```bash
docker-compose -f docker-compose.observability.yml down
```

## Deploying to Kubernetes

The service includes Helm charts for production deployment.

### Quick Start

1. Build and load the Docker image:
```bash
make docker-build
```

For Kind clusters:
```bash
make kind-load-image
```

2. Deploy with Helm:
```bash
helm install file-caching-service ./helm/file-caching-service \
  --set r2.accountId="your-account-id" \
  --set r2.bucketName="your-bucket-name" \
  --set secrets.r2AccessKeyId="your-access-key" \
  --set secrets.r2SecretAccessKey="your-secret-key"
```

3. Check deployment status:
```bash
kubectl get pods
kubectl logs -f deployment/file-caching-service
```

### Helm Configuration

Edit `helm/file-caching-service/values.yaml` or use `--set` flags.

Key configuration options:

```yaml
# Number of replicas
replicaCount: 2

# Application config
config:
  port: "8080"
  cacheTTL: "1h"

# Redis (in-cluster)
redis:
  enabled: true  # Set to false to disable caching
  resources:
    limits:
      cpu: 200m
      memory: 256Mi

# R2 credentials
r2:
  accountId: "your-account-id"
  bucketName: "your-bucket-name"

secrets:
  r2AccessKeyId: "your-access-key"
  r2SecretAccessKey: "your-secret-key"

# Metrics
metrics:
  enabled: true
  serviceMonitor:
    enabled: true  # Creates Prometheus ServiceMonitor
```

### Deployment Without Redis

To run without caching (direct R2 access):

```bash
helm install file-caching-service ./helm/file-caching-service \
  --set redis.enabled=false \
  --set r2.accountId="your-account-id" \
  --set r2.bucketName="your-bucket-name" \
  --set secrets.r2AccessKeyId="your-access-key" \
  --set secrets.r2SecretAccessKey="your-secret-key"
```

### Upgrading

```bash
helm upgrade file-caching-service ./helm/file-caching-service \
  --set config.cacheTTL="2h"
```

### Uninstalling

```bash
helm uninstall file-caching-service
```

## Makefile Targets

The Makefile provides convenient shortcuts for common tasks.

### Local Development
```bash
make build                  # Build the Go binary
make run                    # Run locally
make test                   # Run unit tests
make test-coverage          # Run tests with coverage
make test-integration       # Run integration tests
make clean                  # Clean build artifacts
```

### Docker
```bash
make docker-build           # Build Docker image
make docker-up              # Start docker-compose stack
make docker-down            # Stop docker-compose stack
make docker-logs            # View logs
make docker-clean           # Stop and remove volumes
```

### Kubernetes - Full Stack
```bash
make k8s-setup              # Setup Helm repos and namespaces
make k8s-up                 # Deploy app + monitoring stack
make k8s-down               # Remove everything
make k8s-status             # Show status of all resources
```

### Kubernetes - Individual Components
```bash
make k8s-app-up             # Deploy app with Redis
make k8s-app-up-no-cache    # Deploy app without Redis
make k8s-app-down           # Remove app

make k8s-prometheus-up      # Deploy Prometheus + Grafana
make k8s-prometheus-down    # Remove Prometheus + Grafana

make k8s-loki-up            # Deploy Loki + Promtail
make k8s-loki-down          # Remove Loki + Promtail
```

### Port Forwarding
```bash
make port-forward-app           # Forward app to localhost:8080
make port-forward-grafana       # Forward Grafana to localhost:3000
make port-forward-prometheus    # Forward Prometheus to localhost:9090
make port-forward-all           # Start all port forwards
make stop-port-forwards         # Stop all port forwards
```

### Integration Testing
```bash
make kind-create            # Create Kind cluster
make kind-test              # Full integration test
make kind-test-cleanup      # Test and cleanup
make kind-delete            # Delete Kind cluster
```

### Utility
```bash
make logs-app               # View app logs from K8s
make shell-app              # Open shell in app pod
make describe-app           # Describe app pod
```

## Redis Configuration

### In-Cluster Redis (Default)

The Helm chart deploys a Redis instance alongside the application. This is suitable for development and single-cluster deployments.

Configuration in `values.yaml`:
```yaml
redis:
  enabled: true
  image: redis:7-alpine
  resources:
    limits:
      cpu: 200m
      memory: 256Mi
```

The app automatically connects to `file-caching-service-redis:6379`.

### External Redis

To use an external Redis cluster:

1. Disable in-cluster Redis:
```yaml
redis:
  enabled: false
```

2. Configure the app to connect to your Redis:
```bash
helm install file-caching-service ./helm/file-caching-service \
  --set redis.enabled=false \
  --set config.redisAddr="redis.example.com:6379" \
  --set secrets.redisPassword="your-password"
```

### No Caching

To disable caching entirely:
```bash
helm install file-caching-service ./helm/file-caching-service \
  --set redis.enabled=false
```

Set `REDIS_MODE=disabled` environment variable or omit Redis configuration.

## R2 Storage Setup

### Creating an R2 Bucket

1. Log in to Cloudflare Dashboard
2. Go to R2 Object Storage
3. Create a new bucket
4. Note your bucket name

### Creating API Tokens

1. In R2, go to "Manage R2 API Tokens"
2. Create a new API token
3. Choose permissions: Object Read & Write
4. Save the Access Key ID and Secret Access Key
5. Find your Account ID in the Cloudflare dashboard

### Uploading Test Files

Using `rclone`:
```bash
rclone copy /local/path remote:bucket-name/
```

Using AWS CLI (compatible with R2):
```bash
aws s3 cp file.txt s3://bucket-name/ \
  --endpoint-url https://<account-id>.r2.cloudflarestorage.com
```

## Monitoring and Observability

### Metrics

The service exposes Prometheus metrics at `/metrics`:

- `http_requests_total` - Total HTTP requests by method, path, status
- `http_request_duration_seconds` - Request duration histogram
- `cache_hits_total` - Cache hit counter
- `cache_misses_total` - Cache miss counter

### Grafana Dashboard

When running with docker-compose or in K8s with the monitoring stack, a pre-configured Grafana dashboard is available showing:

- Request rate and latency
- Cache hit ratio
- Error rates
- Resource usage

Access Grafana:
- Docker Compose: http://localhost:3000
- Kubernetes: `make port-forward-grafana` then http://localhost:3000
- Login: admin/admin

### Logs

The service outputs structured JSON logs:

```json
{"time":"2026-01-07T10:00:00Z","level":"INFO","msg":"Starting server","port":"8080"}
```

View logs:
```bash
# Docker
docker-compose -f docker-compose.observability.yml logs -f app

# Kubernetes
kubectl logs -f deployment/file-caching-service

# Using make
make logs-app
```

Logs are automatically collected by Promtail and sent to Loki when the observability stack is running.

## Security

The service is built with security best practices:

- Runs as non-root user (UID 1000)
- Read-only root filesystem
- Drops all Linux capabilities
- No privilege escalation
- Minimal base image (Alpine)
- No hardcoded secrets

### Secrets Management

In Kubernetes, secrets are stored in a Secret resource:

```bash
kubectl create secret generic file-caching-service \
  --from-literal=R2_ACCESS_KEY_ID=your-key \
  --from-literal=R2_SECRET_ACCESS_KEY=your-secret
```

## Troubleshooting

### Pods Not Ready

Check pod status:
```bash
kubectl get pods
kubectl describe pod <pod-name>
kubectl logs <pod-name>
```

Common issues:
- R2 credentials not configured: Health check shows R2 unhealthy but pod will still be ready
- Redis unavailable: App will start without cache if Redis fails
- Image pull error: Ensure image is built and loaded into cluster

### Cache Not Working

Verify Redis connection:
```bash
# Check Redis pod
kubectl get pods | grep redis

# Check app logs for Redis connection
kubectl logs deployment/file-caching-service | grep -i redis

# Test Redis directly
kubectl exec -it deployment/file-caching-service-redis -- redis-cli ping
```

### High Memory Usage

Redis stores files in memory. Adjust cache TTL to reduce memory usage:
```bash
helm upgrade file-caching-service ./helm/file-caching-service \
  --set config.cacheTTL="30m"
```

Or set Redis memory limits:
```yaml
redis:
  resources:
    limits:
      memory: 512Mi
```

### R2 Connection Errors

Verify credentials:
```bash
kubectl get secret file-caching-service -o yaml
```

Check R2 bucket access and permissions in Cloudflare dashboard.

## Testing

### Unit Tests
```bash
make test
```

### Integration Tests
```bash
# Start Redis
docker run -d -p 6379:6379 redis:7-alpine

# Run tests
make test-integration
```

### Full Integration Test with Kind
```bash
# Creates cluster, deploys app, runs tests, cleans up
make kind-test-cleanup
```