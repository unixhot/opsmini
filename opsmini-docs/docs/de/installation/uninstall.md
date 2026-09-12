# Deinstallation

## Dienst stoppen und entfernen

```bash
# systemd-Dienst stoppen und deaktivieren
sudo systemctl stop opsmini
sudo systemctl disable opsmini
sudo rm -f /etc/systemd/system/opsmini.service
sudo systemctl daemon-reload
```

## Binary und Konfiguration entfernen

```bash
# Installationsverzeichnis entfernen (Standard /data/opsmini oder dein benutzerdefiniertes Verzeichnis)
sudo rm -rf /data/opsmini
# oder
sudo rm -rf /data/opsmini /data/opsmini
```

## Dedizierten Benutzer entfernen (optional)

```bash
sudo userdel opsmini
```

## Hinweise

- Die Deinstallation wirkt sich nicht auf die vom Panel verwalteten Ressourcen auf dem Host aus — Docker-Container / -Images, Nginx-Sites, Datenbanken usw. Diese Ressourcen müssen separat behandelt werden.
- Falls eine vollständige Bereinigung gewünscht ist, stelle sicher, dass in `/data/opsmini/opsmini.db` keine Geschäftsdaten enthalten sind, die aufbewahrt werden müssen.
