# Benutzer & Berechtigungen

OpsMini verwendet rollenbasierte Zugriffskontrolle (RBAC) und verwaltet die Panel-Betriebsberechtigungen fein granular.

## Benutzerverwaltung

- Benutzer erstellen / bearbeiten / deaktivieren / löschen
- Zeitpunkt der letzten Anmeldung ansehen
- Jeder Benutzer kann unabhängig MFA (TOTP) binden
- Die Seite der persönlichen Einstellungen unterstützt das Ändern von Spitzname, Avatar, E-Mail und Passwort

## Rollen & Berechtigungen

Drei integrierte Rollen:

| Rolle | Berechtigungsumfang |
|------|----------|
| `admin` | Alle Berechtigungen (inkl. Benutzer / Rollen / Einstellungen) |
| `operator` | Täglicher Betrieb (Apps, Container, Dateien, Terminal, geplante Aufgaben usw.) |
| `readonly` | Nur-Lesen-Anzeige |

Benutzerdefinierte Rollen werden unterstützt und nach **Berechtigungspunkten** fein zugewiesen, z. B.:

- `user.create` / `user.edit` / `user.delete`
- `website.create` / `website.edit` / `website.delete`
- `container.edit` / `container.delete`
- `security.scan` / `security.firewall` / `security.fim` / `security.threat`
- `settings.view` / `settings.edit` (Panel-Einstellungen)
- `apps.install`, `cron.create`, `database.create`, `file.write`, `mcp.manage`, `skill.manage`, `alert.manage` u. a.

## Zwei-Faktor-Authentifizierung (2FA) {: #2fa }

- **Globaler Schalter**: Nach Aktivierung unter Panel „Einstellungen → Zwei-Schritt-Verifizierung" müssen alle Konten mit gebundenem Authenticator bei der Anmeldung einen 6-stelligen dynamischen Code eingeben
- **Binden pro Benutzer**: Der Benutzer erzeugt in den persönlichen Sicherheitseinstellungen einen QR-Code und bindet ihn per Scan mit Google Authenticator / 1Password / Authenticator-Apps verschiedener Cloud-Anbieter
- **Konto-Wiederherstellung**: Bei Verlust des Authenticators kann am Server mit `opsmini -reset-mfa <username>` die MFA-Bindung des Benutzers zurückgesetzt werden

## Sicherheitsdesign

- Passwörter als bcrypt-Hash gespeichert
- Kurzlebiges Access-JWT (15 Minuten) + widerrufbares Refresh-Token (7 Tage)
- JWT-Signaturschlüssel wird beim ersten Start automatisch erzeugt und in der Datenbank persistiert, landet nicht in der Konfigurationsdatei
- Begrenzung und Sperrung bei fehlgeschlagenen Anmeldungen (Schutz vor Brute-Force)
- Kritische Operationen werden im Audit-Log protokolliert
- Bei vergessenem Passwort kann am Server mit `opsmini -reset-pass <username>` zurückgesetzt werden
