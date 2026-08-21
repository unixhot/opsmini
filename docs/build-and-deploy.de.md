<div align="center">

# OpsMini Build und Deployment

**Build- und Deployment-Anleitung**

[English](build-and-deploy.md) · [简体中文](build-and-deploy.zh-CN.md) · [繁體中文](build-and-deploy.zh-TW.md) · [日本語](build-and-deploy.ja.md) · [한국어](build-and-deploy.ko.md) · [ไทย](build-and-deploy.th.md) · [Deutsch](build-and-deploy.de.md)

</div>

---

> Version: v1.0  
> Gilt für: den vollständigen Ablauf vom Build aus dem Quellcode bis zur Produktionsbereitstellung

---

## 1. Voraussetzungen

| Abhängigkeit | Versionsanforderung | Beschreibung |
|------|----------|------|
| Go | **1.25+** | Reines Go ohne CGO; Cross-Kompilierung direkt auf jeder Plattform möglich |
| Arbeitsspeicher | ≥ 512MB | Leichtgewichtig bei Build und Laufzeit |
| Zielhost | Linux / macOS | Serverszenarien zielen auf Linux x64 / arm64; macOS nur für Entwicklung und Debugging |

> **Warum keine CGO-Toolchain nötig ist**: Der SQLite-Treiber nutzt `github.com/glebarez/sqlite` (eine reine Go-Implementierung),
> daher kann mit `CGO_ENABLED=0` auf jeder Plattform ein **statisch gelinktes** Binary cross-kompiliert werden, ohne für jede Zielarchitektur einen separaten C-Cross-Compiler einzurichten.

---

## 2. Code und Abhängigkeiten abrufen

```bash
git clone <仓库地址> opsmini
cd opsmini

# 国内网络需配置 goproxy 镜像（proxy.golang.org 可能被墙）
export GOPROXY=https://goproxy.cn,direct

go mod tidy
```

---

## 3. Build

### 3.1 Für die aktuelle Plattform kompilieren

```bash
make build
# 产物：dist/opsmini
```

### 3.2 Cross-Kompilierung für alle Zielplattformen

```bash
make build-all
# 产物：
#   dist/opsmini-<version>-linux-amd64
#   dist/opsmini-<version>-linux-arm64
#   dist/opsmini-<version>-darwin-amd64
#   dist/opsmini-<version>-darwin-arm64
```

| Plattform | Einsatzszenario |
|------|----------|
| `linux/amd64` | Gängige x86_64-Server (Produktion) |
| `linux/arm64` | ARM-Server (Graviton / Raspberry Pi / Kunpeng / Phytium, Produktion) |
| `darwin/amd64` | Intel-Mac-Entwicklungsrechner (Entwicklung & Debugging) |
| `darwin/arm64` | Apple-Silicon-Entwicklungsrechner (Entwicklung & Debugging) |

> OpsMini ist ein Panel für **Linux-Hosts**; es werden nur Linux-Installationspakete veröffentlicht. macOS-Ziele dienen nur der lokalen Entwicklung und dem Debugging, nicht der Auslieferung.

### 3.3 Versionsnummer injizieren

```bash
# 默认取 git tag / commit，也可显式指定
make build VERSION=v1.0.0

# 查看二进制版本
./dist/opsmini -version
# 输出：opsmini v1.0.0 (commit abc1234, built 2026-08-13T07:00:00Z)
```

> Offizieller Release-Ablauf: `git tag v1.0.0 && make build-all`; `VERSION` wird automatisch aus dem Tag-Namen übernommen.

### 3.4 Weitere Makefile-Ziele

```bash
make clean     # 清理 dist/
make version   # 打印当前版本信息
```

### 3.5 Architektur der Artefakte prüfen

```bash
file dist/*
# 预期：ELF x86-64 / ELF aarch64 / Mach-O x86_64 / Mach-O arm64，均为静态链接
```

---

## 4. Konfiguration

Standardpfad der Konfigurationsdatei ist `configs/config.yaml` (kann mit `-config` angegeben werden). Vollständiges Beispiel:

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

### Umgebungsvariablen

| Variable | Beschreibung |
|------|------|
| `OPSMINI_AI_KEY` | AI API Key, hat **Vorrang vor** `ai.api_key` (vermeidet das Ablegen des Schlüssels auf der Platte) |

---

## 5. Ausführen

```bash
# 直接运行
./opsmini -config configs/config.yaml

# 打印版本并退出
./opsmini -version
```

- Beim ersten Start werden Tabellen automatisch angelegt und ein Standard-Admin-Konto geschrieben
- Der Aufruf von `http://<host>:8888` zeigt das Panel (das Frontend ist ins Binary eingebettet, keine separate Bereitstellung nötig)

### Standardkonto

| Element | Wert |
|----|----|
| Benutzername | `opsmini` (fest) |
| Passwort | **Wird beim ersten Start zufällig generiert**, siehe Startprotokoll |

Nach dem Start gibt das Protokoll etwa Folgendes aus:

```
initial admin account created: username=opsmini password=ggFKEX65ZVF6RNTm
```

> ⚠️ Notieren Sie dieses Passwort sofort und ändern Sie es nach der Anmeldung. Das Passwort wird nur bei der Erstinstallation einmalig generiert; danach muss es über „Benutzerverwaltung" im Panel geändert werden.

---

## 6. Produktionsbereitstellung (Linux + systemd)

### 6.1 Verzeichnisstruktur

```
/opt/opsmini/
├── opsmini              # 二进制
└── config.yaml          # 配置文件
/var/lib/opsmini/
└── opsmini.db           # 数据（自动生成，随 data 目录权限而定）
```

### 6.2 Installationsschritte

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

### 6.3 systemd-Dienst

Legen Sie `/etc/systemd/system/opsmini.service` an:

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

Aktivieren und starten:

```bash
sudo useradd -r -s /usr/sbin/nologin opsmini || true
sudo chown -R opsmini:opsmini /var/lib/opsmini /opt/opsmini
sudo systemctl daemon-reload
sudo systemctl enable --now opsmini
sudo systemctl status opsmini
```

---

## 7. Reverse Proxy (optional)

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

### 7.2 Caddy (automatisches HTTPS)

```caddyfile
panel.example.com {
    reverse_proxy 127.0.0.1:8888
}
```

### 7.3 Secret Entry

Ist `secret_entry: /opsmini_panel` konfiguriert, lautet der Panel-Pfad `http://<host>:8888/opsmini_panel`; damit lässt sich zusammen mit einem Reverse Proxy der echte Zugang verbergen.

---

## 8. Upgrade

```bash
# 1. 备份数据
sudo systemctl stop opsmini
cp /var/lib/opsmini/opsmini.db /var/lib/opsmini/opsmini.db.bak.$(date +%s)

# 2. 替换二进制
sudo cp dist/opsmini-<new>-linux-amd64 /opt/opsmini/opsmini

# 3. 重启
sudo systemctl start opsmini
```

> SQLite wird von GORM automatisch migriert; versionsübergreifende Upgrades erfordern in der Regel keine manuellen Tabellenänderungen.

---

## 9. Häufige Fragen (FAQ)

| Problem | Ursache | Lösung |
|------|------|------|
| `go mod tidy` hängt / Bad Gateway | `proxy.golang.org` ist blockiert | `export GOPROXY=https://goproxy.cn,direct` |
| Hinweis „Go-Version zu niedrig" | Abhängigkeiten erfordern Go 1.25+ | Offizielles vorkompiliertes Binary herunterladen, siehe unten |
| Build meldet einen `vendor`-Verzeichniskonflikt | `vendor/` im Projekt kollidiert mit den Go-Modul-vendor-Konventionen | Das Frontend-Abhängigkeitsverzeichnis wurde in `static/` umbenannt; kein `vendor/` erneut anlegen |
| Zugriff auf `/` liefert 301 `./` | `c.FileFromFS`-Verzeichnis-Redirect für embed.FS | Behoben durch ReadFile + c.Data (nicht zurücksetzen) |

### Go 1.25+ installieren (offizielles Binary, am schnellsten)

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

## 10. Lieferartefakte

| Artefakt | Beschreibung |
|------|------|
| Einzel-Binary `opsmini` | Backend-API + Frontend-UI + SQLite, ca. 28~30MB, statisch gelinkt |
| `configs/config.yaml` | Konfigurationsvorlage |
| `opsmini.db` | Beim ersten Lauf automatisch erzeugte Datendatei |

> Bereitstellung bedeutet nur Binary + Konfigurationsdatei zu kopieren; keine weiteren Laufzeitabhängigkeiten.
