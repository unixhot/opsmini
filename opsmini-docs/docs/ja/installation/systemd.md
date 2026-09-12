# systemd 管理

OpsMini を systemd サービスとして登録し、起動時自動起動とプロセス監視を実現します。

## サービスファイルの作成

`/etc/systemd/system/opsmini.service` を作成します：

```ini
[Unit]
Description=OpsMini Server Panel
After=network.target

[Service]
Type=simple
ExecStart=/data/opsmini/opsmini -config /data/opsmini/config.yaml
Restart=always
RestartSec=3
# セキュリティ強化（任意）
User=opsmini
Group=opsmini
NoNewPrivileges=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

## 有効化と起動

```bash
sudo useradd -r -s /usr/sbin/nologin opsmini || true
sudo chown -R opsmini:opsmini /data/opsmini /data/opsmini
sudo systemctl daemon-reload
sudo systemctl enable --now opsmini
sudo systemctl status opsmini
```

## よく使う管理コマンド

```bash
systemctl status opsmini    # 状態を確認
systemctl restart opsmini   # 再起動
systemctl stop opsmini      # 停止
journalctl -u opsmini -f    # ログをリアルタイムで確認
```

## systemd のない環境

一部のコンテナ / 軽量システムには systemd がないため、手動で起動できます：

```bash
nohup /data/opsmini/opsmini -config /data/opsmini/config.yaml > /var/log/opsmini.log 2>&1 &
```
