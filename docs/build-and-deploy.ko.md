<div align="center">

# OpsMini 빌드 및 배포

**빌드 및 배포 가이드**

[English](build-and-deploy.md) · [简体中文](build-and-deploy.zh-CN.md) · [繁體中文](build-and-deploy.zh-TW.md) · [日本語](build-and-deploy.ja.md) · [한국어](build-and-deploy.ko.md) · [ไทย](build-and-deploy.th.md) · [Deutsch](build-and-deploy.de.md)

</div>

---

> 버전: v1.0  
> 대상: 소스 코드 빌드부터 프로덕션 배포까지의 전체 흐름

---

## 1. 사전 요구 사항

| 의존성 | 버전 요구 사항 | 설명 |
|------|----------|------|
| Go | **1.25+** | 순수 Go 드라이버로 CGO 불필요. 어떤 플랫폼에서든 직접 크로스 컴파일 가능 |
| 메모리 | ≥ 512MB | 빌드와 실행 모두 가벼움 |
| 대상 호스트 | Linux / macOS | 서버 환경은 Linux x64 / arm64 대상. macOS는 개발·디버깅 전용 |

> **CGO 툴체인이 필요 없는 이유**: SQLite 드라이버는 `github.com/glebarez/sqlite`(순수 Go 구현)를 사용하므로,
> `CGO_ENABLED=0`만으로 어떤 플랫폼에서든 **정적 링크** 바이너리를 크로스 컴파일할 수 있으며, 대상 아키텍처별로 C 크로스 컴파일러를 따로 설정할 필요가 없습니다.

---

## 2. 코드 및 의존성 가져오기

```bash
git clone <仓库地址> opsmini
cd opsmini

# 国内网络需配置 goproxy 镜像（proxy.golang.org 可能被墙）
export GOPROXY=https://goproxy.cn,direct

go mod tidy
```

---

## 3. 빌드

### 3.1 현재 플랫폼용 컴파일

```bash
make build
# 产物：dist/opsmini
```

### 3.2 모든 대상 플랫폼 크로스 컴파일

```bash
make build-all
# 产物：
#   dist/opsmini-<version>-linux-amd64
#   dist/opsmini-<version>-linux-arm64
#   dist/opsmini-<version>-darwin-amd64
#   dist/opsmini-<version>-darwin-arm64
```

| 플랫폼 | 사용 시나리오 |
|------|----------|
| `linux/amd64` | 주류 x86_64 서버(프로덕션) |
| `linux/arm64` | ARM 서버(Graviton / 라즈베리 파이 / Kunpeng / Phytium, 프로덕션) |
| `darwin/amd64` | Intel Mac 개발 머신(개발·디버깅) |
| `darwin/arm64` | Apple Silicon 개발 머신(개발·디버깅) |

> OpsMini는 **Linux 호스트**용 패널로 Linux 설치 패키지만 배포합니다. macOS 타깃은 로컬 개발·디버깅 전용이며 배포 산출물이 아닙니다.

### 3.3 버전 번호 주입

```bash
# 默认取 git tag / commit，也可显式指定
make build VERSION=v1.0.0

# 查看二进制版本
./dist/opsmini -version
# 输出：opsmini v1.0.0 (commit abc1234, built 2026-08-13T07:00:00Z)
```

> 정식 릴리스 절차: `git tag v1.0.0 && make build-all` 실행 시 `VERSION`은 자동으로 태그 이름을 사용합니다.

### 3.4 기타 Makefile 타깃

```bash
make clean     # 清理 dist/
make version   # 打印当前版本信息
```

### 3.5 산출물 아키텍처 검증

```bash
file dist/*
# 预期：ELF x86-64 / ELF aarch64 / Mach-O x86_64 / Mach-O arm64，均为静态链接
```

---

## 4. 설정

설정 파일 기본 경로는 `configs/config.yaml`(`-config`로 지정 가능)입니다. 전체 예시:

```yaml
server:
  host: "0.0.0.0"              # 监听地址
  port: 8888                    # 监听端口
  secret_entry: ""              # 安全入口前缀，如 /opsmini_panel（留空不启用）

database:
  path: "opsmini.db"            # SQLite 数据文件路径

jwt:
  secret: "change-me"           # JWT 签名密钥，生产环境务必修改为随机串
  access_ttl_seconds: 900       # access token 有效期（15 分钟）
  refresh_ttl_seconds: 604800   # refresh token 有效期（7 天）

ai:
  enabled: true                 # 是否启用 AI 助手
  provider: "openai"            # openai / deepseek / qwen / ollama
  model: "gpt-4o"               # 模型名
  base_url: "https://api.openai.com/v1"
  api_key: ""                   # 建议用环境变量 OPSMINI_AI_KEY 覆盖

agent:
  token: ""                     # 对外 REST API 认证令牌，空则禁用 /agent/v1
  allowed_commands:             # 命令白名单前缀（仅允许以此开头的命令）
    - ps
    - df
    - free
    - uptime
    - docker
    - systemctl
    - nginx
    - who
```

### 환경 변수

| 변수 | 설명 |
|------|------|
| `OPSMINI_AI_KEY` | AI API Key, `ai.api_key`보다 **우선**합니다(키를 디스크에 저장하지 않음) |

---

## 5. 실행

```bash
# 直接运行
./opsmini -config configs/config.yaml

# 打印版本并退出
./opsmini -version
```

- 최초 시작 시 테이블을 자동 생성하고 기본 관리자 계정을 기록합니다
- `http://<host>:8888`에 접속하면 패널이 표시됩니다(프런트엔드는 바이너리에 내장되어 있어 별도 배포 불필요)

### 기본 계정

| 항목 | 값 |
|----|----|
| 사용자 이름 | `opsmini`(고정) |
| 비밀번호 | **최초 시작 시 무작위 생성**, 시작 로그에서 확인 |

시작 후 로그에 다음과 같이 출력됩니다:

```
initial admin account created: username=opsmini password=ggFKEX65ZVF6RNTm
```

> ⚠️ 이 비밀번호를 즉시 기록하고 로그인 후 변경하세요. 비밀번호는 최초 설치 시 한 번만 생성되며, 이후에는 패널 내 '사용자 관리'에서 변경해야 합니다.

---

## 6. 프로덕션 배포(Linux + systemd)

### 6.1 디렉터리 구성

```
/opt/opsmini/
├── opsmini              # 二进制
└── config.yaml          # 配置文件
/var/lib/opsmini/
└── opsmini.db           # 数据（自动生成，随 data 目录权限而定）
```

### 6.2 설치 절차

```bash
# 1. 放置二进制与配置
sudo mkdir -p /opt/opsmini /var/lib/opsmini
sudo cp dist/opsmini-*-linux-amd64 /opt/opsmini/opsmini
sudo cp configs/config.yaml /opt/opsmini/config.yaml

# 2. 修改配置：JWT secret、数据库绝对路径
sudo sed -i 's|path: "opsmini.db"|path: "/var/lib/opsmini/opsmini.db"|' /opt/opsmini/config.yaml
sudo sed -i 's|secret: "change-me"|secret: "<随机长串>"|' /opt/opsmini/config.yaml

# 3. 赋权
sudo chmod +x /opt/opsmini/opsmini
```

### 6.3 systemd 서비스

`/etc/systemd/system/opsmini.service` 생성:

```ini
[Unit]
Description=OpsMini Server Panel
After=network.target

[Service]
Type=simple
ExecStart=/opt/opsmini/opsmini -config /opt/opsmini/config.yaml
Restart=always
RestartSec=3
# 安全加固（可选）
User=opsmini
Group=opsmini
NoNewPrivileges=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

활성화 및 시작:

```bash
sudo useradd -r -s /usr/sbin/nologin opsmini || true
sudo chown -R opsmini:opsmini /var/lib/opsmini /opt/opsmini
sudo systemctl daemon-reload
sudo systemctl enable --now opsmini
sudo systemctl status opsmini
```

---

## 7. 리버스 프록시(선택)

### 7.1 Nginx

```nginx
server {
    listen 80;
    server_name panel.example.com;

    location / {
        proxy_pass http://127.0.0.1:8888;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # Web 终端（WebSocket）需要升级支持
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

### 7.2 Caddy(자동 HTTPS)

```caddyfile
panel.example.com {
    reverse_proxy 127.0.0.1:8888
}
```

### 7.3 시크릿 엔트리

`secret_entry: /opsmini_panel`을 설정하면 패널 경로가 `http://<host>:8888/opsmini_panel`로 바뀌며, 리버스 프록시와 함께 실제 엔트리를 숨길 수 있습니다.

---

## 8. 업그레이드

```bash
# 1. 备份数据
sudo systemctl stop opsmini
cp /var/lib/opsmini/opsmini.db /var/lib/opsmini/opsmini.db.bak.$(date +%s)

# 2. 替换二进制
sudo cp dist/opsmini-<new>-linux-amd64 /opt/opsmini/opsmini

# 3. 重启
sudo systemctl start opsmini
```

> SQLite는 GORM이 자동 마이그레이션하므로 버전 간 업그레이드 시 대개 수동으로 테이블을 변경할 필요가 없습니다.

---

## 9. 자주 묻는 질문

| 문제 | 원인 | 해결 |
|------|------|------|
| `go mod tidy`가 멈춤/Bad Gateway | `proxy.golang.org` 차단됨 | `export GOPROXY=https://goproxy.cn,direct` |
| Go 버전이 너무 낮다는 오류 | 의존성이 Go 1.25+ 요구 | 공식 사전 컴파일 바이너리 다운로드(아래 참조) |
| 빌드 시 `vendor` 디렉터리 충돌 | 프로젝트 내 `vendor/`와 Go 모듈 vendor 규약 충돌 | 프런트엔드 의존성 디렉터리는 `static/`으로 개명됨, `vendor/`를 다시 만들지 말 것 |
| `/` 접근 시 301 `./` 반환 | embed.FS에 대한 `c.FileFromFS` 디렉터리 리다이렉트 | ReadFile + c.Data로 수정됨(되돌리지 말 것) |

### Go 1.25+ 설치(공식 바이너리, 가장 빠름)

```bash
# macOS（Apple Silicon）
curl -sL -o /tmp/go.tar.gz https://go.dev/dl/go1.25.0.darwin-arm64.tar.gz
mkdir -p ~/go-sdk && tar -C ~/go-sdk -xzf /tmp/go.tar.gz
export PATH=~/go-sdk/go/bin:$PATH

# Linux x64
curl -sL -o /tmp/go.tar.gz https://go.dev/dl/go1.25.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf /tmp/go.tar.gz
export PATH=/usr/local/go/bin:$PATH
```

---

## 10. 배포 산출물 목록

| 산출물 | 설명 |
|------|------|
| `opsmini` 단일 바이너리 | 백엔드 API + 프런트엔드 UI + SQLite, 약 28~30MB, 정적 링크 |
| `configs/config.yaml` | 설정 템플릿 |
| `opsmini.db` | 최초 실행 시 자동 생성되는 데이터 파일 |

> 배포는 바이너리 + 설정 파일 복사만으로 완료되며 다른 런타임 의존성이 없습니다.
