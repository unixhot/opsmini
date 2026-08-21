<div align="center">

# OpsMini

**AI-Powered Server Management Panel**

[简体中文](#简体中文) · [繁體中文](#繁體中文) · [English](#english) · [日本語](#日本語) · [한국어](#한국어) · [ไทย](#ไทย) · [Deutsch](#deutsch)

</div>

---

## English

OpsMini is an AI-powered server management panel (similar to BaoTa / 1Panel), differentiated by built-in AI operations and a standard REST API for external integration.

### Features

- **AI-powered operations** — natural-language diagnosis, log analysis, command execution
- **Standard REST API** — `/api/v1` (browser UI) and `/agent/v1` (machine-to-machine)
- **Docker management** — containers, images, volumes, networks
- **Web terminal** — SSH-like interactive shell over WebSocket + pty
- **7-language i18n** — Simplified/Traditional Chinese, English, Japanese, Korean, Thai, German
- **Single binary** — frontend embedded via `go:embed`, zero runtime dependencies

### Tech Stack

Go · Gin · GORM · SQLite (pure Go) · Vue 3 · ECharts

### Quick Start

```bash
make build          # build for current platform
make build-all      # cross-compile for Linux amd64/arm64

./dist/opsmini -config configs/config.yaml
# open http://localhost:8888  (default account: opsmini — password shown in startup log)
```

### Documentation

- [Developer Guide](docs/developer-guide.md)
- [Backend Architecture](docs/backend-architecture.md)
- [Build & Deployment](docs/build-and-deploy.md)
- [Security Audit](docs/security-audit.md)

### License & Copyright

OpsMini@2026 北京速云科技有限公司 (opsmini.com)

---

## 简体中文

OpsMini 是一款 AI 驱动的主机运维面板（对标宝塔 / 1Panel），以内置 AI 大模型运维与对外标准 REST API 为核心差异化。

### 特性

- **AI 大模型运维** — 自然语言诊断、日志分析、命令执行
- **标准 REST API** — `/api/v1`（浏览器 UI）与 `/agent/v1`（机器对机器）
- **Docker 管理** — 容器、镜像、卷、网络
- **Web 终端** — WebSocket + pty 实现的类 SSH 交互式终端
- **7 语言 i18n** — 简/繁中文、英文、日文、韩文、泰文、德文
- **单二进制交付** — 前端通过 `go:embed` 嵌入，零运行时依赖

### 技术栈

Go · Gin · GORM · SQLite（纯 Go）· Vue 3 · ECharts

### 快速开始

```bash
make build          # 编译当前平台
make build-all      # 交叉编译 Linux amd64/arm64

./dist/opsmini -config configs/config.yaml
# 打开 http://localhost:8888（默认账号 opsmini，密码见首次启动日志）
```

### 文档

- [开发者手册](docs/developer-guide.zh-CN.md)
- [后端架构设计](docs/backend-architecture.md)
- [构建与部署手册](docs/build-and-deploy.md)
- [主机安全设计](docs/security-audit.md)

### 版权

OpsMini@2026 北京速云科技有限公司 (opsmini.com)

---

## 繁體中文

OpsMini 是一款 AI 驅動的主機維運面板（對標寶塔 / 1Panel），以內建 AI 大模型維運與對外標準 REST API 為核心差異化。

### 特性

- **AI 大模型運維** — 自然語言診斷、日誌分析、指令執行
- **標準 REST API** — `/api/v1`（瀏覽器 UI）與 `/agent/v1`（機器對機器）
- **Docker 管理** — 容器、映像、磁碟區、網路
- **Web 終端機** — WebSocket + pty 實作的類 SSH 互動式終端機
- **7 語言 i18n** — 簡/繁中文、英文、日文、韓文、泰文、德文
- **單一執行檔交付** — 前端透過 `go:embed` 嵌入，零執行時期相依

### 技術棧

Go · Gin · GORM · SQLite（純 Go）· Vue 3 · ECharts

### 快速開始

```bash
make build          # 編譯目前平台
make build-all      # 交叉編譯 Linux amd64/arm64

./dist/opsmini -config configs/config.yaml
# 開啟 http://localhost:8888（預設帳號 opsmini，密碼見首次啟動日誌）
```

### 文件

- [開發者手冊](docs/developer-guide.zh-CN.md)
- [後端架構設計](docs/backend-architecture.md)
- [建置與部署手冊](docs/build-and-deploy.md)
- [主機安全設計](docs/security-audit.md)

### 版權

OpsMini@2026 北京速云科技有限公司 (opsmini.com)

---

## 日本語

OpsMini は AI 駆動のサーバー管理パネル（BaoTa / 1Panel に相当）で、内蔵の AI 運用支援と外部統合のための標準 REST API が特長です。

### 機能

- **AI 運用支援** — 自然言語による診断、ログ分析、コマンド実行
- **標準 REST API** — `/api/v1`（ブラウザ UI）と `/agent/v1`（マシン間）
- **Docker 管理** — コンテナ、イメージ、ボリューム、ネットワーク
- **Web ターミナル** — WebSocket + pty による SSH ライクな対話型シェル
- **7 言語 i18n** — 簡体/繁体中国語、英語、日本語、韓国語、タイ語、ドイツ語
- **単一バイナリ** — フロントエンドを `go:embed` で埋め込み、ランタイム依存ゼロ

### 技術スタック

Go · Gin · GORM · SQLite（純 Go）· Vue 3 · ECharts

### クイックスタート

```bash
make build          # 現在のプラットフォーム向けにビルド
make build-all      # Linux amd64/arm64 向けにクロスコンパイル

./dist/opsmini -config configs/config.yaml
# http://localhost:8888 を開く（デフォルト: opsmini、パスワードは起動ログ）
```

### ドキュメント

- [開発者ガイド](docs/developer-guide.md)
- [バックエンドアーキテクチャ](docs/backend-architecture.md)
- [ビルドとデプロイ](docs/build-and-deploy.md)
- [ホストセキュリティ設計](docs/security-audit.md)

### 著作権

OpsMini@2026 北京速云科技有限公司 (opsmini.com)

---

## 한국어

OpsMini는 AI 기반 서버 관리 패널(BaoTa / 1Panel 상당)로, 내장 AI 운영 지원과 외부 통합을 위한 표준 REST API가 특징입니다.

### 기능

- **AI 운영 지원** — 자연어 진단, 로그 분석, 명령 실행
- **표준 REST API** — `/api/v1`(브라우저 UI) 및 `/agent/v1`(머신 간)
- **Docker 관리** — 컨테이너, 이미지, 볼륨, 네트워크
- **웹 터미널** — WebSocket + pty 기반 SSH 유사 대화형 셸
- **7개 언어 i18n** — 간체/번체 중국어, 영어, 일본어, 한국어, 태국어, 독일어
- **단일 바이너리** — 프런트엔드를 `go:embed`로 내장, 런타임 의존성 없음

### 기술 스택

Go · Gin · GORM · SQLite(순수 Go) · Vue 3 · ECharts

### 빠른 시작

```bash
make build          # 현재 플랫폼용 빌드
make build-all      # Linux amd64/arm64 크로스 컴파일

./dist/opsmini -config configs/config.yaml
# http://localhost:8888 열기 (기본 계정: opsmini, 비밀번호는 시작 로그에서 확인)
```

### 문서

- [개발자 가이드](docs/developer-guide.md)
- [백엔드 아키텍처](docs/backend-architecture.md)
- [빌드 및 배포](docs/build-and-deploy.md)
- [호스트 보안 설계](docs/security-audit.md)

### 저작권

OpsMini@2026 北京速云科技有限公司 (opsmini.com)

---

## ไทย

OpsMini เป็นแผงจัดการเซิร์ฟเวอร์ที่ขับเคลื่อนด้วย AI (เทียบเท่า BaoTa / 1Panel) โดยมีจุดเด่นคือ AI ปฏิบัติการในตัวและ REST API มาตรฐานสำหรับการเชื่อมต่อภายนอก

### คุณสมบัติ

- **AI ปฏิบัติการ** — วินิจฉัยด้วยภาษาธรรมชาติ วิเคราะห์ล็อก และรันคำสั่ง
- **REST API มาตรฐาน** — `/api/v1` (UI บราวเซอร์) และ `/agent/v1` (ระหว่างเครื่อง)
- **จัดการ Docker** — คอนเทนเนอร์ อิมเมจ วอลุ่ม และเครือข่าย
- **เทอร์มินัลเว็บ** — เชลล์แบบโต้ตอบคล้าย SSH ผ่าน WebSocket + pty
- **i18n 7 ภาษา** — จีนตัวย่อ/ตัวเต็ม อังกฤษ ญี่ปุ่น เกาหลี ไทย และเยอรมัน
- **ไบนารีเดี่ยว** — ฝังฟรอนต์เอนด์ด้วย `go:embed` ไม่มี dependency ขณะรันไทม์

### สแต็กเทคโนโลยี

Go · Gin · GORM · SQLite (Go แท้) · Vue 3 · ECharts

### เริ่มต้นใช้งาน

```bash
make build          # บิลด์สำหรับแพลตฟอร์มปัจจุบัน
make build-all      # ครอสคอมไพล์สำหรับ Linux amd64/arm64

./dist/opsmini -config configs/config.yaml
# เปิด http://localhost:8888 (บัญชีเริ่มต้น: opsmini ดูรหัสผ่านในล็อกเริ่มต้น)
```

### เอกสาร

- [คู่มือนักพัฒนา](docs/developer-guide.md)
- [สถาปัตยกรรมแบ็กเอนด์](docs/backend-architecture.md)
- [การบิลด์และการปรับใช้](docs/build-and-deploy.md)
- [การออกแบบความปลอดภัยโฮสต์](docs/security-audit.md)

### ลิขสิทธิ์

OpsMini@2026 北京速云科技有限公司 (opsmini.com)

---

## Deutsch

OpsMini ist ein KI-gesteuertes Server-Verwaltungspanel (vergleichbar mit BaoTa / 1Panel), das sich durch integrierten KI-Betrieb und eine standardisierte REST API für externe Integration auszeichnet.

### Funktionen

- **KI-gestützter Betrieb** — Diagnose in natürlicher Sprache, Log-Analyse, Befehlsausführung
- **Standard-REST-API** — `/api/v1` (Browser-UI) und `/agent/v1` (Machine-to-Machine)
- **Docker-Verwaltung** — Container, Images, Volumes, Netzwerke
- **Web-Terminal** — SSH-ähnliche interaktive Shell über WebSocket + pty
- **i18n in 7 Sprachen** — Chinesisch (vereinfacht/traditionell), Englisch, Japanisch, Koreanisch, Thailändisch, Deutsch
- **Einzelnes Binary** — Frontend per `go:embed` eingebettet, keine Laufzeitabhängigkeiten

### Tech-Stack

Go · Gin · GORM · SQLite (reines Go) · Vue 3 · ECharts

### Schnellstart

```bash
make build          # Build für die aktuelle Plattform
make build-all      # Cross-Compile für Linux amd64/arm64

./dist/opsmini -config configs/config.yaml
# http://localhost:8888 öffnen (Standardkonto: opsmini, Passwort im Startprotokoll)
```

### Dokumentation

- [Entwicklerhandbuch](docs/developer-guide.md)
- [Backend-Architektur](docs/backend-architecture.md)
- [Build & Deployment](docs/build-and-deploy.md)
- [Host-Sicherheitsdesign](docs/security-audit.md)

### Lizenz & Urheberrecht

OpsMini@2026 北京速云科技有限公司 (opsmini.com)
