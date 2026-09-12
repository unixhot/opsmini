# Upgrade

## Upgrade-Schritte

```bash
# 1. Dienst stoppen und Daten sichern
sudo systemctl stop opsmini
cp /data/opsmini/opsmini.db /data/opsmini/opsmini.db.bak.$(date +%s)

# 2. Binary ersetzen
sudo cp dist/opsmini-<new>-linux-amd64 /data/opsmini/opsmini

# 3. Neustarten
sudo systemctl start opsmini
```

## Hinweise

- SQLite wird von GORM automatisch migriert; versionsübergreifende Upgrades erfordern in der Regel keine manuellen Tabellenänderungen
- Vor dem Upgrade unbedingt `opsmini.db` sichern — die Daten sind der gesamte Geschäftszustand des Panels
- Falls eine neue Version Konfigurationselemente einführt, muss `config.yaml` entsprechend aktualisiert werden (siehe Vorlage `configs/config.yaml`)

## Rollback

Falls nach dem Upgrade ein Fehler auftritt, das alte Binary wiederherstellen und die Datensicherung einspielen:

```bash
sudo systemctl stop opsmini
sudo cp /data/opsmini/opsmini /data/opsmini/opsmini.new.bad
sudo cp /data/opsmini/opsmini.old /data/opsmini/opsmini   # altes Binary verwenden
sudo cp /data/opsmini/opsmini.db.bak.<ts> /data/opsmini/opsmini.db
sudo systemctl start opsmini
```
