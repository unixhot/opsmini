# AI-Assistent

OpsMini enthält integrierte AI-LLM-Betriebsfähigkeiten: Systemdiagnose, Log-Analyse und Befehlsausführung in natürlicher Sprache.

## Fähigkeiten

- **Diagnose in natürlicher Sprache**: Fehlersymptome beschreiben; die AI liefert auf Basis des Echtzeit-Systemzustands Diagnoseempfehlungen
- **Log-Analyse**: Die AI anomale Logs analysieren und Fehlerursachen eingrenzen lassen
- **Befehlsausführung**: Die AI wandelt Absichten in natürlicher Sprache in Betriebsbefehle um, die nach Berechtigungsbestätigung ausgeführt werden
- **Tiefes Nachdenken**: optionaler Modus „erst nachdenken, dann antworten" zur Lösung komplexer Schlussfolgerungsprobleme
- **Integrierte Tools**: injiziert Host-Systeminformationen in Echtzeit (CPU/Speicher/Festplatte/Prozesse/Ports/Firewall) und unterstützt das Suchen und Installieren von Skills im SkillHub über Funktionsaufrufe

## Unterstützte LLM-Anbieter

| Provider | Beschreibung |
|----------|------|
| `openai` | OpenAI-kompatible Schnittstelle |
| `deepseek` | DeepSeek |
| `qwen` | 通义千问 (DashScope-kompatible Schnittstelle) |
| `ollama` | Lokales Ollama-Deployment |

## Konfiguration

Einheitlich in **Panel „Einstellungen → AI-LLM-Anbindung"** konfigurieren (der frühere `ai`-Abschnitt in `config.yaml` wurde hierher verlagert, zur Laufzeit sofort wirksam):

- AI-Assistent aktivieren (Hauptschalter)
- Modell-Anbieter / Modellname / API-Key / API-Adresse
- Schalter für Operationen in natürlicher Sprache / intelligente Log-Analyse / intelligente Alarm-Diagnose

> Konfigurationspriorität: Umgebungsvariable `OPSMINI_AI_KEY` > Panel-Einstellungen > verbliebene `ai.*` in der Konfigurationsdatei. Es wird empfohlen, den Schlüssel in einer Umgebungsvariable zu speichern, um eine Speicherung auf der Festplatte zu vermeiden. Details siehe [Panel-Einstellungen](../configuration/panel-settings.md#ai-config).

## Sicherheitsmechanismen

- Wenn die AI eine „Ausführungsabsicht" erkennt, erzeugt sie eine strukturierte action + Parameter
- Ausführung nur bei Treffer in der Berechtigungs-Whitelist; nicht autorisierte / risikoreiche Operationen erfordern eine zweite Bestätigung durch den Benutzer
- Ausführungsaktionen werden im Audit-Log protokolliert
- Nach Deaktivieren des Schalters „AI-Assistent aktivieren" werden AI-bezogene Schnittstellen direkt abgelehnt

## MCP-Verwaltung

Unterstützt die Konfiguration von MCP-Diensten (Model Context Protocol), um die Fähigkeiten und Datenquellen des AI-Assistenten zu erweitern. Unter „AI-Assistent → MCP" lassen sich MCP-Konfigurationen erstellen, lesen, ändern und löschen; unterstützt werden die beiden Übertragungsarten Befehl (stdio) und URL (SSE/HTTP).

## Skills (Skill Hub)

Der integrierte Tab „Skills" ist an den SkillHub-Skillmarkt angebunden:

- Skills suchen und per Ein-Klick installieren
- Lokale Skills hochladen, Skills manuell erstellen
- Installierte Skills aktivieren/deaktivieren, löschen
