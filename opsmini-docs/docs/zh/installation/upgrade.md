# 升级

## 方式一：一键升级（推荐）

重新执行安装脚本即可，检测到已安装会自动进入升级模式（替换二进制并重启，**不重置密码、不改配置**）：

```bash
curl -fsSL https://opsmini.com/install.sh | sudo bash
```

升级过程中会自动备份旧二进制到 `/data/opsmini/opsmini.bak.<时间戳>`，便于回滚。

## 方式二：手动替换二进制

```bash
# 1. 停止服务并备份数据
sudo systemctl stop opsmini
cp /data/opsmini/opsmini.db /data/opsmini/opsmini.db.bak.$(date +%s)

# 2. 替换二进制
sudo cp dist/opsmini-<new>-linux-amd64 /data/opsmini/opsmini

# 3. 重启
sudo systemctl start opsmini
```

## 说明

- SQLite 由 GORM 自动迁移，跨版本升级通常无需手动改表
- 升级前务必备份 `opsmini.db`，数据即面板的全部业务状态
- 若新版本引入配置项，需同步更新 `config.yaml`（参照 `configs/config.yaml` 模板）

## 回滚

若升级后异常，替换回旧二进制并恢复数据备份：

```bash
sudo systemctl stop opsmini
sudo cp /data/opsmini/opsmini.bak.<时间戳> /data/opsmini/opsmini   # 用旧二进制（install.sh 升级时自动备份）
sudo systemctl start opsmini
```
