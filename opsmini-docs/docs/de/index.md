---
title: OpsMini — Leichtgewichtiges Linux-Server-Verwaltungspanel
hide:
  - navigation
  - toc
---

<div class="home-hero" markdown>

# OpsMini

**Leichtgewichtiges Linux-Server-Verwaltungspanel** — integrierte AI-LLM-Betriebsführung, standardmäßige REST API für externe Integration

Vergleichbar mit 宝塔 / 1Panel — macht die Einzel-Server-Verwaltung einfacher, intelligenter und besser integrierbar.

<div class="home-cta" markdown>
[Schnellinstallation](getting-started/quick-install.md){ .md-button .md-button--primary }
[Dokumentation ansehen](getting-started/index.md){ .md-button }
[:fontawesome-brands-github: GitHub](https://github.com/unixhot/opsmini){ .md-button }
</div>

</div>

---

## Zwei zentrale Alleinstellungsmerkmale

<div class="grid cards" markdown>

-   :material-robot-outline: **AI-LLM-Betriebsführung**

    ---

    Fehler in natürlicher Sprache diagnostizieren, Logs analysieren, Betriebsbefehle ausführen. Anbindung an OpenAI / DeepSeek / Qwen / Ollama — lass KI dein Ops-Copilot sein.

-   :material-api: **Standard-REST-API**

    ---

    Integrierte `/api/v1` (Panel-API) und `/agent/v1` (Machine-to-Machine-API), die von jeder Monitoring-Plattform, jedem Automatisierungsskript und jedem Orchestrierungstool direkt integriert und aufgerufen werden können.

</div>

---

## Funktionsübersicht

<div class="grid cards" markdown>

-   :material-view-dashboard-outline: **Dashboard & Monitoring**

    ---

    Echtzeit-Zeitreihen für CPU / Speicher / Festplatte / Netzwerk, geschlossener Regelkreis aus Alarmregeln und Alarmereignissen.

-   :material-docker: **Container-Verwaltung**

    ---

    Vollständige Verwaltung der vier Ressourcenarten Docker-Container, Images, Volumes und Netzwerke, dazu ein App-Store zur Ein-Klick-Installation gängiger Anwendungen.

-   :material-console: **Web-Terminal**

    ---

    SSH-ähnliches interaktives Terminal auf Basis von WebSocket + pty — Server direkt im Browser bedienen.

-   :material-shield-check-outline: **Host-Sicherheit**

    ---

    Baseline-Prüfung, File-Integrity-Monitoring (FIM), Bedrohungserkennung, Firewall, Anmeldesicherheit — die gesamte Sicherheitslage auf einen Blick.

-   :material-folder-outline: **Dateien & Websites**

    ---

    Dateien durchsuchen / hochladen / bearbeiten, Verwaltung von Nginx-Websites, Datenbanken und SSL-Zertifikaten an einem Ort.

-   :material-translate: **7 Sprachen · einzelnes Binary**

    ---

    Oberfläche in 7 Sprachen: vereinfachtes/traditionelles Chinesisch, Englisch, Japanisch, Koreanisch, Thailändisch, Deutsch; Frontend per `go:embed` eingebettet, ca. 30 MB statisch gelinkt, null Laufzeitabhängigkeiten.

</div>

---

## Ein-Klick-Installation

<div class="home-section" markdown>

### In 5 Minuten startklar

```bash
# Linux x86_64 / aarch64, Ein-Klick-Skript installiert nach /data/opsmini, Port 8888
curl -fsSL https://opsmini.com/install.sh | sudo bash
```

Beim ersten Start werden automatisch das Administratorkonto `opsmini` und ein zufälliges Passwort erzeugt (siehe Startprotokoll). Öffne anschließend `http://<host>:8888`.

[Vollständige Installationsdokumentation ansehen →](installation/index.md){ .md-button }

</div>

---

<div class="home-section" markdown>

## Jetzt starten

Mit einem einzigen Befehl bereitstellen, KI-gestützter Betrieb, standardmäßige API für nahtlose Integration.

[Installation starten](getting-started/quick-install.md){ .md-button .md-button--primary }
[Dokumentation entdecken](getting-started/index.md){ .md-button }

</div>
