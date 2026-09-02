# 升级

## 升级步骤

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
sudo cp /data/opsmini/opsmini /data/opsmini/opsmini.new.bad
sudo cp /data/opsmini/opsmini.old /data/opsmini/opsmini   # 用旧二进制
sudo cp /data/opsmini/opsmini.db.bak.<ts> /data/opsmini/opsmini.db
sudo systemctl start opsmini
```
