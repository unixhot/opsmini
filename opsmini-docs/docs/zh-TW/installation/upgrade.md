# 升級

## 升級步驟

```bash
# 1. 停止服务并备份数据
sudo systemctl stop opsmini
cp /data/opsmini/opsmini.db /data/opsmini/opsmini.db.bak.$(date +%s)

# 2. 替换二进制
sudo cp dist/opsmini-<new>-linux-amd64 /data/opsmini/opsmini

# 3. 重启
sudo systemctl start opsmini
```

## 說明

- SQLite 由 GORM 自動遷移，跨版本升級通常無需手動改表
- 升級前務必備份 `opsmini.db`，資料即面板的全部業務狀態
- 若新版本引入配置項，需同步更新 `config.yaml`（參照 `configs/config.yaml` 模板）

## 回滾

若升級後異常，替換回舊二進位制並恢復資料備份：

```bash
sudo systemctl stop opsmini
sudo cp /data/opsmini/opsmini /data/opsmini/opsmini.new.bad
sudo cp /data/opsmini/opsmini.old /data/opsmini/opsmini   # 用旧二进制
sudo cp /data/opsmini/opsmini.db.bak.<ts> /data/opsmini/opsmini.db
sudo systemctl start opsmini
```
