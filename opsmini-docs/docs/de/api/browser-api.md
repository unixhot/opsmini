# Panel-API (/api/v1)

Die Panel-API richtet sich an die Browser-UI und verwendet Benutzersitzungs-JWT + RBAC-Authentifizierung.

## Authentifizierung

- Die Anmeldeschnittstelle `POST /auth/login` gibt `access`- und `refresh`-Token zurück
- Folgeanfragen tragen `Authorization: Bearer <access_token>`
- Nach Ablauf des Access-Tokens wird es per refresh erneuert
- Anmelde-Begrenzung (Schutz vor Brute-Force); bei aktiviertem 2FA muss nach der Anmeldung zuerst `POST /auth/mfa/verify` den dynamischen Code prüfen

## Einheitliche Antwort

```json
{ "code": 0, "message": "ok", "data": { } }
```

`code` ungleich 0 bedeutet einen Geschäftsfehler.

## Endpunkt-Übersicht

### Authentifizierung & Benutzer

| Methode | Pfad | Beschreibung |
|------|------|------|
| POST | `/auth/login` | Anmelden (mit Begrenzung) |
| POST | `/auth/refresh` | Token erneuern |
| POST | `/auth/logout` | Abmelden |
| POST | `/auth/mfa/verify` | MFA-Dynamikcode bei der Anmeldung prüfen |
| GET | `/auth/mfa/status` | MFA-Bindungsstatus des aktuellen Benutzers abfragen |
| POST | `/auth/mfa/setup` | MFA-Bindungs-QR-Code/Geheimnis erzeugen |
| POST | `/auth/mfa/enable` | MFA prüfen und aktivieren |
| POST | `/auth/mfa/disable` | MFA lösen |
| GET | `/profile` | Persönliches Profil (Spitzname/Avatar/E-Mail) |
| PUT | `/profile` | Persönliches Profil ändern |
| POST | `/profile/password` | Passwort ändern |
| GET/POST/PUT/DELETE | `/users` | Benutzer-CRUD |
| GET/POST/PUT/DELETE | `/roles` | Rollen-CRUD |
| GET | `/roles/groups` | Berechtigungsgruppen |
| GET | `/permissions` | Berechtigungen des aktuellen Benutzers |

### Panel-Einstellungen

| Methode | Pfad | Beschreibung |
|------|------|------|
| GET | `/settings` | Alle nicht-sensiblen Einstellungen lesen (`settings.view`) |
| PUT | `/settings` | Einstellungen stapelweise aktualisieren (`settings.edit`) |

> Sensible Elemente (`jwt_secret`, `metrics_pass`) werden nicht zurückgegeben; schreibgeschützte Elemente (`jwt_secret`) können nicht überschrieben werden. Details siehe [Panel-Einstellungen](../configuration/panel-settings.md).

### Monitoring & Alarme

| Methode | Pfad | Beschreibung |
|------|------|------|
| GET | `/dashboard/overview` | Dashboard-Überblick |
| GET | `/dashboard/metrics` | Dashboard-Metriken |
| GET | `/system/monitor` | Monitoring-Zusammenfassung |
| CRUD | `/alert-rules` | Alarmregeln |
| GET | `/alert-events` | Alarmereignisse |

### Benachrichtigungen

| Methode | Pfad | Beschreibung |
|------|------|------|
| GET | `/notifications` | Benachrichtigungsliste |
| GET | `/notifications/unread-count` | Anzahl ungelesener |
| PUT | `/notifications/read-all` | Alle als gelesen markieren |
| PUT | `/notifications/:id/read` | Als gelesen markieren |
| DELETE | `/notifications/:id` / `/notifications` | Einzelne löschen / leeren |

### Ressourcenverwaltung

| Methode | Pfad | Beschreibung |
|------|------|------|
| CRUD | `/websites` | Websites |
| CRUD | `/databases` | Datenbanken |
| GET/POST | `/apps` `/app-categories` | App-Store und Kategorien |
| GET/POST | `/containers` `/images` `/volumes` `/networks` | Die vier Container-Ressourcen |
| GET/POST | `/files` | Dateien |
| CRUD | `/cron-jobs` | Geplante Aufgaben |
| GET | `/logs` `/logs/tail` | Logs |

### System & Sicherheit

| Methode | Pfad | Beschreibung |
|------|------|------|
| GET | `/system/info` `/processes` `/ports` `/disks` `/network` `/users` `/groups` `/firewall` | Systemressourcen |
| GET/POST | `/security/...` | Host-Sicherheit (Baseline / FIM / Bedrohung / Firewall / Anmeldesicherheit) |
| GET | `/audit-logs` `/access-logs` | Audit- / Zugriffslogs |

### AI & Integration

| Methode | Pfad | Beschreibung |
|------|------|------|
| POST | `/ai/chat` | AI-Dialog (Funktionsaufruf) |
| POST | `/ai/chat/stream` | AI-Dialog (Streaming SSE) |
| GET/POST/PUT/DELETE | `/mcp` | MCP-Konfiguration |
| GET/POST/DELETE | `/skills` u. a. | AI-Skills (SkillHub-Suche/Installation/Upload) |

### Terminal

| Methode | Pfad | Beschreibung |
|------|------|------|
| GET | `/terminal` | WebSocket-Terminal (Authentifizierung per query token) |
| GET | `/containers/:id/exec` | Container-WebSocket-Terminal |

## Hinweise

- Alle Schreiboperationen werden durch RBAC-Berechtigungspunkte gesteuert (z. B. `website.create`, `container.edit`, `settings.edit`)
- Kritische Operationen werden im Audit-Log protokolliert, angemeldete Anfragen im Zugriffslog
