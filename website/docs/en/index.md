---
title: OpsMini — Lightweight Linux Server Panel
hide:
  - navigation
  - toc
---

<div class="home-hero" markdown>

# OpsMini

**Lightweight Linux server panel** — built-in AI-powered operations and a standard REST API for external integration

A lean, smarter, more integrable alternative to BaoTa / 1Panel.

<div class="home-cta" markdown>
[Quick Install](getting-started/quick-install.md){ .md-button .md-button--primary }
[Documentation](getting-started/index.md){ .md-button }
[:fontawesome-brands-github: GitHub](https://github.com/unixhot/opsmini){ .md-button }
</div>

</div>

---

## Two Core Differentiators

<div class="grid cards" markdown>

-   :material-robot-outline: **AI-Powered Operations**

    ---

    Diagnose issues, analyze logs, and run operations in natural language. Connect OpenAI / DeepSeek / Qwen / Ollama.

-   :material-api: **Standard REST API**

    ---

    Built-in `/api/v1` (panel API) and `/agent/v1` (machine-to-machine), ready for monitoring, automation, and orchestration tools.

</div>

---

## Feature Highlights

<div class="grid cards" markdown>

-   :material-view-dashboard-outline: **Dashboard & Monitoring**

    ---

    Real-time CPU / memory / disk / network metrics, with alert rules and events.

-   :material-docker: **Container Management**

    ---

    Full control of Docker containers, images, volumes, and networks, plus a one-click app store.

-   :material-console: **Web Terminal**

    ---

    SSH-like interactive shell over WebSocket + pty, right in the browser.

-   :material-shield-check-outline: **Host Security**

    ---

    Baseline checks, file integrity monitoring (FIM), threat detection, firewall, and login security.

-   :material-folder-outline: **Files & Websites**

    ---

    File browsing / upload / editing, plus Nginx websites, databases, and SSL certificates.

-   :material-translate: **7 Languages · Single Binary**

    ---

    7-language UI; frontend embedded via `go:embed`, ~30MB static binary, zero runtime dependencies.

</div>

---

## One-Click Install

<div class="home-section" markdown>

### Up and running in 5 minutes

```bash
# Linux x86_64 / aarch64, installs to /data/opsmini on port 8888
curl -fsSL https://opsmini.com/install.sh | sudo bash
```

An admin account `opsmini` with a random password is generated on first start. Open `http://<host>:8888`.

[Full installation guide →](installation/index.md){ .md-button }

</div>

---

<div class="home-section" markdown>

## Get Started

Deploy with one command. Let AI power your operations. Integrate via standard API.

[Start Now](getting-started/quick-install.md){ .md-button .md-button--primary }
[Explore Docs](getting-started/index.md){ .md-button }

</div>
