# データストレージ

OpsMini は組み込みの SQLite を使用してすべての業務データ（ユーザー、タスク、アラート、ウェブサイト、監査ログなど）を保存します。

## データファイル

- デフォルトパス：ワンクリックインストールでは `/data/opsmini/opsmini.db`（`config.yaml` の `database.path` で指定。デフォルトでは設定ファイルと同じディレクトリの相対パス）
- 初回起動時に自動作成されテーブルが作成されます（GORM 自動マイグレーション）
- センシティブフィールド（シークレット / Token / TOTP secret）は AES-GCM で暗号化してから DB に保存されます

## バックアップ

SQLite は単一ファイルなので、コピーするだけでバックアップできます：

```bash
# 整合性を保つため、先にサービスを停止してからバックアップすることを推奨
sudo systemctl stop opsmini
cp /data/opsmini/opsmini.db /backup/opsmini.db.$(date +%s)
sudo systemctl start opsmini
```

## 復元

```bash
sudo systemctl stop opsmini
cp /backup/opsmini.db.<ts> /data/opsmini/opsmini.db
sudo systemctl start opsmini
```

## 保存内容の概要

| データ | テーブル | 説明 |
|------|-----|------|
| ユーザーとロール | `users` / `roles` | アカウント、パスワードハッシュ、RBAC ロールと権限 |
| セッション | `sessions` | refresh token と有効期間 |
| パネル設定 | `settings` | テーマ / 言語 / メニュー表示/非表示などの KV |
| ウェブサイト / データベース | `websites` / `databases` | サイトとデータベースインスタンスの記録 |
| スケジュールタスク | `cron_jobs` | パネルのスケジュールタスク |
| アラートルール / イベント | `alert_rules` / `alert_events` | 監視アラート |
| 監査 / アクセスログ | `audit_logs` / `access_logs` | 操作とアクセスの記録（デフォルト保持 7 日） |

## データクリーンアップ

- 監査ログとアクセスログはデフォルトで 7 日保持され、1 時間ごとに自動クリーンアップされます。保持日数はパネル設定で調整できます
- アラートイベントは保持期間に従って自動クリーンアップされます（デフォルト 30 日）
