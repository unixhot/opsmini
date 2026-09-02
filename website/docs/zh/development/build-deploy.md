# 构建与部署

## 前置依赖

- Go 1.25+（纯 Go 驱动无 CGO，可直接交叉编译）

## 获取代码

```bash
git clone <仓库地址> opsmini
cd opsmini
export GOPROXY=https://goproxy.cn,direct   # 国内加速
go mod tidy
```

## 构建命令

```bash
make build          # 编译当前平台 → dist/opsmini
make build-all      # 交叉编译 linux/darwin amd64/arm64
make clean          # 清理 dist/
make version        # 打印版本信息
```

## 版本号注入

构建信息通过 `-ldflags -X main.version/-X main.buildTime/-X main.gitCommit` 注入：

```bash
make build VERSION=v1.0.0
./dist/opsmini -version
# opsmini v1.0.0 (commit abc1234, built 2026-08-13T07:00:00Z)
```

## 产物命名

`opsmini-<version>-<os>-<arch>`，如 `opsmini-v1.0.0-linux-amd64`。

## 交付产物

| 产物 | 说明 |
|------|------|
| `opsmini` 单二进制 | 后端 API + 前端 UI + SQLite，约 28~30MB 静态链接 |
| `configs/config.yaml` | 配置模板 |
| `opsmini.db` | 首次运行自动生成 |

> 部署即拷贝二进制 + 配置文件，无其他运行时依赖。
