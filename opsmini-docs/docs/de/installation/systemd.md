# systemd-Verwaltung

Registriere OpsMini als systemd-Dienst, um Autostart beim Booten und Prozessüberwachung zu ermöglichen.

## Dienstdatei erstellen

Erstelle `/etc/systemd/system/opsmini.service`:

```ini
[Unit]
Description=OpsMini Server Panel
After=network.target

[Service]
Type=simple
ExecStart=/data/opsmini/opsmini -config /data/opsmini/config.yaml
Restart=always
RestartSec=3
# Sicherheitshärtung (optional)
User=opsmini
Group=opsmini
NoNewPrivileges=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

## Aktivieren und starten

```bash
sudo useradd -r -s /usr/sbin/nologin opsmini || true
sudo chown -R opsmini:opsmini /data/opsmini /data/opsmini
sudo systemctl daemon-reload
sudo systemctl enable --now opsmini
sudo systemctl status opsmini
```

## Häufige Verwaltungsbefehle

```bash
systemctl status opsmini    # Status anzeigen
systemctl restart opsmini   # Neustarten
systemctl stop opsmini      # Stoppen
journalctl -u opsmini -f    # Logs in Echtzeit anzeigen
```

## Umgebungen ohne systemd

Einige Container-/Minimalsysteme haben kein systemd; dort manuell starten:

```bash
nohup /data/opsmini/opsmini -config /data/opsmini/config.yaml > /var/log/opsmini.log 2>&1 &
```
