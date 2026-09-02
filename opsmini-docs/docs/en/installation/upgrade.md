# Upgrade

## Steps

```bash
# 1. Stop the service and back up data
sudo systemctl stop opsmini
cp /data/opsmini/opsmini.db /data/opsmini/opsmini.db.bak.$(date +%s)

# 2. Replace the binary
sudo cp dist/opsmini-<new>-linux-amd64 /data/opsmini/opsmini

# 3. Restart
sudo systemctl start opsmini
```

## Notes

- SQLite is auto-migrated by GORM; cross-version upgrades usually need no manual table changes.
- Always back up `opsmini.db` — it holds all panel state.
- If a new version adds config keys, update `config.yaml` accordingly.

## Rollback

```bash
sudo systemctl stop opsmini
sudo cp /data/opsmini/opsmini /data/opsmini/opsmini.new.bad
sudo cp /data/opsmini/opsmini.old /data/opsmini/opsmini   # old binary
sudo cp /data/opsmini/opsmini.db.bak.<ts> /data/opsmini/opsmini.db
sudo systemctl start opsmini
```
