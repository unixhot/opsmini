# OpsMini 多架构发布构建（面向 Linux 主机面板）。
# 依赖：Go 1.25+（纯 Go 驱动无 CGO，可直接交叉编译，无需额外工具链）。

VERSION    ?= $(shell git describe --tags --always 2>/dev/null || echo dev)
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS    := -s -w \
	-X main.version=$(VERSION) \
	-X main.buildTime=$(BUILD_TIME) \
	-X main.gitCommit=$(GIT_COMMIT)

# 目标平台：os/arch（Linux 服务器 + macOS 开发机，无 Windows）
TARGETS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64

DIST_DIR := dist
BIN      := opsmini

.PHONY: all build build-all clean version

## build: 编译当前平台的二进制到 dist/
build:
	@mkdir -p $(DIST_DIR)
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BIN) ./cmd/agent
	@echo "built: $(DIST_DIR)/$(BIN) ($(VERSION))"

## build-all: 交叉编译全部目标平台
build-all: $(TARGETS)

$(TARGETS):
	@mkdir -p $(DIST_DIR)
	$(eval GOOS := $(word 1,$(subst /, ,$@)))
	$(eval GOARCH := $(word 2,$(subst /, ,$@)))
	$(eval OUT := $(DIST_DIR)/$(BIN)-$(VERSION)-$(GOOS)-$(GOARCH))
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -trimpath -ldflags "$(LDFLAGS)" -o $(OUT) ./cmd/agent
	@echo "built: $(OUT)"

## clean: 清理构建产物
clean:
	rm -rf $(DIST_DIR)

## version: 打印当前版本信息
version:
	@echo "version=$(VERSION) commit=$(GIT_COMMIT) time=$(BUILD_TIME)"
