# Build variables
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
BIN_DIR ?= bin
BIN_NAME ?= $(shell basename $(CURDIR))

.DEFAULT_GOAL := help

.PHONY: help
help: ## help: 显示帮助 (Show available commands)
	@echo "Usage: make [target]"
	@echo ""
	@echo "开发 (Development):"
	@grep -E '^[a-zA-Z_-]+:.*?## dev:' $(firstword $(MAKEFILE_LIST)) | sort | \
		awk -F':.*?## dev: ' '{printf "  make %-15s %s\n", $$1, $$2}'
	@echo ""
	@echo "构建 (Build):"
	@grep -E '^[a-zA-Z_-]+:.*?## build:' $(firstword $(MAKEFILE_LIST)) | sort | \
		awk -F':.*?## build: ' '{printf "  make %-15s %s\n", $$1, $$2}'
	@echo ""
	@echo "测试 (Test):"
	@grep -E '^[a-zA-Z_-]+:.*?## test:' $(firstword $(MAKEFILE_LIST)) | sort | \
		awk -F':.*?## test: ' '{printf "  make %-15s %s\n", $$1, $$2}'
	@echo ""
	@echo "部署 (Deploy):"
	@grep -E '^[a-zA-Z_-]+:.*?## deploy:' $(firstword $(MAKEFILE_LIST)) | sort | \
		awk -F':.*?## deploy: ' '{printf "  make %-15s %s\n", $$1, $$2}'

.PHONY: rundev
rundev: ## dev: 启动本地开发服务 (Start local development server)
	@echo "Starting local dev server..."
	@go run main.go

.PHONY: fmt
fmt: ## dev: 格式化代码 (Format Go source code)
	@if command -v gofumpt >/dev/null 2>&1; then \
		echo "Formatting with gofumpt..."; \
		gofumpt -l -w .; \
	else \
		gofmt -l -w .; \
		echo "gofumpt not found, using go fmt. Install: go install mvdan.cc/gofumpt@latest"; \
	fi

.PHONY: lint
lint: ## dev: 静态分析 (Run static analysis via go vet)
	@go vet ./...

.PHONY: build
build: ## build: 编译当前平台 (Compile for current platform)
	@mkdir -p $(BIN_DIR)
	@echo "Building for $(GOOS)/$(GOARCH)..."
	@go build -o $(BIN_DIR)/$(BIN_NAME) main.go
	@echo "Build complete: $(BIN_DIR)/$(BIN_NAME)"

.PHONY: build-linux
build-linux: ## build: 交叉编译 Linux amd64 (Cross-compile for Linux amd64)
	@mkdir -p $(BIN_DIR)
	@echo "Building for linux/amd64..."
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o $(BIN_DIR)/$(BIN_NAME) main.go
	@echo "Build complete: $(BIN_DIR)/$(BIN_NAME)"

.PHONY: test
test: ## test: 运行单元测试 (Run all unit tests)
	@echo "Running tests..."
	@go test -v ./... -count=1 && echo "All tests passed."

.PHONY: clean
clean: ## build: 清理编译产物 (Remove build artifacts)
	@echo "Cleaning build artifacts..."
	@rm -rf $(BIN_DIR)
	@echo "Done."

# Deploy variables (iqa demo server: 111.229.4.203, ssh alias from ~/.ssh/config)
DEPLOY_PATH ?= /opt/project/sg-scout/backend
DEPLOY_SERVICE ?= sg-scout-backend

.PHONY: deploy
deploy: build-linux ## deploy: 部署到 iqa 演示机 (scp + systemd restart + health check)
	@echo "Uploading to iqa ($(DEPLOY_SERVICE))..."
	@scp -q $(BIN_DIR)/$(BIN_NAME) deploy/sg-scout-backend.service config.toml iqa:/tmp/
	@ssh iqa 'install -m 755 /tmp/$(BIN_NAME) $(DEPLOY_PATH)/$(BIN_NAME) && install -m 644 /tmp/sg-scout-backend.service /etc/systemd/system/$(DEPLOY_SERVICE).service && install -m 644 /tmp/config.toml $(DEPLOY_PATH)/config.toml && systemctl daemon-reload && systemctl restart $(DEPLOY_SERVICE) && sleep 2 && curl -sf http://127.0.0.1:1324/common/health && echo "Deploy complete: DEPLOY_OK"'
