# Makefile for File Caching Service
# Usage: make <target>

.PHONY: help build run clean \
        docker-build docker-up docker-down docker-logs \
        k8s-setup k8s-up k8s-down k8s-status \
        k8s-prometheus-up k8s-prometheus-down \
        k8s-loki-up k8s-loki-down \
        k8s-app-up k8s-app-down \
        port-forward-grafana port-forward-app port-forward-prometheus \
        stop-port-forwards

# Variables
APP_NAME := file-caching-service
DOCKER_IMAGE := $(APP_NAME):latest
NAMESPACE := default
MONITORING_NAMESPACE := monitoring
HELM_RELEASE := $(APP_NAME)
PROMETHEUS_RELEASE := prometheus
LOKI_RELEASE := loki

# Colors for output
GREEN := \033[0;32m
YELLOW := \033[0;33m
RED := \033[0;31m
NC := \033[0m # No Color

help: ## Show this help message
	@echo "$(GREEN)File Caching Service - Makefile Commands$(NC)"
	@echo ""
	@echo "$(YELLOW)Local Development:$(NC)"
	@echo "  $(GREEN)build$(NC)                     Build the Go application locally"
	@echo "  $(GREEN)run$(NC)                       Run the application locally"
	@echo "  $(GREEN)test$(NC)                      Run unit tests"
	@echo "  $(GREEN)test-coverage$(NC)             Run tests with coverage report"
	@echo "  $(GREEN)test-coverage-html$(NC)        Generate HTML coverage report"
	@echo "  $(GREEN)test-integration$(NC)          Run integration tests (requires Redis)"
	@echo "  $(GREEN)test-bench$(NC)                Run benchmark tests"
	@echo "  $(GREEN)clean$(NC)                     Clean build artifacts"
	@echo ""
	@echo "$(YELLOW)Kind Integration Tests:$(NC)"
	@echo "  $(GREEN)kind-create$(NC)               Create Kind cluster"
	@echo "  $(GREEN)kind-delete$(NC)               Delete Kind cluster"
	@echo "  $(GREEN)kind-load-image$(NC)           Build and load Docker image into Kind"
	@echo "  $(GREEN)kind-deploy$(NC)               Deploy app to Kind with Helm"
	@echo "  $(GREEN)kind-wait-ready$(NC)           Wait for pods to be ready"
	@echo "  $(GREEN)kind-port-forward$(NC)         Start port-forward to Kind"
	@echo "  $(GREEN)kind-run-tests$(NC)            Run integration tests"
	@echo "  $(GREEN)kind-test$(NC)                 Full integration test pipeline"
	@echo "  $(GREEN)kind-test-cleanup$(NC)         Full test with cleanup"
	@echo ""
	@echo "$(YELLOW)Docker:$(NC)"
	@echo "  $(GREEN)docker-build$(NC)              Build Docker image"
	@echo "  $(GREEN)docker-up$(NC)                 Start Docker Compose (app + observability)"
	@echo "  $(GREEN)docker-down$(NC)               Stop Docker Compose"
	@echo "  $(GREEN)docker-logs$(NC)               View Docker Compose logs"
	@echo "  $(GREEN)docker-logs-app$(NC)           View app logs only"
	@echo "  $(GREEN)docker-clean$(NC)              Stop and remove Docker volumes"
	@echo ""
	@echo "$(YELLOW)Kubernetes - Full Stack:$(NC)"
	@echo "  $(GREEN)k8s-setup$(NC)                 Initial setup (helm repos, namespaces)"
	@echo "  $(GREEN)k8s-up$(NC)                    Deploy everything to Kubernetes"
	@echo "  $(GREEN)k8s-down$(NC)                  Remove everything from Kubernetes"
	@echo "  $(GREEN)k8s-status$(NC)                Show status of all resources"
	@echo ""
	@echo "$(YELLOW)Kubernetes - Components:$(NC)"
	@echo "  $(GREEN)k8s-prometheus-up$(NC)         Deploy Prometheus + Grafana"
	@echo "  $(GREEN)k8s-prometheus-down$(NC)       Remove Prometheus + Grafana"
	@echo "  $(GREEN)k8s-loki-up$(NC)               Deploy Loki + Promtail"
	@echo "  $(GREEN)k8s-loki-down$(NC)             Remove Loki + Promtail"
	@echo "  $(GREEN)k8s-app-up$(NC)                Deploy app with Redis"
	@echo "  $(GREEN)k8s-app-up-no-cache$(NC)       Deploy app without Redis cache"
	@echo "  $(GREEN)k8s-app-down$(NC)              Remove application"
	@echo ""
	@echo "$(YELLOW)Port Forwarding:$(NC)"
	@echo "  $(GREEN)port-forward-grafana$(NC)      Forward Grafana (localhost:3000)"
	@echo "  $(GREEN)port-forward-prometheus$(NC)   Forward Prometheus (localhost:9090)"
	@echo "  $(GREEN)port-forward-loki$(NC)         Forward Loki (localhost:3100)"
	@echo "  $(GREEN)port-forward-app$(NC)          Forward app (localhost:8080)"
	@echo "  $(GREEN)port-forward-all$(NC)          Start all port forwards in background"
	@echo "  $(GREEN)stop-port-forwards$(NC)        Stop all background port forwards"
	@echo ""
	@echo "$(YELLOW)Utility:$(NC)"
	@echo "  $(GREEN)logs-app$(NC)                  View application logs from K8s"
	@echo "  $(GREEN)logs-prometheus$(NC)           View Prometheus logs"
	@echo "  $(GREEN)logs-loki$(NC)                 View Loki logs"
	@echo "  $(GREEN)logs-promtail$(NC)             View Promtail logs"
	@echo "  $(GREEN)shell-app$(NC)                 Open shell in application pod"
	@echo "  $(GREEN)describe-app$(NC)              Describe application pod"

build: ## Build the Go application locally
	@echo "$(GREEN)Building Go application...$(NC)"
	go build -o bin/$(APP_NAME) cmd/server/main.go

run: ## Run the application locally
	@echo "$(GREEN)Running application...$(NC)"
	go run cmd/server/main.go

test: ## Run tests
	@echo "$(GREEN)Running tests...$(NC)"
	go test -v ./internal/... -short

test-coverage: ## Run tests with coverage
	@echo "$(GREEN)Running tests with coverage...$(NC)"
	go test -v ./internal/... -short -coverprofile=coverage.out -covermode=atomic
	go tool cover -func=coverage.out
	@echo "$(GREEN)Coverage report generated: coverage.out$(NC)"

test-coverage-html: test-coverage ## Generate HTML coverage report
	@echo "$(GREEN)Generating HTML coverage report...$(NC)"
	go tool cover -html=coverage.out -o coverage.html
	@echo "$(GREEN)Coverage report: coverage.html$(NC)"
	open coverage.html

test-integration: ## Run integration tests (requires Redis)
	@echo "$(GREEN)Running integration tests...$(NC)"
	go test -v ./internal/...

test-bench: ## Run benchmark tests
	@echo "$(GREEN)Running benchmark tests...$(NC)"
	go test -bench=. -benchmem ./internal/handlers/

KIND_CLUSTER_NAME := file-caching-test

kind-create: ## Create Kind cluster for integration testing
	@echo "$(GREEN)Creating Kind cluster: $(KIND_CLUSTER_NAME)...$(NC)"
	@if kind get clusters 2>/dev/null | grep -q "^$(KIND_CLUSTER_NAME)$$"; then \
		echo "$(YELLOW)Cluster already exists$(NC)"; \
	else \
		kind create cluster --name $(KIND_CLUSTER_NAME) --wait 60s; \
	fi
	kubectl cluster-info --context kind-$(KIND_CLUSTER_NAME)

kind-delete: ## Delete Kind cluster
	@echo "$(YELLOW)Deleting Kind cluster: $(KIND_CLUSTER_NAME)...$(NC)"
	kind delete cluster --name $(KIND_CLUSTER_NAME) 2>/dev/null || true

kind-load-image: docker-build ## Build and load Docker image into Kind cluster
	@echo "$(GREEN)Loading image into Kind cluster...$(NC)"
	kind load docker-image $(DOCKER_IMAGE) --name $(KIND_CLUSTER_NAME)

kind-deploy: ## Deploy application to Kind cluster using Helm
	@echo "$(GREEN)Deploying to Kind cluster...$(NC)"
	@if [ -z "$(R2_ACCOUNT_ID)" ]; then echo "$(RED)Error: R2_ACCOUNT_ID is required$(NC)"; exit 1; fi
	@if [ -z "$(R2_ACCESS_KEY_ID)" ]; then echo "$(RED)Error: R2_ACCESS_KEY_ID is required$(NC)"; exit 1; fi
	@if [ -z "$(R2_SECRET_ACCESS_KEY)" ]; then echo "$(RED)Error: R2_SECRET_ACCESS_KEY is required$(NC)"; exit 1; fi
	@if [ -z "$(R2_BUCKET_NAME)" ]; then echo "$(RED)Error: R2_BUCKET_NAME is required$(NC)"; exit 1; fi
	@helm upgrade --install $(HELM_RELEASE) ./helm/$(APP_NAME) \
		--namespace $(NAMESPACE) \
		--set image.repository=$(APP_NAME) \
		--set image.tag=latest \
		--set image.pullPolicy=Never \
		--set replicaCount=1 \
		--set redis.enabled=true \
		--set metrics.serviceMonitor.enabled=false \
		--set r2.accountId="$(R2_ACCOUNT_ID)" \
		--set r2.bucketName="$(R2_BUCKET_NAME)" \
		--set secrets.r2AccessKeyId="$(R2_ACCESS_KEY_ID)" \
		--set secrets.r2SecretAccessKey="$(R2_SECRET_ACCESS_KEY)" \
		--wait --timeout 5m
	@echo "$(GREEN)Deployment complete!$(NC)"

kind-wait-ready: ## Wait for pods to be ready in Kind cluster
	@echo "$(GREEN)Waiting for pods to be ready...$(NC)"
	kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=$(APP_NAME) -n $(NAMESPACE) --timeout=180s
	kubectl wait --for=condition=ready pod -l app.kubernetes.io/component=redis -n $(NAMESPACE) --timeout=180s
	@echo "$(GREEN)Checking pod status...$(NC)"
	kubectl get pods -n $(NAMESPACE)
	@echo "$(GREEN)Checking pod logs...$(NC)"
	kubectl logs -l app.kubernetes.io/name=$(APP_NAME) -n $(NAMESPACE) --tail=20 || true

kind-port-forward: ## Start port-forward to Kind cluster (background)
	@echo "$(GREEN)Starting port-forward...$(NC)"
	@pkill -f "kubectl port-forward" 2>/dev/null || true
	@sleep 2
	@echo "$(GREEN)Port-forwarding to service...$(NC)"
	@kubectl port-forward svc/$(HELM_RELEASE) 8080:80 -n $(NAMESPACE) &
	@sleep 5
	@echo "$(GREEN)Testing connection...$(NC)"
	@curl -s http://localhost:8080/health || echo "$(RED)Connection test failed$(NC)"
	@echo "$(GREEN)Service available at http://localhost:8080$(NC)"

kind-run-tests: ## Run integration tests against Kind cluster
	@echo "$(GREEN)Running integration tests...$(NC)"
	SERVICE_URL=http://localhost:8080 TEST_FILE_NAME=$(TEST_FILE_NAME) go test -v ./tests/integration/... -timeout 5m

kind-test: kind-create kind-load-image kind-deploy kind-wait-ready kind-port-forward kind-run-tests ## Full integration test pipeline
	@echo "$(GREEN)Integration tests completed!$(NC)"

kind-test-cleanup: kind-test kind-delete ## Full integration test with cleanup
	@echo "$(GREEN)Integration tests completed and cluster deleted!$(NC)"

clean: ## Clean build artifacts
	@echo "$(GREEN)Cleaning...$(NC)"
	rm -rf bin/
	rm -rf logs/

docker-build: ## Build Docker image
	@echo "$(GREEN)Building Docker image...$(NC)"
	docker build -t $(DOCKER_IMAGE) .

docker-up: docker-build ## Start Docker Compose (app + observability stack)
	@echo "$(GREEN)Starting Docker Compose stack...$(NC)"
	docker compose -f docker-compose.observability.yml up -d
	@echo "$(GREEN)Services started!$(NC)"
	@echo "  - App:        http://localhost:8080"
	@echo "  - Grafana:    http://localhost:3000 (admin/admin)"
	@echo "  - Prometheus: http://localhost:9090"
	@echo "  - Loki:       http://localhost:3100"

docker-down: ## Stop Docker Compose
	@echo "$(YELLOW)Stopping Docker Compose stack...$(NC)"
	docker compose -f docker-compose.observability.yml down

docker-logs: ## View Docker Compose logs
	docker compose -f docker-compose.observability.yml logs -f

docker-logs-app: ## View app logs only
	docker compose -f docker-compose.observability.yml logs -f app

docker-clean: docker-down ## Stop and remove Docker volumes
	@echo "$(RED)Removing Docker volumes...$(NC)"
	docker compose -f docker-compose.observability.yml down -v
	docker rmi $(DOCKER_IMAGE) 2>/dev/null || true

k8s-setup: ## Initial Kubernetes setup (add helm repos, create namespaces)
	@echo "$(GREEN)Setting up Kubernetes...$(NC)"
	@echo "Adding Helm repositories..."
	helm repo add prometheus-community https://prometheus-community.github.io/helm-charts 2>/dev/null || true
	helm repo add grafana https://grafana.github.io/helm-charts 2>/dev/null || true
	helm repo update
	@echo "Creating namespaces..."
	kubectl create namespace $(MONITORING_NAMESPACE) 2>/dev/null || true
	kubectl create namespace $(NAMESPACE) 2>/dev/null || true
	@echo "$(GREEN)Setup complete!$(NC)"

k8s-up: k8s-setup k8s-prometheus-up k8s-loki-up k8s-app-up ## Deploy everything to Kubernetes
	@echo "$(GREEN)Full Kubernetes stack deployed!$(NC)"
	@make k8s-status

k8s-down: k8s-app-down k8s-loki-down k8s-prometheus-down ## Remove everything from Kubernetes
	@echo "$(YELLOW)Full Kubernetes stack removed!$(NC)"

k8s-status: ## Show status of all Kubernetes resources
	@echo "$(GREEN)=== Monitoring Namespace ===$(NC)"
	kubectl get pods -n $(MONITORING_NAMESPACE)
	@echo ""
	@echo "$(GREEN)=== Application Namespace ===$(NC)"
	kubectl get pods -n $(NAMESPACE)
	@echo ""
	@echo "$(GREEN)=== Helm Releases ===$(NC)"
	helm list -A

k8s-prometheus-up: k8s-setup ## Deploy Prometheus + Grafana to Kubernetes
	@echo "$(GREEN)Deploying Prometheus stack...$(NC)"
	helm upgrade --install $(PROMETHEUS_RELEASE) prometheus-community/kube-prometheus-stack \
		--namespace $(MONITORING_NAMESPACE) \
		-f k8s/monitoring/prometheus-values.yaml
	@echo "$(GREEN)Prometheus + Grafana deployed!$(NC)"
	@echo "$(YELLOW)Note: Pods may take a few minutes to become ready. Run 'make k8s-status' to check.$(NC)"

k8s-prometheus-down: ## Remove Prometheus + Grafana from Kubernetes
	@echo "$(YELLOW)Removing Prometheus stack...$(NC)"
	helm uninstall $(PROMETHEUS_RELEASE) -n $(MONITORING_NAMESPACE) 2>/dev/null || true
	@echo "$(GREEN)Prometheus stack removed!$(NC)"

k8s-loki-up: k8s-setup ## Deploy Loki + Promtail to Kubernetes
	@echo "$(GREEN)Deploying Loki stack...$(NC)"
	helm upgrade --install $(LOKI_RELEASE) grafana/loki-stack \
		--namespace $(MONITORING_NAMESPACE) \
		-f k8s/monitoring/loki-values.yaml
	@echo "$(GREEN)Loki + Promtail deployed!$(NC)"
	@echo "$(YELLOW)Note: Pods may take a few minutes to become ready. Run 'make k8s-status' to check.$(NC)"

k8s-loki-down: ## Remove Loki + Promtail from Kubernetes
	@echo "$(YELLOW)Removing Loki stack...$(NC)"
	helm uninstall $(LOKI_RELEASE) -n $(MONITORING_NAMESPACE) 2>/dev/null || true
	@echo "$(GREEN)Loki stack removed!$(NC)"

k8s-app-up: ## Deploy application with Redis to Kubernetes
	@echo "$(GREEN)Deploying application with Redis...$(NC)"
	helm upgrade --install $(HELM_RELEASE) ./helm/$(APP_NAME) \
		--namespace $(NAMESPACE) \
		-f helm/$(APP_NAME)/values.yaml \
		--set redis.enabled=true \
		--set image.repository=$(APP_NAME) \
		--set image.tag=latest
	@echo "$(GREEN)Application deployed with Redis!$(NC)"
	@echo "$(YELLOW)Note: Pods may take a few minutes to become ready. Run 'make k8s-status' to check.$(NC)"

k8s-app-up-no-cache: ## Deploy application without Redis cache
	@echo "$(GREEN)Deploying application without Redis cache...$(NC)"
	helm upgrade --install $(HELM_RELEASE) ./helm/$(APP_NAME) \
		--namespace $(NAMESPACE) \
		-f helm/$(APP_NAME)/values.yaml \
		--set redis.enabled=false \
		--set image.repository=$(APP_NAME) \
		--set image.tag=latest
	@echo "$(GREEN)Application deployed without cache!$(NC)"
	@echo "$(YELLOW)Note: Pods may take a few minutes to become ready. Run 'make k8s-status' to check.$(NC)"

k8s-app-down: ## Remove application from Kubernetes
	@echo "$(YELLOW)Removing application...$(NC)"
	helm uninstall $(HELM_RELEASE) -n $(NAMESPACE) 2>/dev/null || true
	@echo "$(GREEN)Application removed!$(NC)"

port-forward-grafana: ## Port forward Grafana (http://localhost:3000)
	@echo "$(GREEN)Port forwarding Grafana to http://localhost:3000$(NC)"
	@echo "$(YELLOW)Press Ctrl+C to stop$(NC)"
	kubectl port-forward svc/$(PROMETHEUS_RELEASE)-grafana 3000:80 -n $(MONITORING_NAMESPACE)

port-forward-prometheus: ## Port forward Prometheus (http://localhost:9090)
	@echo "$(GREEN)Port forwarding Prometheus to http://localhost:9090$(NC)"
	@echo "$(YELLOW)Press Ctrl+C to stop$(NC)"
	kubectl port-forward svc/$(PROMETHEUS_RELEASE)-kube-prometheus-prometheus 9090:9090 -n $(MONITORING_NAMESPACE)

port-forward-loki: ## Port forward Loki (http://localhost:3100)
	@echo "$(GREEN)Port forwarding Loki to http://localhost:3100$(NC)"
	@echo "$(YELLOW)Press Ctrl+C to stop$(NC)"
	kubectl port-forward svc/$(LOKI_RELEASE) 3100:3100 -n $(MONITORING_NAMESPACE)

port-forward-app: ## Port forward application (http://localhost:8080)
	@echo "$(GREEN)Port forwarding app to http://localhost:8080$(NC)"
	@echo "$(YELLOW)Press Ctrl+C to stop$(NC)"
	kubectl port-forward svc/$(HELM_RELEASE) 8080:80 -n $(NAMESPACE)

port-forward-all: ## Start all port forwards in background
	@echo "$(GREEN)Starting all port forwards in background...$(NC)"
	@mkdir -p .pids
	@kubectl port-forward svc/$(PROMETHEUS_RELEASE)-grafana 3000:80 -n $(MONITORING_NAMESPACE) > /dev/null 2>&1 & echo $$! > .pids/grafana.pid
	@kubectl port-forward svc/$(PROMETHEUS_RELEASE)-kube-prometheus-prometheus 9090:9090 -n $(MONITORING_NAMESPACE) > /dev/null 2>&1 & echo $$! > .pids/prometheus.pid
	@kubectl port-forward svc/$(LOKI_RELEASE) 3100:3100 -n $(MONITORING_NAMESPACE) > /dev/null 2>&1 & echo $$! > .pids/loki.pid
	@kubectl port-forward svc/$(HELM_RELEASE) 8080:80 -n $(NAMESPACE) > /dev/null 2>&1 & echo $$! > .pids/app.pid
	@sleep 2
	@echo "$(GREEN)All port forwards started!$(NC)"
	@echo "  - Grafana:    http://localhost:3000 (admin/prom-operator)"
	@echo "  - Prometheus: http://localhost:9090"
	@echo "  - Loki:       http://localhost:3100"
	@echo "  - App:        http://localhost:8080"
	@echo ""
	@echo "$(YELLOW)Run 'make stop-port-forwards' to stop all$(NC)"

stop-port-forwards: ## Stop all background port forwards
	@echo "$(YELLOW)Stopping all port forwards...$(NC)"
	@if [ -f .pids/grafana.pid ]; then kill $$(cat .pids/grafana.pid) 2>/dev/null || true; fi
	@if [ -f .pids/prometheus.pid ]; then kill $$(cat .pids/prometheus.pid) 2>/dev/null || true; fi
	@if [ -f .pids/loki.pid ]; then kill $$(cat .pids/loki.pid) 2>/dev/null || true; fi
	@if [ -f .pids/app.pid ]; then kill $$(cat .pids/app.pid) 2>/dev/null || true; fi
	@rm -rf .pids
	@echo "$(GREEN)All port forwards stopped!$(NC)"

logs-app: ## View application logs from Kubernetes
	kubectl logs -l app.kubernetes.io/name=$(APP_NAME) -n $(NAMESPACE) -f

logs-prometheus: ## View Prometheus logs
	kubectl logs -l app.kubernetes.io/name=prometheus -n $(MONITORING_NAMESPACE) -f --tail=100

logs-loki: ## View Loki logs
	kubectl logs -l app.kubernetes.io/name=loki -n $(MONITORING_NAMESPACE) -f --tail=100

logs-promtail: ## View Promtail logs
	kubectl logs -l app.kubernetes.io/name=promtail -n $(MONITORING_NAMESPACE) -f --tail=100

shell-app: ## Open shell in application pod
	kubectl exec -it $$(kubectl get pod -l app.kubernetes.io/name=$(APP_NAME) -n $(NAMESPACE) -o jsonpath='{.items[0].metadata.name}') -n $(NAMESPACE) -- /bin/sh

describe-app: ## Describe application pod
	kubectl describe pod -l app.kubernetes.io/name=$(APP_NAME) -n $(NAMESPACE)

