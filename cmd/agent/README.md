# cmd/agent

Main entry point of the OpsMini server panel.

`main.go` bootstraps the application:

1. Parses flags (`-config`, `-version`);
2. Loads configuration from YAML;
3. Initializes SQLite (auto-migration + seed admin);
4. Starts the metric collector (10s sampling);
5. Runs the Gin HTTP server.

Build:

```bash
go build -o opsmini ./cmd/agent
```
