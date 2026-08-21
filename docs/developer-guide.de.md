<div align="center">

# OpsMini Developer Guide

**Entwicklerhandbuch**

[English](developer-guide.md) · [简体中文](developer-guide.zh-CN.md) · [繁體中文](developer-guide.zh-TW.md) · [日本語](developer-guide.ja.md) · [한국어](developer-guide.ko.md) · [ไทย](developer-guide.th.md) · [Deutsch](developer-guide.de.md)

</div>

---

> Version: v1.0.0 · Sprache: Deutsch · [English](developer-guide.md)

Dieses Handbuch erklärt die OpsMini-Codebasis im Detail: Architektur, Verzeichnisaufbau, jedes Modul, API-Design, Datenmodell, RBAC und wie man sie erweitert.

---

## 1. Überblick

OpsMini ist ein **leichtgewichtiges Verwaltungspanel für einen einzelnen Host-Server** (ähnlich BaoTa / 1Panel) und unterscheidet sich durch zwei Dinge:

1. **KI-gestützter Betrieb** — ein LLM-Adapter ermöglicht die Systemverwaltung in natürlicher Sprache (Diagnose, Log-Analyse, Befehlsausführung).
2. **Ein sauberer REST-Vertrag** — das Panel stellt `/api/v1` (für die Browser-UI) und `/agent/v1` (Maschine-zu-Maschine) bereit, sodass ein zentraler Proxy Metriken abrufen und viele Hosts verwalten kann.

Es wird als **einzelne statische Binärdatei** ausgeliefert: Das Vue3-Frontend wird über `go:embed` eingebettet, SQLite verwendet den reinen Go-Treiber (kein CGO), sodass die Cross-Kompilierung mit `CGO_ENABLED=0` für linux/amd64, linux/arm64, darwin und windows funktioniert.

### Tech-Stack

| Ebene | Technologie |
|-------|-------------|
| Backend | Go, Gin, GORM, `glebarez/sqlite` (reines Go), gopsutil/v4, JWT (golang-jwt/v5), bcrypt, robfig/cron/v3 |
| Frontend | Vue 3 (Single-File-SPA), ECharts, xterm.js (Web-Terminal) |
| Speicher | SQLite |
| Auslieferung | einzelne Binärdatei (`go:embed`-Frontend) |

---

## 2. Architektur

Das Backend folgt einer sauberen Schichtenarchitektur:

```
HTTP-Anfrage
   │
   ▼
router (gin.Engine, Routenregistrierung + Middleware-Verkabelung)
   │
   ▼
middleware (Authentifizierung → RBAC-Berechtigung → Audit → Zugriffsprotokoll)
   │
   ▼
api/v1 handler (Anfrage parsen/validieren, Service aufrufen, Antwort schreiben)
   │
   ▼
service (Geschäftslogik, Orchestrierung, externe Systeme)
   │
   ▼
repository (GORM-Datenzugriff)
   │
   ▼
model (auf SQLite-Tabellen abgebildete Structs)
```

Regeln:

- **handler** übernehmen nur das Parsen/Validieren von Anfragen und die Antwortgestaltung — keine Geschäftslogik.
- **services** enthalten die Geschäftslogik und orchestrieren Repositories + Systemressourcen (gopsutil, Docker SDK, crontab, pty).
- **repositories** kapseln GORM und sind die einzige Ebene, die auf die Datenbank zugreift.
- **models** sind GORM-Structs mit Tabellenzuordnung und JSON-Tags.

### Anfragefluss (authentifizierte Panel-API)

```
Browser ──► /api/v1/xxx
             │  authMw      (JWT parsen, Benutzer-ID/Rolle injizieren)
             │  permMw      (RBAC-Berechtigungsprüfung, optional pro Route)
             │  auditMw     (Audit-Protokoll aufzeichnen)
             │  accessLogMw (Zugriffsprotokoll aufzeichnen)
             ▼
          handler → service → repository → SQLite
```

Die Agent-API (`/agent/v1/*`) verwendet eine **separate Agent-Token**-Authentifizierung (kein Benutzer-JWT) und wird durch `middleware.AgentAuth` geschützt.

---

## 3. Verzeichnisstruktur

```
opsmini/
├── cmd/
│   └── agent/main.go        # Einstiegspunkt: Flags parsen, Konfiguration laden, DB initialisieren, HTTP starten
├── configs/
│   └── config.yaml          # Standard-Konfigurationsvorlage
├── internal/
│   ├── config/              # Konfigurationsladen (Config-Struct + YAML + Standardwerte)
│   ├── router/              # gin-Router, Routenregistrierung, Middleware-Verkabelung
│   ├── middleware/          # auth, perm (RBAC), audit, accesslog, cors, ratelimit, metrics_auth, agent
│   ├── api/v1/              # HTTP-Handler (Panel- + Agent-Endpunkte)
│   ├── service/             # Geschäftslogik (eine Datei pro Modul)
│   ├── repository/          # GORM-Datenzugriff (eine Datei pro Modell)
│   ├── model/               # GORM-Modelle + RBAC-Berechtigungsgruppen
│   └── pkg/
│       ├── jwt/             # JWT-Signierung/Verifizierung (access + refresh)
│       ├── response/        # einheitliche API-Antwort-Hülle
│       └── store/           # SQLite-Initialisierung, Auto-Migration, Seed-Daten
├── web/
│   ├── index.html           # Single-File-Vue3-SPA (i18n mit 7 Sprachen, Themes)
│   ├── embed.go             # go:embed bettet das Frontend in die Binärdatei ein
│   └── static/              # Offline-Frontend-Abhängigkeiten
├── install/                 # Installationsskript
├── docs/                    # Projektdokumentation
└── Makefile                 # Build- / Cross-Kompilierungs- / Versionsziele
```

---

## 4. Modulreferenz

### 4.1 `cmd/agent/main.go`

Einstiegspunkt. Zuständigkeiten:

- Flags parsen (`-config`, `-version`);
- Konfiguration über `internal/config` laden;
- SQLite über `internal/pkg/store` öffnen;
- eingebaute Rollen und das Standard-Admin-Konto seeden;
- Services + Handler konstruieren und an `internal/router` übergeben;
- den HTTP-Server (und den optionalen Metrik-Endpunkt) starten.

### 4.2 `internal/config`

`config.go` definiert den `Config`-Struct und lädt YAML aus dem `-config`-Pfad. Wenn die Datei fehlt, werden eingebaute Standardwerte zurückgegeben. Abschnitte: `server`, `database`, `jwt`, `ai`, `agent`.

### 4.3 `internal/router`

`router.go` ist der einzige Ort, an dem jede Route registriert wird:

- öffentliche Routen (`/healthz`, `/auth/login`, `/auth/refresh`, ...);
- authentifizierte Panel-Routen (`/api/v1/*`) hinter `authMw + audit + accesslog`;
- Schreib-Routen zusätzlich durch `permMw("permission.key")` geschützt;
- Agent-Routen (`/agent/v1/*`) hinter `middleware.AgentAuth`;
- WebSocket-Routen (`/terminal`, `/containers/:id/exec`).

**Konvention:** Jeder Schreib-Endpunkt (POST/PUT/DELETE) muss einen `permMw(...)`-Guard tragen. Lese-Endpunkte sind für alle angemeldeten Benutzer offen, sofern sie keine sensiblen Daten preisgeben.

### 4.4 `internal/middleware`

| Datei | Zweck |
|-------|-------|
| `auth.go` | JWT-Authentifizierung; injiziert Benutzer-ID/Rolle in den Kontext |
| `perm.go` | RBAC-Berechtigungsprüfung (`permMw`) |
| `audit.go` | schreibt Audit-Protokolleinträge |
| `accesslog.go` | schreibt Zugriffsprotokolleinträge |
| `cors.go` | CORS-Header |
| `ratelimit.go` | Anmelde-Ratenbegrenzung |
| `metrics_auth.go` | Bearer-Token-Guard für `/metrics` |
| `agent.go` | Agent-Token-Authentifizierung für `/agent/v1` |

### 4.5 `internal/api/v1`

Eine Handler-Datei pro Modul. Jeder Handler:

1. bindet/validiert die Anfrage;
2. ruft die entsprechende Service-Methode auf;
3. gibt ein einheitliches `response.OK` / `response.Error` zurück.

Panel-Handler liegen im `v1`-Paket (z. B. `system.go`, `file.go`, `skill.go`); Agent-Handler liegen in `agent.go`.

### 4.6 `internal/service`

Geschäftslogik-Ebene. Eine Datei pro Modul. Wichtige Module:

| Datei | Modul |
|-------|-------|
| `auth.go`, `user.go`, `role.go`, `totp.go` | Auth, Benutzer, RBAC, 2FA |
| `system.go`, `metrics.go`, `prometheus.go` | Host-Info, Metriken, Prometheus-Export |
| `website.go`, `database.go`, `cron.go`, `crontab.go` | Ressourcenverwaltung |
| `file.go` | Dateioperationen mit Schutz vor Pfad-Traversal |
| `docker.go` | Docker-Container/Images/Volumes/Netzwerke |
| `ai.go` | LLM-Adapter (Chat, Streaming) |
| `skill.go`, `mcp.go` | KI-Skills + MCP-Server |
| `alert.go`, `alertmonitor.go` | Alarmregeln + Auswertung |
| `security.go`, `securitymonitor.go`, `baseline.go`, `fim.go`, `threat.go`, `firewall.go`, `loginsecurity.go` | Host-Sicherheitssuite |
| `notification.go`, `audit.go`, `setting.go` | Benachrichtigungen, Audit, Einstellungen |
| `appstore.go`, `apptemplate.go`, `appcategory.go` | App-Store / Vorlagen |

### 4.7 `internal/repository`

GORM-Datenzugriff, eine Datei pro Modell. Bietet CRUD- und Abfragehelfer. Enthält niemals Geschäftslogik.

### 4.8 `internal/model`

Auf SQLite-Tabellen abgebildete GORM-Structs sowie `role.go`, die einzige maßgebliche Quelle der RBAC-Berechtigungsgruppen (`PermGroups()`, `AllPermKeys()`, `BuiltinRoles()`).

### 4.9 `internal/pkg`

| Paket | Zweck |
|-------|-------|
| `jwt` | Signierung & Verifizierung von Access-/Refresh-Tokens |
| `response` | einheitliche Antwort-Hülle `{code,message,data}` |
| `store` | SQLite öffnen, Auto-Migration, Seed (Standard-Admin) |

### 4.10 `web`

- `index.html` — Single-File-Vue3-SPA: i18n mit 7 Sprachen, Theme-System, Login, Dashboard, Monitoring, Sicherheit, Dateien, Terminal, KI-Assistent, Einstellungen.
- `embed.go` — `go:embed` bettet das Frontend in die Binärdatei ein.
- `static/` — Offline-Frontend-Abhängigkeiten (Vue, ECharts).

---

## 5. API-Design

### 5.1 Antwort-Hülle

Jeder Endpunkt gibt zurück:

```json
{ "code": 0, "message": "ok", "data": { } }
```

- `code == 0` → Erfolg, `data` enthält die Nutzlast;
- `code != 0` → Geschäftsfehler, `message` beschreibt ihn.

### 5.2 Authentifizierung

- Panel-API (`/api/v1`): JWT-Access-Token (`Authorization: Bearer <token>`), kurzlebig, wird über `/auth/refresh` erneuert.
- Agent-API (`/agent/v1`): statisches Agent-Token (Konfiguration `agent.token`).

### 5.3 Endpunkt-Familien

| Familie | Zielgruppe | Auth |
|---------|------------|------|
| `/api/v1/*` | Browser-UI | Benutzer-JWT + RBAC |
| `/agent/v1/*` | externe Systeme / Proxy | Agent-Token |

---

## 6. Datenmodell

Modelle sind GORM-Structs in `internal/model`. Tabellen werden beim Start durch `internal/pkg/store` automatisch migriert. Repräsentative Modelle:

- `User` (id, username, Passwort-Hash, Rolle, MFA-Secret, ...)
- `Role` (name, label, perms, builtin)
- `Website`, `Database`, `CronJob`
- `AlertRule`, `AlertEvent`, `Notification`
- `McpServer`, `Skill` (Skills werden als Verzeichnisse auf der Festplatte gespeichert, nicht in der DB)
- `AuditLog`, `AccessLog`, `Setting`
- Sicherheit: `BaselineResult`, `FimBaseline`, `FimChange`, `ThreatFinding`

---

## 7. RBAC & Berechtigungen

Berechtigungsgruppen werden in `internal/model/role.go` (`PermGroups()`) definiert, der einzigen Quelle der Wahrheit. Drei eingebaute Rollen:

- **admin** — Berechtigungen `"*"` (alle);
- **operator** — alle Berechtigungen außer `user.*` und `settings.edit`;
- **readonly** — nur `*.view`-Berechtigungen.

Frontend-Menüs/-Buttons werden über dieselben Schlüssel mit `hasPerm('key')` gesteuert; das Backend erzwingt sie über `permMw("key")`. Beim Hinzufügen einer neuen Schreibfunktion MÜSSEN Sie alle drei tun:

1. den Berechtigungsschlüssel zu `PermGroups()` hinzufügen;
2. die Route mit `permMw(...)` schützen;
3. den Button mit `hasPerm(...)` schützen.

---

## 8. Build & Deployment

```bash
make build          # für die aktuelle Plattform bauen
make build-all      # alle Plattformen cross-kompilieren
make version        # Versionsinformationen ausgeben
```

Die Binärdatei ist statisch (kein CGO). Siehe [`build-and-deploy.md`](build-and-deploy.md) für systemd-, Reverse-Proxy- und Upgrade-Anweisungen.

---

## 9. Entwicklungsleitfaden

### 9.1 Ein neues Modul hinzufügen

Folgen Sie dem Schichtenmuster — erstellen (oder erweitern) Sie:

1. `internal/model/xxx.go` — GORM-Struct;
2. `internal/repository/xxx.go` — Datenzugriff;
3. `internal/service/xxx.go` — Geschäftslogik;
4. `internal/api/v1/xxx.go` — Handler;
5. Routen in `internal/router/router.go` registrieren.

### 9.2 Einen neuen Schreib-Endpunkt hinzufügen

1. Berechtigungsschlüssel zu `internal/model/role.go` hinzufügen;
2. Route mit `permMw("...")` registrieren;
3. `hasPerm("...")`-Guard zum Frontend-Button hinzufügen;
4. i18n-Schlüssel (alle 7 Sprachen) in `web/index.html` hinzufügen.

### 9.3 i18n-Konvention

Die Frontend-i18n-Wörterbücher (`I18N`...`I18N8`) enthalten 7 Sprachen: `zh-CN`, `zh-TW`, `en`, `ja`, `ko`, `th`, `de`. Jeder neue Schlüssel muss zu **allen 7 Sprachen** hinzugefügt werden — der `t(key)`-Helfer fällt bei Fehlen auf `zh-CN` zurück, aber eine fehlende Übersetzung zeigt Nicht-Chinesisch-Sprechern Chinesisch.

### 9.4 Code-Stil

- Exportierte Go-Funktionen/-Typen tragen einen Dokumentationskommentar, der mit ihrem Namen beginnt.
- Kommentare werden auf Englisch verfasst.
- Jedes Verzeichnis hat eine `README.md`, die seine Dateien beschreibt.
