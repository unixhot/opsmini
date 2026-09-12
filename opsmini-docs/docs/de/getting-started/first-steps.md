# Erste Anmeldung und Initialisierung

Führe nach abgeschlossener Installation die folgenden Schritte für die erste Anmeldung und die Sicherheitsinitialisierung aus.

## 1. Initiales Passwort abrufen

Das Startprotokoll gibt die initialen Kontoinformationen aus:

```
initial admin account created: username=opsmini password=ggFKEX65ZVF6RNTm
```

Das Passwort kann auch unter `/data/opsmini/.init_passwd` (Berechtigung 600) im Installationsverzeichnis eingesehen werden.

## 2. Am Panel anmelden

1. `http://<host>:8888` im Browser öffnen
2. Benutzername `opsmini` und initiales Passwort eingeben
3. Nach erfolgreicher Anmeldung gelangst du zum Dashboard

> ⚠️ In der Produktionsumgebung das Panel unbedingt über [Reverse-Proxy + HTTPS](../configuration/https.md) bereitstellen, um Klartextübertragung zu vermeiden.

## 3. Passwort ändern

1. „Persönliche Einstellungen" öffnen
2. Unter „Passwort ändern" ein neues Passwort setzen und speichern

Bei vergessenem Passwort kannst du dich am Server anmelden und mit `opsmini -reset-pass <username>` zurücksetzen.

## 4. Zwei-Faktor-Authentifizierung binden (empfohlen)

1. Im Panel unter „Einstellungen → Zwei-Schritt-Verifizierung" die globale 2FA aktivieren
2. Die persönlichen Sicherheitseinstellungen öffnen und den TOTP-Authenticator per QR-Code binden (Google Authenticator / 1Password usw.)
3. Den dynamischen Code eingeben, um die Bindung abzuschließen

Nach der Bindung ist bei jeder Anmeldung zusätzlich ein 6-stelliger dynamischer Code erforderlich, was die Kontosicherheit deutlich erhöht. Bei Verlust des Authenticators kann mit `opsmini -reset-mfa <username>` wiederhergestellt werden.

## 5. Sicherheitseingang konfigurieren (optional)

Im Panel unter „Einstellungen → Grundeinstellungen" das Präfix des Sicherheitseingangs festlegen (oder direkt `server.secret_entry` in `config.yaml` ändern):

```yaml
server:
  secret_entry: "/opsmini_panel"
```

Nach der Konfiguration lautet die Panel-Adresse `http://<host>:8888/opsmini_panel`. In Kombination mit einem Reverse-Proxy lässt sich der tatsächliche Eingang verbergen und Port-Scans vorbeugen.

## 6. Nächste Schritte

- [AI-Assistent konfigurieren](../configuration/panel-settings.md#ai-config)
- [Externe REST API aktivieren](../api/agent-api.md)
- [Funktionen entdecken](../features/index.md)
