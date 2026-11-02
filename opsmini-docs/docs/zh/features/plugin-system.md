# 插件系统

内核只做「所有主机都需要的事」：采集、审计、Agent 运行时、护栏。业务能力（某个中间件的指标、某类日志的解析、某个平台的动作对接）用**插件**扩展。

这样做的目的不是追求架构美观，而是避免两件具体的事：**二进制无限膨胀**，以及**为了一个新能力就得发一个版本**。

## 插件是独立进程，不是 Go plugin

- Go 的 `plugin` 包要求编译期完全一致的构建环境，跨版本必然崩，直接排除
- 插件是**独立可执行文件**，面板以子进程方式启动它，通过 Unix socket 用**行分隔 JSON** 通信
- 插件崩溃不影响面板：进程退出会被记录、按重启策略拉起、连续失败标记为 failed 并停止重试

## 包格式

```
foo-collector-1.2.0-linux-amd64.tar.gz
├── plugin.yaml          # 清单
├── bin/plugin           # 入口（可执行）
├── SIGNATURE            # ed25519 签名（对整包 sha256 签名）
└── README.md            # 可选
```

清单示例：

```yaml
apiVersion: opsmini.plugin/v1
id: foo-collector              # 全局唯一，^[a-z0-9][a-z0-9-]{2,40}$
name: Foo 采集器
version: 1.2.0                 # semver
vendor: 速云科技
description: 采集 Foo 服务的队列深度与消费速率
kind: collector                # collector | action | panel
entry: bin/plugin              # 相对包内路径
platform: [linux/amd64, linux/arm64]

capabilities:                  # 能力声明，同时作为审计依据
  - metrics
  - read:/etc/foo
  - exec:systemctl
  - net:outbound

runtime:
  memoryLimitMB: 128
  cpuQuotaPercent: 20
  restartPolicy: on-failure    # always | on-failure | never

requires:
  opsmini: ">=1.4.0"           # 主机 Agent 版本要求
```

安装时的校验顺序（任一失败即拒绝，不装「半截插件」）：

1. 包 sha256（服务端计算，可与上传者声明的交叉校验）
2. ed25519 签名
3. `apiVersion` 兼容性
4. `platform` 是否匹配当前主机
5. 清单字段合法性、版本要求

## 三条通道

插件与内核只通过 socket 说四种消息，全部是单行 JSON：

| 消息 | 方向 | 用途 |
|---|---|---|
| `hello` | 插件 → 面板 | 启动握手，声明自己是谁、版本、能力 |
| `metric` | 插件 → 面板 | 上报指标（进入监控与告警规则） |
| `event` | 插件 → 面板 | 上报事件（可转成信号进入事件中心） |
| `heartbeat` | 插件 → 面板 | 存活与自检状态 |

插件不直接碰数据库、不直接调用面板接口、不继承面板的会话。

## 生命周期

```
安装（上传包 → 校验 → 解包到 releases/<version>/ → current 软链指向它）
   ↓
启用 → 启动进程 → 等 hello 握手 → 进入 running
   ↓
运行 → 崩溃/退出 → 按 restartPolicy 处理，记重启次数与最后一次错误
   ↓
停用 → SIGTERM → 等待退出 → 状态 stopped
   ↓
升级 → 新版本解包 → current 软链切换 → 重启（旧版本目录保留，可回退）
   ↓
删除 → 停进程 → 移除程序文件（数据目录保留）
```

## 权限边界：说清楚哪些做到了

这一节必须诚实，否则「插件安全」会变成一句空话。

**做到了：**

- 插件是独立进程，崩溃不会拖垮面板
- 安装必须验签（除非显式开启 unsigned 模式），且校验版本、平台、清单
- 安装、启用、停用、删除全部记审计日志
- 插件通过 socket 通信，不能直接操作面板数据库

**当前版本没做到（不要假设有）：**

- **没有切换到独立系统用户**：插件以面板进程的用户运行，通常是 `root`
- **没有 cgroup 资源限制**：清单里的 `memoryLimitMB` / `cpuQuotaPercent` 目前是声明，未强制
- **`capabilities` 未强制**：它是声明与审计依据，不是内核级白名单
- **不能审计插件内部的系统调用**

所以结论是：**签名证明「来源可信」，不证明「行为安全」**。只安装你信任来源的插件；第三方插件请自行审计代码。

## 安装前提（当前版本需要注意）

安装时会依次检查：`sha256` → 签名 → 清单。默认配置下**没有内置公钥，也没有开启 unsigned 模式**，因此直接上传会被拒绝，报「未配置插件公钥且未开启 unsigned 模式」。

两种解决方式，任选一种（写进设置后**需要重启面板**才生效，因为公钥在启动时读取）：

```bash
# 方式一：配置 ed25519 公钥（推荐，正式环境）
curl -X POST http://127.0.0.1:8888/api/v1/settings \
  -H "Authorization: Bearer <token>" -H "Content-Type: application/json" \
  -d '{"plugin_public_key":"<hex 编码的 32 字节公钥>"}'

# 方式二：开发/内网环境临时允许未签名包
curl -X POST http://127.0.0.1:8888/api/v1/settings \
  -H "Authorization: Bearer <token>" -H "Content-Type: application/json" \
  -d '{"plugin_allow_unsigned":"true"}'
```

改完 `systemctl restart opsmini`，再回到「面板设置 → 插件」上传。

> 当前版本面板还没有提供这两个设置项的输入界面，只能通过上面的接口或直接改设置表。下一版会补。

## 面板操作

**面板设置 → 插件**：

| 操作 | 说明 |
|---|---|
| 上传安装 | 选择 `*.tar.gz` 包，最大 64MB |
| 启用 / 停用 | 立即生效，不需要重启面板 |
| 重启 | 单独重启某个插件（例如改了它的配置） |
| 删除 | 移除程序文件，数据目录保留 |
| 事件 | 查看该插件最近上报的指标与事件 |

列表里能看到每个插件的：名称、版本、类型、状态（running / stopped / failed / installed）、重启次数、最后一次错误。

## 现状说明

机制是完整的，**生态目前是空的**——v1.4.0 没有附带官方插件包。这意味着装完 OpsMini 后，插件页是空的，这是预期状态而不是故障。

规划中的官方插件方向（未发布）：安全基线扩展巡检、特定中间件指标采集、日志解析规则包、第三方平台动作对接。

## 与其他模块的关系

| 模块 | 关系 |
|---|---|
| [自主 Agent（事件中心）](autonomous-agent.md) | 插件上报的 `event` 可转成信号进入事件中心，参与聚合与处置 |
| [仪表盘与监控](dashboard-monitoring.md) | 插件上报的 `metric` 可被告警规则使用 |
| [日志管道](log-pipeline.md) | 日志解析类能力既可以做进管道的 processor，也可以做成插件；前者更轻，后者可独立升级 |
| [主机安全](security.md) | 官方安全插件是主机安全的扩展方向 |
