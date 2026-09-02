# 構建與部署

## 前置依賴

- Go 1.25+（純 Go 驅動無 CGO，可直接交叉編譯）

## 獲取程式碼

```bash
git clone <仓库地址> opsmini
cd opsmini
export GOPROXY=https://goproxy.cn,direct   # 国内加速
go mod tidy
```

## 構建命令

```bash
make build          # 编译当前平台 → dist/opsmini
make build-all      # 交叉编译 linux/darwin amd64/arm64
make clean          # 清理 dist/
make version        # 打印版本信息
```

## 版本號注入

構建資訊透過 `-ldflags -X main.version/-X main.buildTime/-X main.gitCommit` 注入：

```bash
make build VERSION=v1.0.0
./dist/opsmini -version
# opsmini v1.0.0 (commit abc1234, built 2026-08-13T07:00:00Z)
```

## 產物命名

`opsmini-<version>-<os>-<arch>`，如 `opsmini-v1.0.0-linux-amd64`。

## 交付產物

| 產物 | 說明 |
|------|------|
| `opsmini` 單二進位制 | 後端 API + 前端 UI + SQLite，約 28~30MB 靜態連結 |
| `configs/config.yaml` | 配置模板 |
| `opsmini.db` | 首次執行自動生成 |

> 部署即複製二進位制 + 配置檔案，無其他執行時依賴。
