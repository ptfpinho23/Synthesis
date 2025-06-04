# Synthesis Container Orchestrator Makefile

BINARY_DIR := bin
GO_CMD := go
GO_BUILD := $(GO_CMD) build
GO_CLEAN := $(GO_CMD) clean
GO_TEST := $(GO_CMD) test
GO_MOD := $(GO_CMD) mod

SERVER_BINARY := synthesis-server
CLI_BINARY := synthesis-cli

SERVER_DIR := cmd/synthesis-server
CLI_DIR := cmd/synthesis-cli

.PHONY: all build build-server build-cli clean test deps run-server help

all: build

build: build-server build-cli

build-server:
	@echo "Building synthesis-server..."
	@mkdir -p $(BINARY_DIR)
	$(GO_BUILD) -o $(BINARY_DIR)/$(SERVER_BINARY) ./$(SERVER_DIR)

build-cli:
	@echo "Building synthesis-cli..."
	@mkdir -p $(BINARY_DIR)
	$(GO_BUILD) -o $(BINARY_DIR)/$(CLI_BINARY) ./$(CLI_DIR)

clean:
	@echo "Cleaning..."
	$(GO_CLEAN)
	rm -rf $(BINARY_DIR)

test:
	@echo "Running tests..."
	$(GO_TEST) -v ./...

deps:
	@echo "Downloading dependencies..."
	$(GO_MOD) tidy
	$(GO_MOD) download

run-server: build-server
	@echo "Starting synthesis-server (containerd runtime)..."
	./$(BINARY_DIR)/$(SERVER_BINARY) start --debug

run-server-bg: build-server
	@echo "Starting synthesis-server in background..."
	./$(BINARY_DIR)/$(SERVER_BINARY) start --debug &

start: run-server

deploy-example: build-cli
	@echo "Deploying example Kubernetes manifests..."
	@echo "📦 Creating NGINX Deployment..."
	./$(BINARY_DIR)/$(CLI_BINARY) apply -f examples/nginx-workload.yaml
	@echo "🌐 Creating NGINX Service..."
	./$(BINARY_DIR)/$(CLI_BINARY) apply -f examples/nginx-service.yaml
	@echo "✅ Kubernetes manifests applied successfully!"

stop-example: build-cli
	@echo "Stopping example Kubernetes resources..."
	./$(BINARY_DIR)/$(CLI_BINARY) delete -f examples/nginx-workload.yaml
	./$(BINARY_DIR)/$(CLI_BINARY) delete -f examples/nginx-service.yaml

status: build-cli
	@echo "=== Kubernetes-compatible Resource Status ==="
	@echo "📋 Deployments:"
	@./$(BINARY_DIR)/$(CLI_BINARY) get deployments 2>/dev/null || echo "No deployments found"
	@echo
	@echo "🚀 Pods:"
	@./$(BINARY_DIR)/$(CLI_BINARY) get pods 2>/dev/null || echo "No pods found"
	@echo
	@echo "🌐 Services:"
	@./$(BINARY_DIR)/$(CLI_BINARY) get services 2>/dev/null || echo "No services found"
	@echo
	@echo "🖥️  Nodes:"
	@./$(BINARY_DIR)/$(CLI_BINARY) get nodes 2>/dev/null || echo "No nodes found"

health: build-cli
	@echo "Checking Synthesis health..."
	./$(BINARY_DIR)/$(CLI_BINARY) get --server http://localhost:8080 /healthz

build-linux:
	@echo "Building Linux binaries..."
	@mkdir -p $(BINARY_DIR)/linux
	GOOS=linux GOARCH=amd64 $(GO_BUILD) -o $(BINARY_DIR)/linux/$(SERVER_BINARY) ./$(SERVER_DIR)
	GOOS=linux GOARCH=amd64 $(GO_BUILD) -o $(BINARY_DIR)/linux/$(CLI_BINARY) ./$(CLI_DIR)

install: build
	@echo "Installing binaries..."
	cp $(BINARY_DIR)/$(SERVER_BINARY) $(GOPATH)/bin/
	cp $(BINARY_DIR)/$(CLI_BINARY) $(GOPATH)/bin/

dev: clean deps build run-server-bg
	@echo "🚀 Synthesis development environment started!"
	@echo "📋 Server running with containerd runtime"
	@echo "✅ Ready to accept Kubernetes manifests"
	@echo ""
	@echo "Try these commands:"
	@echo "  make deploy-example  # Deploy Kubernetes manifests"
	@echo "  make status         # Check resource status"
	@echo "  make health         # Check system health"

fmt:
	@echo "Formatting code..."
	$(GO_CMD) fmt ./...

vet:
	@echo "Vetting code..."
	$(GO_CMD) vet ./...

check: fmt vet test

docker-build:
	@echo "Building Docker images..."
	docker build -t synthesis-server -f docker/Dockerfile.server .
	docker build -t synthesis-cli -f docker/Dockerfile.cli .

k8s-export:
	@echo "Exporting Kubernetes resources (if kubectl available)..."
	@which kubectl >/dev/null 2>&1 && kubectl get deployment,service,pod -o yaml > k8s-export.yaml || echo "kubectl not found"

help:
	@echo "Synthesis Container Orchestrator - Like Kubernetes, but tinier"
	@echo "Fully compatible with Kubernetes Pod, Deployment, StatefulSet manifests"
	@echo ""
	@echo "Available commands:"
	@echo ""
	@echo "  📦 Building:"
	@echo "    make build         - Build all binaries"
	@echo "    make build-server  - Build synthesis-server"
	@echo "    make build-cli     - Build synthesis-cli"
	@echo ""
	@echo "  🚀 Running:"
	@echo "    make start         - Quick start (build and run)"
	@echo "    make run-server    - Run server (foreground)"
	@echo "    make dev           - Full development environment"
	@echo ""
	@echo "  📋 Kubernetes Manifests:"
	@echo "    make deploy-example - Deploy example K8s manifests"
	@echo "    make stop-example  - Stop example resources"
	@echo "    make status        - Show all resources (kubectl-style)"
	@echo ""
	@echo "  🔧 Development:"
	@echo "    make test          - Run tests"
	@echo "    make clean         - Remove build artifacts"
	@echo "    make deps          - Download dependencies"
	@echo "    make fmt           - Format code"
	@echo "    make vet           - Vet code"
	@echo "    make check         - Format, vet and test"
	@echo ""
	@echo "  📤 Utilities:"
	@echo "    make install       - Install binaries to GOPATH"
	@echo "    make health        - Check system health"
	@echo "    make k8s-export    - Export existing K8s resources"
	@echo "    make help          - Show this help" 