# OpsMini 多架构发布构建（Linux 服务器 + Windows 主机 + macOS 开发机）。
# 依赖：Go 1.25+（纯 Go 驱动无 CGO，可直接交叉编译，无需额外工具链）。

VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
GIT_SHA    := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
# 工作区是否有未提交改动。有改动却仍打版本号，会让二进制与 tag 语义脱节
# （v1.4.0、v1.2.1 各踩过一次）。这里让 -version 直接把 -dirty 后缀打出来，
# 谁都能一眼看出"这个包不是干净的 tag 构建"。
GIT_DIRTY  := $(shell git status --porcelain 2>/dev/null | head -n 1)
GIT_COMMIT := $(GIT_SHA)$(if $(GIT_DIRTY),-dirty,)
# 版本资源的数值部分（VERSIONINFO 只认 a.b.c.d）。
# VERSION 可能是 v1.5.0 / v1.5.0-3-gabc123-dirty / dev，这里只取前面的数字段。
VER_NUM    := $(shell echo '$(VERSION)' | sed -E 's/^v//' | grep -oE '^[0-9]+(\.[0-9]+){0,2}' || echo 0.0.0)
VER_MAJOR  := $(or $(word 1,$(subst ., ,$(VER_NUM))),0)
VER_MINOR  := $(or $(word 2,$(subst ., ,$(VER_NUM))),0)
VER_PATCH  := $(or $(word 3,$(subst ., ,$(VER_NUM))),0)

LDFLAGS    := -s -w \
	-X main.version=$(VERSION) \
	-X main.buildTime=$(BUILD_TIME) \
	-X main.gitCommit=$(GIT_COMMIT)

# 目标平台：os/arch（Linux 服务器 + macOS 开发机 + Windows 主机）
TARGETS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

DIST_DIR := dist
BIN      := opsmini

.PHONY: all build build-all winres clean version push push-github release-check release

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
	@if [ "$(GOOS)" = windows ]; then $(MAKE) --no-print-directory winres; fi
	$(eval GOARCH := $(word 2,$(subst /, ,$@)))
	$(eval EXT := $(if $(filter windows,$(GOOS)),.exe,))
	$(eval OUT := $(DIST_DIR)/$(BIN)-$(VERSION)-$(GOOS)-$(GOARCH)$(EXT))
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -trimpath -ldflags "$(LDFLAGS)" -o $(OUT) ./cmd/agent
	@echo "built: $(OUT)"

## winres: 生成 Windows 版本资源（右键属性里能看到版本/公司/说明）
## 产物 cmd/agent/resource_windows_{386,amd64,arm,arm64}.syso 由构建过程生成，不进仓库。
## 版本号取自 VERSION，跟 -ldflags 注入的保持一致；注释里带 commit，便于对包。
winres:
	@cd cmd/agent && \
	  if ! go run github.com/josephspurrier/goversioninfo/cmd/goversioninfo@v1.7.0 \
	    -platform-specific -64 \
	    -ver-major $(VER_MAJOR) -ver-minor $(VER_MINOR) -ver-patch $(VER_PATCH) -ver-build 0 \
	    -product-ver-major $(VER_MAJOR) -product-ver-minor $(VER_MINOR) -product-ver-patch $(VER_PATCH) -product-ver-build 0 \
	    -file-version "$(VER_NUM).0" -product-version "$(VER_NUM).0" \
	    -comment "$(VERSION) $(GIT_COMMIT)" \
	    versioninfo.json >/dev/null; then \
	      echo "WARN: winres 跳过（goversioninfo 不可用，多为离线环境）；本次 exe 不带版本资源"; \
	  else \
	      echo "winres: cmd/agent/resource_windows_*.syso ($(VER_NUM).0)"; \
	  fi

## clean: 清理构建产物
clean:
	rm -rf $(DIST_DIR)

## version: 打印当前版本信息
version:
	@echo "version=$(VERSION) commit=$(GIT_COMMIT) time=$(BUILD_TIME)"
	@if [ -n "$(GIT_DIRTY)" ]; then echo "⚠ 工作区有未提交改动（构建产物会带 -dirty）"; fi

## push: 只推 codeup（origin）——内部主仓库，可随时推
## ⚠️ GitHub（github）是**直接对外发布**的镜像，规则（2026-10-28 用户明确）：
##   未经用户明确同意，**不得**推送 GitHub。所以默认 push 不碰它。
##    需要更新对外版本时，先征得同意，再显式执行：make push-github
## 若 HEAD 上有 tag，会随 push 一起推到 origin；tag 冲突时 git 会拒绝，需人工确认。
push:
	@set -e; \
	git remote | grep -qx origin || { echo "✗ 缺少远端 origin"; exit 1; }; \
	echo "→ origin (codeup)"; git push origin main; \
	tags=$$(git tag --points-at HEAD | tr '\n' ' '); \
	if [ -n "$$(echo $$tags | tr -d ' ')" ]; then \
		echo "→ tags: $$tags"; git push origin --tags; \
	else \
		echo "→ 当前提交没有 tag，跳过 tag 推送"; \
	fi; \
	echo "✓ origin 已同步到 $$(git rev-parse --short HEAD)"; \
	echo "  GitHub 未推送（对外版本，需用户同意后执行 make push-github）"

## push-github: 更新 GitHub 公开镜像——**必须先获得用户同意**
## 这是对外发布动作，会立刻被所有人看到，所以做成独立目标、不混在 push 里。
push-github:
	@set -e; \
	git remote | grep -qx github || { echo "✗ 缺少远端 github（git remote add github git@github.com:unixhot/opsmini.git）"; exit 1; }; \
	echo "⚠️  即将推送到 GitHub 公开仓库 git@github.com:unixhot/opsmini.git"; \
	echo "→ github main"; git push github main; \
	tags=$$(git tag --points-at HEAD | tr '\n' ' '); \
	if [ -n "$$(echo $$tags | tr -d ' ')" ]; then \
		echo "→ tags: $$tags"; git push github --tags; \
	else \
		echo "→ 当前提交没有 tag，跳过 tag 推送"; \
	fi; \
	echo "✓ GitHub 已同步到 $$(git rev-parse --short HEAD)"

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
