# 卸载

## 停止并移除服务

```bash
# 停止并禁用 systemd 服务
sudo systemctl stop opsmini
sudo systemctl disable opsmini
sudo rm -f /etc/systemd/system/opsmini.service
sudo systemctl daemon-reload
```

## 移除二进制与配置

```bash
# 移除安装目录（默认 /data/opsmini，或你的自定义目录）
sudo rm -rf /data/opsmini
# 或
sudo rm -rf /data/opsmini /data/opsmini
```

## 移除专用用户（可选）

```bash
sudo userdel opsmini
```

## 说明

- 卸载不会影响宿主机上的 Docker 容器 / 镜像、Nginx 站点、数据库等由面板管理的资源，这些资源需单独处理
- 若需彻底清理，请确认 `/data/opsmini/opsmini.db` 中无需要保留的业务数据
