# OpsMini 多架构发布构建（面向 Linux 主机面板）。
# 依赖：Go 1.25+（纯 Go 驱动无 CGO，可直接交叉编译，无需额外工具链）。

VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
GIT_SHA    := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
# 工作区是否有未提交改动。有改动却仍打版本号，会让二进制与 tag 语义脱节
# （v1.4.0、v1.2.1 各踩过一次）。这里让 -version 直接把 -dirty 后缀打出来，
# 谁都能一眼看出"这个包不是干净的 tag 构建"。
GIT_DIRTY  := $(shell git status --porcelain 2>/dev/null | head -n 1)
GIT_COMMIT := $(GIT_SHA)$(if $(GIT_DIRTY),-dirty,)
LDFLAGS    := -s -w \
	-X main.version=$(VERSION) \
	-X main.buildTime=$(BUILD_TIME) \
	-X main.gitCommit=$(GIT_COMMIT)

# 目标平台：os/arch（Linux 服务器 + macOS 开发机，无 Windows）
TARGETS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64

DIST_DIR := dist
BIN      := opsmini

.PHONY: all build build-all clean version push release-check release

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
	@if [ -n "$(GIT_DIRTY)" ]; then echo "⚠ 工作区有未提交改动（构建产物会带 -dirty）"; fi

## push: 推送到两个远端——codeup（origin，主仓库）与 GitHub（github，公开镜像）
## 曾经漏推 GitHub 导致公开仓库停在 v1.1.0，这里合成一条命令。
## 若 HEAD 上有 tag，会自动一起推；tag 已存在且指向不同提交时 git 会拒绝，
## 需要人工确认后再 --force，避免误改已发布的 tag。
push:
	@set -e; \
	git remote | grep -qx origin || { echo "✗ 缺少远端 origin"; exit 1; }; \
	git remote | grep -qx github || { echo "✗ 缺少远端 github（git remote add github git@github.com:unixhot/opsmini.git）"; exit 1; }; \
	echo "→ origin (codeup)"; git push origin main; \
	echo "→ github"; git push github main; \
	tags=$$(git tag --points-at HEAD | tr '\n' ' '); \
	if [ -n "$$(echo $$tags | tr -d ' ')" ]; then \
		echo "→ tags: $$tags"; git push origin --tags; git push github --tags; \
	else \
		echo "→ 当前提交没有 tag，跳过 tag 推送"; \
	fi; \
	echo "✓ 两个远端均已同步到 $$(git rev-parse --short HEAD)"

## release-check: 发布前预检——工作区必须干净，且 HEAD 必须正好落在 tag 上
## 正确顺序：git commit → git tag vX.Y.Z → make push → make release
## （这样 VERSION 就是干净的 tag 名，产物名才符合 opsmini-v<版本>-linux-<架构>，
##   且 -version 的 commit == tag 指向的 commit；tag 也是随 push 一起发出去的）
release-check:
	@if [ -n "$$(git status --porcelain)" ]; then \
		echo "✗ 工作区有未提交改动，先 commit 再构建（否则 -version 的 commit 与 tag 不符）"; \
		git status --porcelain; exit 1; fi
	@if [ -z "$$(git describe --tags --exact-match 2>/dev/null)" ]; then \
		echo "✗ HEAD 未指向任何 tag，当前 VERSION=$(VERSION)"; \
		echo "  请先打 tag：git tag vX.Y.Z && make push && make release"; exit 1; fi
	@echo "✓ 工作区干净，tag=$$(git describe --tags --exact-match)，commit=$$(git rev-parse --short HEAD)"

## release: 预检通过后交叉编译全部平台
release: release-check build-all
	@echo "发布包："
	@ls -lh $(DIST_DIR)/$(BIN)-$(VERSION)-* 2>/dev/null || true
