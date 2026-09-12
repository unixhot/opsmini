# Häufig gestellte Fragen

## Installation & Start

**F: `go mod tidy` hängt oder meldet Bad Gateway?**

Unter chinesischem Netzwerk kann `proxy.golang.org` blockiert sein; führe `export GOPROXY=https://goproxy.cn,direct` aus.

**F: Meldung „Go-Version zu niedrig"?**

OpsMini erfordert Go 1.25+. Lade das offizielle vorkompilierte Binary herunter und füge es dem PATH hinzu.

**F: Zugriff auf `/` liefert 301 oder eine leere Seite?**

Die statischen Frontend-Ressourcen sind per `go:embed` eingebettet; verwende das `make build`-Ergebnis und teile das Frontend-Verzeichnis nicht manuell auf.

## Konto & Sicherheit

**F: Administratorpasswort vergessen?**

Am Server `opsmini -config <path> -reset-pass opsmini` ausführen, um ein zufälliges Passwort zu setzen (Details siehe [Installation & Deployment](../installation/index.md)).

**F: MFA-Zwei-Faktor-Code verloren?**

`opsmini -config <path> -reset-mfa opsmini` ausführen, um die MFA-Bindung zu entfernen, danach erneut anmelden und binden.

## Deployment & Betrieb

**F: Wie führe ich ein Versions-Upgrade durch?**

`opsmini.db` sichern → Binary ersetzen → Dienst neu starten. SQLite wird von GORM automatisch migriert; in der Regel sind keine manuellen Tabellenänderungen nötig.

**F: Das Web-Terminal verbindet sich hinter einem Reverse-Proxy nicht?**

In Nginx müssen die WebSocket-Upgrade-Header aktiviert werden (`proxy_http_version 1.1` + `Upgrade`/`Connection`), Details siehe [Ports & Reverse-Proxy](../configuration/ports-proxy.md).

**F: Wie lasse ich Prometheus den lokalen Rechner überwachen?**

In der Konfiguration `metrics` aktivieren; Prometheus scrapt direkt `http://<host>:8888/metrics`.
