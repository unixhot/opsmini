# Panel-Einstellungen

Die Seite „Einstellungen" des Panels verwaltet zentral die Laufzeitkonfiguration. Änderungen werden sofort wirksam und in der SQLite-Datenbank persistiert — weder Bearbeiten der Konfigurationsdatei noch Neustart des Dienstes erforderlich. Einige zuvor in `config.yaml` geschriebene Konfigurationen (AI, Agent-Token, Monitoring-Authentifizierung) wurden hierher verlagert.

> Der Zugriff auf die „Panel-Einstellungen" erfordert die Berechtigung `settings.view`, das Speichern von Änderungen die Berechtigung `settings.edit` (standardmäßig hat die Rolle `admin` diese).

## Grundeinstellungen

| Punkt | Beschreibung |
|----|------|
| Panel-Port | Lauschport des Panels (entspricht `server.port`) |
| Panel-Domäne | Zugriffsdomäne des Panels, für Link-Erzeugung und Reverse-Proxy-Szenarien |
| Sicherheitseingang | Verborgenes Pfad-Präfix (entspricht `server.secret_entry`), z. B. `/opsmini_panel`, leer = deaktiviert |
| HTTPS aktivieren | Ob HTTPS aktiviert wird (empfohlen in Kombination mit Reverse-Proxy zur TLS-Terminierung am Rand) |

## Zwei-Schritt-Verifizierung (2FA)

- **Globaler Schalter**: Nach Aktivierung müssen alle Konten mit gebundenem Authenticator bei der Anmeldung einen 6-stelligen dynamischen Code eingeben
- Jeder Benutzer kann unabhängig einen TOTP-Authenticator binden/lösen (kompatibel mit Google Authenticator / 1Password / Authenticator-Apps verschiedener Cloud-Anbieter)
- Bei Verlust des Authenticators kann am Server mit `opsmini -reset-mfa <username>` zurückgesetzt werden

Details siehe [Benutzer & Berechtigungen](../features/users-rbac.md#2fa).

## AI-LLM-Anbindung {: #ai-config }

Hier wird zentral die Modellanbindung des integrierten AI-Assistenten konfiguriert und ersetzt den früheren `ai`-Abschnitt in `config.yaml`:

| Punkt | Beschreibung |
|----|------|
| AI-Assistent aktivieren | Hauptschalter; bei Deaktivierung sind die AI-Funktionen nicht verfügbar |
| Modell-Anbieter | `openai` / `deepseek` / `qwen` / `ollama` |
| Modell | Modellname, z. B. `gpt-4o`, `deepseek-chat`, `qwen-plus` |
| API-Key | Anbieter-Schlüssel; leer = Lesen der Umgebungsvariable `OPSMINI_AI_KEY` |
| API-Adresse | Base-URL der OpenAI-kompatiblen Schnittstelle, z. B. `https://api.openai.com/v1` |
| Operationen in natürlicher Sprache ausführen | Ob die AI Betriebsoperationen ausführen darf |
| Intelligente Log-Analyse | Ob die AI-Log-Analyse aktiviert ist |
| Intelligente Alarm-Diagnose | Ob die AI-Alarm-Diagnose aktiviert ist |

> Konfigurationspriorität: Umgebungsvariable `OPSMINI_AI_KEY` > Panel-Einstellungen > verbliebene `ai.*` in der Konfigurationsdatei. Details siehe [AI-Assistent](../features/ai-assistant.md).

## API-Token

Zugriffstoken für externe Systeme erzeugen (Monitoring-Plattformen, Automatisierungsskripte, Orchestrierungstools von Drittanbietern):

- Das erzeugte Token dient zum Aufruf der `/agent/v1`-Schnittstelle; der Anfrage-Header trägt `Authorization: Bearer <token>`
- Leer bedeutet **Deaktivierung** der Agent-API
- „Zufällig erzeugen" unterstützt die Ein-Klick-Erzeugung eines starken Zufallstokens; nach dem Speichern wirksam
- Nach einem Leck sofort neu erzeugen

Die Befehls-Whitelist wird weiterhin in `agent.allowed_commands` in `config.yaml` gepflegt. Details siehe [Agent-API](../api/agent-api.md).

## Erscheinungsbild & Design

- **Benutzerdefinierte Primärfarbe**: die Marken-Primärfarbe des Panels festlegen (Unternehmensmarkenfarbe / persönliche Vorliebe), Standard OpsMini-Blau `#4f6ef7`
- **Menü-Anzeige / Sprache**: Sprache der Panel-Oberfläche (7 Sprachen) und Menü-Sichtbarkeit

## Monitoring-Export (Prometheus)

Steuert die Authentifizierung des `/metrics`-Endpunkts (node_exporter-kompatibel):

| Punkt | Beschreibung |
|----|------|
| Authentifizierung aktivieren | Ob HTTP-Basic-Authentifizierung für `/metrics` aktiviert wird |
| Benutzername / Passwort | Zugangsdaten der Basic-Authentifizierung; leerer Benutzername = offener Zugriff |

> Diese Einstellung hat Vorrang vor `metrics.user/password` in `config.yaml`. Beim Scrapen durch Prometheus muss das entsprechende `basic_auth` konfiguriert werden. Details siehe [Prometheus-Metriken](../api/prometheus.md).

## Anwendungsvorlagen (App-Store)

Verwaltung der Anwendungsvorlagen des App-Stores:

- **Offizielle Vorlagen synchronisieren**: offizielle Anwendungsvorlagen per Ein-Klick aus dem App-Store auf opsmini.com synchronisieren
- **Manuell importieren/exportieren**: In Offline-Umgebungen können Vorlagen von der offiziellen Website heruntergeladen und manuell importiert werden; lokale Vorlagen lassen sich auch als Sicherung exportieren
- **Datenverzeichnis**: persistentes Verzeichnis der Anwendungsdaten (vom Installationsskript über die Umgebungsvariable `DATA_DIR` referenziert)

## Benachrichtigungen

Konfiguration der Alarm-Benachrichtigungskanäle:

| Kanal | Beschreibung |
|------|------|
| SMTP-E-Mail | SMTP-Server / Port / Absender / Empfänger / Benutzername / Passwort |
| WeCom | Webhook-Adresse des Gruppen-Bots |
| DingTalk | Webhook-Adresse des Gruppen-Bots |
| Feishu | Webhook-Adresse des Gruppen-Bots |

Nach Auslösen eines Alarms werden Benachrichtigungen über die hier konfigurierten Kanäle gesendet.

## Log-Aufbewahrung

- **Aufbewahrungszeit der Panel-Logs**: Aufbewahrungstage für Audit- und Zugriffslogs (Standard 7 Tage); nach Ablauf automatische Bereinigung
- Die Aufbewahrungstage für Alarmereignisse können separat festgelegt werden (Standard 30 Tage)

## Speicherung der Einstellungen

Die Panel-Einstellungen werden als Schlüssel-Wert-Paare in der Tabelle `settings` der SQLite-Datenbank gespeichert und zur Laufzeit gelesen. Zwei Arten sensibler Elemente werden serverseitig geschützt:

- **Nur-Lesen-sensibel** (`jwt_secret`, `metrics_pass`): Die Schnittstelle gibt sie nicht an das Frontend zurück
- **Schreibgeschützt** (`jwt_secret`): Der Client kann sie nicht überschreiben; sie werden serverseitig automatisch erzeugt und verwaltet
