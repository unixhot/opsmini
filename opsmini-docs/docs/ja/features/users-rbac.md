# ユーザーと権限

OpsMini はロールベースのアクセス制御（RBAC）を採用し、パネル操作の権限をきめ細かく管理します。

## ユーザー管理

- ユーザーの作成 / 編集 / 停止 / 削除
- 最終ログイン時刻の確認
- 各ユーザーは独立した MFA（TOTP）をバインド可能
- 個人設定ページでニックネーム、アバター、メール、パスワードを変更可能

## ロールと権限

3 種類のロールが内蔵されています：

| ロール | 権限範囲 |
|------|----------|
| `admin` | 全権限（ユーザー / ロール / 設定を含む） |
| `operator` | 日常運用（アプリ、コンテナ、ファイル、ターミナル、スケジュールタスクなど） |
| `readonly` | 読み取り専用 |

カスタムロールに対応し、**権限ポイント**単位で細かく割り当てられます。例：

- `user.create` / `user.edit` / `user.delete`
- `website.create` / `website.edit` / `website.delete`
- `container.edit` / `container.delete`
- `security.scan` / `security.firewall` / `security.fim` / `security.threat`
- `settings.view` / `settings.edit`（パネル設定）
- `apps.install`、`cron.create`、`database.create`、`file.write`、`mcp.manage`、`skill.manage`、`alert.manage` など

## 二要素認証（2FA） {: #2fa }

- **グローバルスイッチ**：パネル「設定 → 二段階認証」を有効にすると、検証アプリをバインド済みのすべてのアカウントはログイン時に 6 桁の動的コードの入力が必要になります
- **ユーザーごとのバインド**：ユーザーは個人のセキュリティ設定で QR コードを生成し、Google Authenticator / 1Password / 各クラウド事業者の検証アプリ APP でスキャンしてバインドします
- **アカウント復旧**：検証アプリを紛失した場合は、サーバーにログインして `opsmini -reset-mfa <username>` を実行し、そのユーザーの MFA バインディングをリセットします

## セキュリティ設計

- パスワードは bcrypt ハッシュで保存
- 短期の access JWT（15 分）+ 失効可能な refresh token（7 日）
- JWT 署名シークレットは初回起動時に自動生成してデータベースに永続化し、設定ファイルには残しません
- ログイン失敗のレート制限とロック（ブルートフォース対策）
- 重要な操作は監査ログに記録
- パスワードを忘れた場合はサーバーにログインして `opsmini -reset-pass <username>` でリセットできます
