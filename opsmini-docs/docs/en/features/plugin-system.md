# Plugin System

The kernel only does what every host needs: collection, audit, the agent runtime and its guardrails. Business capabilities (a specific middleware's metrics, parsing a particular log format, integrating with a particular platform's actions) are added as **plugins**.

The point is not architectural elegance, it is avoiding two concrete problems: an **ever-growing binary**, and **shipping a release for every new capability**.

## Plugins are processes, not Go plugins

- Go's `plugin` package requires an identical build environment; across versions it crashes. Ruled out immediately
- A plugin is a **standalone executable** that the panel starts as a child process and talks to over a Unix socket using **line-delimited JSON**
- A plugin crash does not affect the panel: the exit is recorded, it is restarted according to policy, and repeated failures mark it `failed` and stop retrying

## Package format

```
foo-collector-1.2.0-linux-amd64.tar.gz
├── plugin.yaml          # manifest
├── bin/plugin           # entry point (executable)
├── SIGNATURE            # ed25519 signature over the package digest
└── README.md            # optional
```

Manifest example:

```yaml
apiVersion: opsmini.plugin/v1
id: foo-collector              # globally unique, ^[a-z0-9][a-z0-9-]{2,40}$
name: Foo collector
version: 1.2.0                 # semver
vendor: Suyun Technology
description: Collects queue depth and consume rate of the Foo service
kind: collector                # collector | action | panel
entry: bin/plugin              # path inside the package
platform: [linux/amd64, linux/arm64]

capabilities:                  # declared capabilities, also used for audit
  - metrics
  - read:/etc/foo
  - exec:systemctl
  - net:outbound

runtime:
  memoryLimitMB: 128
  cpuQuotaPercent: 20
  restartPolicy: on-failure    # always | on-failure | never

requires:
  opsmini: ">=1.4.0"           # host agent version requirement
```

Verification order at install time (any failure rejects the install, no half-installed plugins):

1. Package sha256 (computed server-side, cross-checked against the uploader's claim)
2. ed25519 signature
3. `apiVersion` compatibility
4. `platform` matches the host
5. Manifest field validity and version requirement

## Three channels

A plugin and the kernel exchange four message types over the socket, all single-line JSON:

| Message | Direction | Purpose |
|---|---|---|
| `hello` | plugin → panel | handshake: who it is, version, capabilities |
| `metric` | plugin → panel | report metrics (usable by alert rules) |
| `event` | plugin → panel | report events (can become signals in the event center) |
| `heartbeat` | plugin → panel | liveness and self-check |

Plugins never touch the database directly, never call panel APIs, and do not inherit the panel's session.

## Lifecycle

```
Install (upload → verify → unpack to releases/<version>/ → point `current` symlink at it)
   ↓
Enable → start the process → wait for the hello handshake → running
   ↓
Run → crash/exit → apply restartPolicy, record restart count and last error
   ↓
Disable → SIGTERM → wait for exit → stopped
   ↓
Upgrade → unpack the new version → switch the symlink → restart (old version kept for rollback)
   ↓
Delete → stop the process → remove program files (data directory kept)
```

## Permission boundary: what is actually true

This section must be honest, otherwise "plugin security" is an empty phrase.

**What is in place:**

- A plugin is a separate process; a crash cannot take the panel down
- Installs require a valid signature (unless unsigned mode is explicitly enabled), and version, platform and manifest are checked
- Install, enable, disable and delete are all audited
- Plugins talk over a socket and cannot touch the panel database directly

**What is NOT in place in this version (do not assume it is):**

- **No switch to a dedicated system user**: plugins run as the panel's user, usually `root`
- **No cgroup limits**: `memoryLimitMB` / `cpuQuotaPercent` in the manifest are declarative, not enforced
- **`capabilities` is not enforced**: it is a declaration and an audit input, not a kernel-level allowlist
- **No syscall-level auditing of plugin behaviour**

So: **a signature proves provenance, not behaviour**. Install only from sources you trust; audit third-party plugin code yourself.

## Install prerequisites (important in this version)

Install checks `sha256` → signature → manifest in order. By default **no public key is built in and unsigned mode is off**, so an upload is rejected with "no plugin public key configured and unsigned mode is disabled".

Two ways out; after writing the setting **the panel must be restarted** (the key is read at startup):

```bash
# Option 1: configure an ed25519 public key (recommended for production)
curl -X POST http://127.0.0.1:8888/api/v1/settings \
  -H "Authorization: Bearer <token>" -H "Content-Type: application/json" \
  -d '{"plugin_public_key":"<hex-encoded 32-byte public key>"}'

# Option 2: allow unsigned packages temporarily (dev / internal networks)
curl -X POST http://127.0.0.1:8888/api/v1/settings \
  -H "Authorization: Bearer <token>" -H "Content-Type: application/json" \
  -d '{"plugin_allow_unsigned":"true"}'
```

Then `systemctl restart opsmini` and upload again from **Panel settings → Plugins**.

> This version has no input fields for these two settings in the panel; use the API above or edit the settings table. The next version will add them.

## Panel operations

**Panel settings → Plugins**:

| Action | Notes |
|---|---|
| Upload & install | choose a `*.tar.gz`, 64MB maximum |
| Enable / Disable | takes effect immediately, no panel restart |
| Restart | restart one plugin (e.g. after changing its config) |
| Delete | removes program files, keeps the data directory |
| Events | recent metrics and events reported by that plugin |

The list shows name, version, kind, state (running / stopped / failed / installed), restart count and last error.

## Current state

The mechanism is complete; **the ecosystem is empty** — v1.4.0 ships no official plugin packages. An empty plugin page after install is expected, not a fault.

Planned directions (unreleased): extended security baseline checks, middleware metric collectors, log parsing rule packs, third-party platform action integrations.

## Related

| Module | Relationship |
|---|---|
| [Autonomous agent](autonomous-agent.md) | A plugin's `event` can become a signal entering the event center |
| [Dashboard & monitoring](dashboard-monitoring.md) | A plugin's `metric` can feed alert rules |
| [Log pipeline](log-pipeline.md) | Log parsing can be a pipeline processor (lighter) or a plugin (independently upgradeable) |
| [Host security](security.md) | Official security plugins are the extension path for host security |
