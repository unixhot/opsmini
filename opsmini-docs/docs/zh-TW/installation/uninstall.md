# 解除安裝

## 停止並移除服務

```bash
# 停止并禁用 systemd 服务
sudo systemctl stop opsmini
sudo systemctl disable opsmini
sudo rm -f /etc/systemd/system/opsmini.service
sudo systemctl daemon-reload
```

## 移除二進位制與配置

```bash
# 移除安装目录（默认 /data/opsmini，或你的自定义目录）
sudo rm -rf /data/opsmini
# 或
sudo rm -rf /data/opsmini /data/opsmini
```

## 移除專用使用者（可選）

```bash
sudo userdel opsmini
```

## 說明

- 解除安裝不會影響宿主機上的 Docker 容器 / 映象、Nginx 站點、資料庫等由面板管理的資源，這些資源需單獨處理
- 若需徹底清理，請確認 `/data/opsmini/opsmini.db` 中無需要保留的業務資料
