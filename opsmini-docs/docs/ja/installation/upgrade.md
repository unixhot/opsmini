# アップグレード

## アップグレード手順

```bash
# 1. サービスを停止してデータをバックアップ
sudo systemctl stop opsmini
cp /data/opsmini/opsmini.db /data/opsmini/opsmini.db.bak.$(date +%s)

# 2. バイナリを置き換え
sudo cp dist/opsmini-<new>-linux-amd64 /data/opsmini/opsmini

# 3. 再起動
sudo systemctl start opsmini
```

## 説明

- SQLite は GORM が自動マイグレーションするため、バージョンをまたいだアップグレードでも通常は手動でテーブルを変更する必要はありません
- アップグレード前には必ず `opsmini.db` をバックアップしてください。データはパネルの全業務状態です
- 新しいバージョンで設定項目が追加された場合は、`config.yaml` を同期更新する必要があります（`configs/config.yaml` テンプレートを参照）

## ロールバック

アップグレード後に異常が発生した場合は、古いバイナリに戻してデータバックアップを復元します：

```bash
sudo systemctl stop opsmini
sudo cp /data/opsmini/opsmini /data/opsmini/opsmini.new.bad
sudo cp /data/opsmini/opsmini.old /data/opsmini/opsmini   # 古いバイナリを使用
sudo cp /data/opsmini/opsmini.db.bak.<ts> /data/opsmini/opsmini.db
sudo systemctl start opsmini
```
