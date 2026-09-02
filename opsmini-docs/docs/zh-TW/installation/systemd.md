# systemd 託管

將 OpsMini 註冊為 systemd 服務，實現開機自啟與程序守護。

## 建立服務檔案

建立 `/etc/systemd/system/opsmini.service`：

```ini
[Unit]
Description=OpsMini Server Panel
After=network.target

[Service]
Type=simple
ExecStart=/data/opsmini/opsmini -config /data/opsmini/config.yaml
Restart=always
RestartSec=3
# 安全加固（可选）
User=opsmini
Group=opsmini
NoNewPrivileges=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

## 啟用並啟動

```bash
sudo useradd -r -s /usr/sbin/nologin opsmini || true
sudo chown -R opsmini:opsmini /data/opsmini /data/opsmini
sudo systemctl daemon-reload
sudo systemctl enable --now opsmini
sudo systemctl status opsmini
```

## 常用管理命令

```bash
systemctl status opsmini    # 查看状态
systemctl restart opsmini   # 重启
systemctl stop opsmini      # 停止
journalctl -u opsmini -f    # 实时查看日志
```

## 無 systemd 環境

部分容器/精簡系統沒有 systemd，可手動啟動：

```bash
nohup /data/opsmini/opsmini -config /data/opsmini/config.yaml > /var/log/opsmini.log 2>&1 &
```
