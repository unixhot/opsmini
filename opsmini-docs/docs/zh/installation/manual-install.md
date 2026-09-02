# 手动安装（从源码构建）

不想用一键脚本时，可从源码构建并手动部署。

## 前置依赖

| 依赖 | 版本要求 | 说明 |
|------|----------|------|
| Go | **1.25+** | 纯 Go 驱动无 CGO，任意平台可直接交叉编译 |
| 内存 | ≥ 512MB | 构建与运行均轻量 |

> **为什么无需 CGO 工具链**：SQLite 驱动采用 `github.com/glebarez/sqlite`（纯 Go 实现），
> `CGO_ENABLED=0` 即可在任意平台上交叉编译出**静态链接**的二进制。

## 获取代码与依赖

```bash
git clone <仓库地址> opsmini
cd opsmini

# 国内网络需配置 goproxy 镜像
export GOPROXY=https://goproxy.cn,direct

go mod tidy
```

## 构建

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

> 正式发版流程：`git tag v1.0.0 && make build-all`，`VERSION` 自动取 tag 名。

## 校验产物架构

```bash
file dist/*
# 预期：ELF x86-64 / ELF aarch64 / Mach-O x86_64 / Mach-O arm64，均为静态链接
```

## 手动部署

### 目录规划

```
/data/opsmini/
├── opsmini              # 二进制
├── config.yaml          # 配置文件
└── opsmini.db           # 数据（首次启动自动生成）
```

### 放置二进制与配置

```bash
sudo mkdir -p /data/opsmini
sudo cp dist/opsmini-*-linux-amd64 /data/opsmini/opsmini
sudo cp configs/config.yaml /data/opsmini/config.yaml

# 修改配置：数据库绝对路径（JWT 签名密钥无需配置，首次启动自动生成）
sudo sed -i 's|path: "opsmini.db"|path: "/data/opsmini/opsmini.db"|' /data/opsmini/config.yaml

sudo chmod +x /data/opsmini/opsmini
```

### 运行

```bash
/data/opsmini/opsmini -config /data/opsmini/config.yaml
```

首次启动自动建表并写入默认管理员账号，日志打印账号密码。访问 `http://<host>:8888` 即见面板。

## 下一步

- [systemd 托管](systemd.md)
- [配置详解](../configuration/config-file.md)
