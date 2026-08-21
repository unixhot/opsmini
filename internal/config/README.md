# internal/config

Configuration loading.

`config.go` defines the `Config` struct and loads YAML from the `-config`
path. When the file does not exist, built-in defaults are returned.

Sections: `server`, `database`, `jwt`, `ai`, `agent`.
