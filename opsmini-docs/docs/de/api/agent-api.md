# Agent-Anbindung (gRPC)

Im OpsMini-Prozess steckt ein **Agent-Client**: Er baut von sich aus einen langlebigen gRPC-Bidirektionalstream auf, registriert sich, sendet Heartbeats, empfängt Aufgaben und überträgt Dateien in beide Richtungen. **Weder SSH noch eingehende Ports sind nötig** — ideal hinter NAT, im internen Netz oder ohne öffentliche IP.

!!! warning "Zur alten /agent/v1 REST-API"
    Die `/agent/v1` REST-API und die Seite "Einstellungen → API Token" wurden **mit v1.2.1 entfernt**. Für Steuerung von außen **MCP** nutzen (`/mcp`, siehe [Panel-API](browser-api.md)); für Flottenverwaltung den hier beschriebenen gRPC-Agent.

## Kommunikationsmodell

| Richtung | |
|------|------|
| Agent → Server | `Register` · `Heartbeat` · `TaskResult` · `FileChunkAck` |
| Server → Agent | `RegisterAck` · `ExecuteTask` · `PushFileTask` · `Ping` |

- Ein Port genügt: HTTP/2-Multiplexing statt SaltStack-Doppelport
- Reconnect mit exponentiellem Backoff, regelmäßige Heartbeats
- Meldet sich derselbe Hostname neu, wird die alte Verbindung verworfen

## Konfiguration

```yaml
agent:
  server_addr: "47.104.172.52:4507"
  server_token: "<token from server>"
  heartbeat_seconds: 30
```

Bleibt `server_addr` leer, ist der Agent-Modus aus; das Panel läuft weiter eigenständig.

## Fähigkeiten

| Fähigkeit | Hinweis |
|------|------|
| Befehle / Skripte | `ExecuteTask` unterscheidet per oneof command und script |
| Dateien verteilen | `PushFileTask` streamt Chunks mit `resume_offset` und md5-Prüfung |
| Dateien abholen | Der Agent liest und schickt die Datei zurück |
| Metriken | Nicht über gRPC — weiterhin `/metrics` für Prometheus |

!!! note "Wo der Server liegt"
    OpsMini Server liegt im OpsAnt-Projekt (`github.com/opsany/opsmini-server`); dieses Repository enthält nur die Agent-Seite (`internal/agent/client.go`).
