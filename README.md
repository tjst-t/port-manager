# portman - 開発ポート管理ツール

開発環境向けDHCPライクなポート管理CLI + Caddyリバースプロキシ自動設定ツール。

## 課題

複数プロジェクト（および同一プロジェクトの複数 worktree）で開発を行う際、以下の問題が発生する：

- **ポート衝突** — 各プロジェクトが好き勝手にポートを使い、衝突する
- **アクセスの面倒さ** — `localhost:????` のポート番号を覚えておく必要がある
- **プロセス殺害** — 衝突時に既存プロセスを止めてポートを奪ってしまう

## コンセプト

- DHCPのようにポートを動的に割り当て、同じworktree+nameには同じポートを返す
- Caddy Admin APIと連携してリバースプロキシを自動設定
- サブドメインベースでワンクリックアクセス可能なダッシュボード生成
- CLIツール（デーモン不要）、状態はSQLiteで永続化

## インストール

```bash
go install github.com/tjst-t/port-manager@latest
```

またはAnsible roleでセットアップ：

```yaml
# requirements.yml 経由、またはgit submoduleで deploy/ansible/roles/portman を参照
```

## 使い方

```bash
# ポート取得
portman lease --name api --expose

# ラップ実行（{}をポート番号に置換）
portman exec --name api --expose -- go run main.go --port {}

# Docker Compose連携
portman env --expose --name api --name db --output .env
docker compose up

# 一覧
portman list

# GC（不要リースの回収）
portman gc

# リース外ポート使用の検出
portman audit

# Caddy再登録
portman sync
```

## 設定

- `/etc/portman/config.toml` — アプリ設定
- `/etc/portman/services.json` — 常設サービス・保護ポート（Ansibleで管理）
- `/var/lib/portman/portman.db` — リースデータ（SQLite）

## 設計ドキュメント

詳細は [docs/DESIGN.md](docs/DESIGN.md) を参照。
