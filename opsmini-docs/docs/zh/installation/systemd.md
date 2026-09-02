# systemd 托管

将 OpsMini 注册为 systemd 服务，实现开机自启与进程守护。

## 创建服务文件

创建 `/etc/systemd/system/opsmini.service`：

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

## 启用并启动

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

## 无 systemd 环境

部分容器/精简系统没有 systemd，可手动启动：

```bash
nohup /data/opsmini/opsmini -config /data/opsmini/config.yaml > /var/log/opsmini.log 2>&1 &
```
