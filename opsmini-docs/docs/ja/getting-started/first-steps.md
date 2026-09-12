# 初回ログインと初期化

インストール完了後、以下の手順で初回ログインとセキュリティ初期化を行います。

## 1. 初期パスワードの取得

起動ログに初期アカウント情報が表示されます：

```
initial admin account created: username=opsmini password=ggFKEX65ZVF6RNTm
```

パスワードはインストールディレクトリの `/data/opsmini/.init_passwd`（権限 600）でも確認できます。

## 2. パネルへのログイン

1. ブラウザで `http://<host>:8888` を開く
2. ユーザー名 `opsmini` と初期パスワードを入力
3. ログイン成功後、ダッシュボードに移動

> ⚠️ 本番環境では必ず [リバースプロキシ + HTTPS](../configuration/https.md) 経由でパネルを公開し、平文転送を避けてください。

## 3. パスワードの変更

1. 「個人設定」に入る
2. 「パスワード変更」で新しいパスワードを設定して保存

パスワードを忘れた場合は、サーバーにログインして `opsmini -reset-pass <username>` でリセットできます。

## 4. 二要素認証のバインド（推奨）

1. パネル「設定 → 二段階認証」でグローバル 2FA を有効化
2. 個人のセキュリティ設定に入り、QR コードをスキャンして TOTP 検証アプリ（Google Authenticator / 1Password など）をバインド
3. 動的コードを入力してバインドを完了

バインド後は、ログインのたびに 6 桁の動的コードの入力が追加で必要になり、アカウントの安全性が大幅に向上します。検証アプリを紛失した場合は `opsmini -reset-mfa <username>` で復旧できます。

## 5. セキュリティ入口の設定（任意）

パネル「設定 → 基本設定」でセキュリティ入口プレフィックスを設定します（または `config.yaml` の `server.secret_entry` を直接変更）：

```yaml
server:
  secret_entry: "/opsmini_panel"
```

設定後、パネルアドレスは `http://<host>:8888/opsmini_panel` になります。リバースプロキシと組み合わせて実際の入口を隠し、ポートスキャンを防げます。

## 6. 次のステップ

- [AI アシスタントを設定](../configuration/panel-settings.md#ai-config)
- [外部向け REST API を有効化](../api/agent-api.md)
- [機能を探す](../features/index.md)
