# systemd

Register OpsMini as a systemd service for auto-start and process supervision.

## Create the Service File

Create `/etc/systemd/system/opsmini.service`:

```ini
[Unit]
Description=OpsMini Server Panel
After=network.target

[Service]
Type=simple
ExecStart=/data/opsmini/opsmini -config /data/opsmini/config.yaml
Restart=always
RestartSec=3
# Optional hardening
User=opsmini
Group=opsmini
NoNewPrivileges=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

## Enable and Start

```bash
sudo useradd -r -s /usr/sbin/nologin opsmini || true
sudo chown -R opsmini:opsmini /data/opsmini /data/opsmini
sudo systemctl daemon-reload
sudo systemctl enable --now opsmini
sudo systemctl status opsmini
```

## Common Commands

```bash
systemctl status opsmini    # check status
systemctl restart opsmini   # restart
systemctl stop opsmini      # stop
journalctl -u opsmini -f    # follow logs
```

## Environments without systemd

```bash
nohup /data/opsmini/opsmini -config /data/opsmini/config.yaml > /var/log/opsmini.log 2>&1 &
```
