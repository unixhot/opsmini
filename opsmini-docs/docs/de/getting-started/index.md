# Schnellstart

Willkommen bei OpsMini — einem leichtgewichtigen Linux-Server-Verwaltungspanel. Dieser Leitfaden hilft dir, in kürzester Zeit zu installieren, dich anzumelden und die Kernfunktionen kennenzulernen.

## Hier beginnen

<div class="grid cards" markdown>

-   :material-rocket-launch: **[Schnellinstallation in 5 Minuten](quick-install.md)**

    ---

    Bereitstellung mit einem einzigen Befehl; beim ersten Start werden Administratorkonto und zufälliges Passwort automatisch erzeugt.

-   :material-login: **[Erste Anmeldung](first-steps.md)**

    ---

    Am Panel anmelden, Passwort ändern, Zwei-Faktor-Authentifizierung (MFA) binden.

-   :material-information-outline: **[Produktvorstellung](introduction.md)**

    ---

    Positionierung, Kernfunktionen und Technologie-Stack von OpsMini kennenlernen.

</div>

## Kernkonzepte

| Konzept | Beschreibung |
|------|------|
| **Einzel-Server-Panel** | Jeder Host betreibt eine unabhängige OpsMini-Instanz, ohne zentralen Knoten |
| **Einzel-Binary-Auslieferung** | Frontend-UI per `go:embed` eingebettet; Deployment bedeutet lediglich das Kopieren einer ausführbaren Datei |
| **Doppelte API** | `/api/v1` für die Browser-UI (JWT + RBAC), `/agent/v1` für Maschinen/Drittanbieter-Integration (Token + Befehls-Whitelist) |
| **AI-LLM-Betriebsführung** | Diagnose in natürlicher Sprache, Log-Analyse, Befehlsausführung, Anbindung an OpenAI / DeepSeek / Qwen / Ollama |

## Nächste Schritte

Nach abgeschlossener Installation empfiehlt sich die Lektüre in dieser Reihenfolge:

1. [Installation & Deployment](../installation/index.md) — Systemanforderungen, Ein-Klick-Installation, manuelles Deployment, Reverse-Proxy
2. [Konfigurationsleitfaden](../configuration/index.md) — Konfigurationsdatei, Datenspeicherung, HTTPS
3. [Funktionsleitfaden](../features/index.md) — Dashboard, AI-Assistent, App-Verwaltung, Host-Sicherheit u. a.
4. [REST API](../api/index.md) — Integrationsschnittstellen nach außen
