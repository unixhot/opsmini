# Uninstall

## Stop and Remove the Service

```bash
sudo systemctl stop opsmini
sudo systemctl disable opsmini
sudo rm -f /etc/systemd/system/opsmini.service
sudo systemctl daemon-reload
```

## Remove Binary and Config

```bash
# Remove the install directory (default /data/opsmini, or your custom path)
sudo rm -rf /data/opsmini
# or
sudo rm -rf /data/opsmini /data/opsmini
```

## Remove the Dedicated User (Optional)

```bash
sudo userdel opsmini
```

## Notes

- Uninstalling does not remove resources managed by the panel (Docker containers/images, Nginx sites, databases); handle those separately.
- Confirm no needed business data remains in `opsmini.db` before cleanup.
