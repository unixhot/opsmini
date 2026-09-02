# 手動安裝（從原始碼構建）

不想用一鍵指令碼時，可從原始碼構建並手動部署。

## 前置依賴

| 依賴 | 版本要求 | 說明 |
|------|----------|------|
| Go | **1.25+** | 純 Go 驅動無 CGO，任意平臺可直接交叉編譯 |
| 記憶體 | ≥ 512MB | 構建與執行均輕量 |

> **為什麼無需 CGO 工具鏈**：SQLite 驅動採用 `github.com/glebarez/sqlite`（純 Go 實現），
> `CGO_ENABLED=0` 即可在任意平臺上交叉編譯出**靜態連結**的二進位制。

## 獲取程式碼與依賴

```bash
git clone <仓库地址> opsmini
cd opsmini

# 国内网络需配置 goproxy 镜像
export GOPROXY=https://goproxy.cn,direct

go mod tidy
```

## 構建

```bash
# 编译当前平台 → dist/opsmini
make build

# 交叉编译全部目标平台
make build-all
# 产物：
#   dist/opsmini-<version>-linux-amd64
#   dist/opsmini-<version>-linux-arm64
#   dist/opsmini-<version>-darwin-amd64
#   dist/opsmini-<version>-darwin-arm64

# 显式指定版本号
make build VERSION=v1.0.0

# 查看二进制版本
./dist/opsmini -version
```

> 正式發版流程：`git tag v1.0.0 && make build-all`，`VERSION` 自動取 tag 名。

## 校驗產物架構

```bash
file dist/*
# 预期：ELF x86-64 / ELF aarch64 / Mach-O x86_64 / Mach-O arm64，均为静态链接
```

## 手動部署

### 目錄規劃

```
/data/opsmini/
├── opsmini              # 二进制
├── config.yaml          # 配置文件
└── opsmini.db           # 数据（首次启动自动生成）
```

### 放置二進位制與配置

```bash
sudo mkdir -p /data/opsmini
sudo cp dist/opsmini-*-linux-amd64 /data/opsmini/opsmini
sudo cp configs/config.yaml /data/opsmini/config.yaml

# 修改配置：数据库绝对路径（JWT 签名密钥无需配置，首次启动自动生成）
sudo sed -i 's|path: "opsmini.db"|path: "/data/opsmini/opsmini.db"|' /data/opsmini/config.yaml

sudo chmod +x /data/opsmini/opsmini
```

### 執行

```bash
/data/opsmini/opsmini -config /data/opsmini/config.yaml
```

首次啟動自動建表並寫入預設管理員賬號，日誌列印賬號密碼。訪問 `http://<host>:8888` 即見面板。

## 下一步

- [systemd 託管systemd.md
- [配置詳解../configuration/config-file.md
