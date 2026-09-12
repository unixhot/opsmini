# アンインストール

## サービスの停止と削除

```bash
# systemd サービスを停止して無効化
sudo systemctl stop opsmini
sudo systemctl disable opsmini
sudo rm -f /etc/systemd/system/opsmini.service
sudo systemctl daemon-reload
```

## バイナリと設定の削除

```bash
# インストールディレクトリを削除（デフォルト /data/opsmini、またはカスタムディレクトリ）
sudo rm -rf /data/opsmini
# または
sudo rm -rf /data/opsmini /data/opsmini
```

## 専用ユーザーの削除（任意）

```bash
sudo userdel opsmini
```

## 説明

- アンインストールはホストマシン上の Docker コンテナ / イメージ、Nginx サイト、データベースなどパネルが管理するリソースには影響しません。これらのリソースは個別に処理する必要があります
- 完全にクリーンアップする場合は、`/data/opsmini/opsmini.db` に保持する必要のある業務データがないことを確認してください
