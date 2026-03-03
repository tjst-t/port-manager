# portman — Claude 向けテストサーバー起動ガイド

このドキュメントは、Claude（AI）が各プロジェクトで portman を使ったテストサーバー起動スクリプト（Makefile ターゲット等）を正しく作成するためのリファレンスです。

---

## 各プロジェクトの CLAUDE.md に書くこと

以下をプロジェクトの CLAUDE.md に追加してください。
サーバー起動スクリプトの作成・変更を依頼する際に、Claude がこのガイドを参照します。

```markdown
## サーバー起動

- テストサーバーは `make serve` で起動すること。ポート番号を直接指定してはいけない。
- サーバー起動スクリプトを作成・変更する場合は、portman ガイドを参照すること:
  https://raw.githubusercontent.com/tjst-t/port-manager/main/docs/CLAUDE_INTEGRATION.md
- .env ファイルを git commit してはいけない（.gitignore に追加すること）
```

プロジェクトに複数サービスがある場合は、ターゲット名も明記してください。

```markdown
## サーバー起動

- `make serve` — API サーバー起動
- `make serve-frontend` — フロントエンド起動
- サーバー起動スクリプトを作成・変更する場合は、portman ガイドを参照すること:
  https://raw.githubusercontent.com/tjst-t/port-manager/main/docs/CLAUDE_INTEGRATION.md
```

---

## portman とは

開発環境のポート番号を自動管理する CLI ツール。
プロジェクト・ブランチ・サービス名の組み合わせで一意にポートをリースし、衝突を防ぐ。
`--expose` を付けると Caddy リバースプロキシにホスト名が自動登録され、`{name}--{branch}--{repo}.example.com` でアクセスできる。

---

## 基本ルール

1. **ソースコードや設定ファイルにポート番号をハードコードしない**
2. **`make serve` 等の Makefile ターゲットを経由して起動する**（portman を直接叩かない）
3. **`.env` ファイルは `.gitignore` に追加する**

---

## パターン別の実装方法

### パターン 1: 単一サーバー（`portman exec`）

ポートを `{}` プレースホルダーで渡してコマンドを実行する。

```makefile
.PHONY: serve
serve:
	portman exec --name api --expose -- go run main.go --port {}
```

- `--` の後にコマンドを書く
- `{}` がリースしたポート番号に自動置換される
- `--name api` はサービス識別名（省略時: `default`）
- `--expose` は Caddy へのホスト名登録が必要な場合に付ける（外部からアクセスしない開発用なら不要）
- プロセス終了後もリースは残る（再起動時に同じポートが返る）

#### 言語別の例

```makefile
# Go
serve:
	portman exec --name api --expose -- go run ./cmd/server --port {}

# Node.js (Express)
serve:
	portman exec --name api --expose -- node server.js --port {}

# Node.js (Vite)
serve-frontend:
	portman exec --name frontend --expose -- npx vite --port {}

# Python (Flask)
serve:
	portman exec --name api --expose -- flask run --port {}

# Python (uvicorn)
serve:
	portman exec --name api --expose -- uvicorn main:app --port {}

# Ruby (Rails)
serve:
	portman exec --name api --expose -- bin/rails server --port {}

# Rust (cargo)
serve:
	portman exec --name api --expose -- cargo run -- --port {}
```

#### `{}` の展開ルール

`{}` はコマンド引数のどこにでも使える。

```makefile
# 引数の一部として
serve:
	portman exec --name api --expose -- node server.js --listen 0.0.0.0:{}

# 環境変数として渡す（sh -c 経由）
serve:
	portman exec --name api --expose -- sh -c 'PORT={} node server.js'

# 複数箇所
serve:
	portman exec --name api --expose -- my-app --port {} --advertise-port {}
```

### パターン 2: 複数サービス（Makefile ターゲット分割）

サービスごとに `--name` を変えて Makefile ターゲットを分ける。

```makefile
.PHONY: serve serve-frontend serve-worker

serve:
	portman exec --name api --expose -- go run main.go --port {}

serve-frontend:
	portman exec --name frontend --expose -- npx vite --port {}

serve-worker:
	portman exec --name worker -- node worker.js --port {}
```

- 各サービスは独立したポートを取得する
- `--expose` はブラウザからアクセスするサービスに付ける
- バックグラウンドワーカーなど外部アクセス不要なら `--expose` は不要

### パターン 3: Docker Compose（`portman env`）

Docker Compose のポートマッピングに portman を使う。

#### Makefile

```makefile
.PHONY: up down

up: .env
	docker compose up

.env:
	portman env --expose --name api --name db --output .env

clean-env:
	rm -f .env
	portman release --name api
	portman release --name db
```

#### docker-compose.yml

```yaml
services:
  api:
    build: .
    ports:
      - "${API_PORT}:3000"   # 外部ポート:コンテナ内ポート
  db:
    image: postgres:16
    ports:
      - "${DB_PORT}:5432"
```

#### 環境変数名の生成ルール

`--name` の値が以下のルールで変換される:
- 小文字 → 大文字
- ハイフン(`-`) → アンダースコア(`_`)
- 末尾に `_PORT` を付与

| `--name` | 環境変数名 |
|---|---|
| `api` | `API_PORT` |
| `db` | `DB_PORT` |
| `my-service` | `MY_SERVICE_PORT` |
| `frontend` | `FRONTEND_PORT` |

#### .gitignore への追加

```
.env
```

### パターン 4: exec + Docker run

Docker コンテナを直接実行する場合。

```makefile
serve-nginx:
	portman exec --name nginx --expose -- docker run --rm -p {}:80 nginx:alpine
```

ホスト側ポート（`{}`）をコンテナ内ポート（`80`）にマッピングする。

### パターン 5: worktree を明示する（非 git 環境）

git リポジトリ外で使う場合は `--worktree` を明示する。

```makefile
serve:
	portman exec --name api --worktree manual-name --expose -- ./server --port {}
```

---

## `--expose` の判断基準

| 用途 | `--expose` |
|---|---|
| ブラウザからアクセスする API / Web | 付ける |
| フロントエンドの dev server | 付ける |
| ローカルだけで使う内部サービス | 付けない |
| バックグラウンドワーカー | 付けない |
| DB（ツールから直接つなぐ） | 付けない |

---

## ポート番号をアプリに渡す方法がない場合

アプリがポート番号のコマンドライン引数を受け付けない場合は、環境変数で渡す。

```makefile
serve:
	portman exec --name api --expose -- sh -c 'export PORT={} && node server.js'
```

または `portman env` でファイルに出力してから読み込む。

```makefile
serve: .env
	source .env && node server.js

.env:
	portman env --name api --expose --output .env
```

---

## よくある間違い

### NG: ポート番号をハードコード

```makefile
# NG
serve:
	go run main.go --port 8080
```

### NG: portman を使わずにポートを決める

```makefile
# NG
serve:
	PORT=8080 node server.js
```

### NG: `--` を忘れる

```makefile
# NG — portman のフラグとコマンドが混ざる
serve:
	portman exec --name api go run main.go --port {}
```

正しくは:

```makefile
serve:
	portman exec --name api -- go run main.go --port {}
```

### NG: .env を git にコミット

`.env` ファイルは portman が生成するものなので、必ず `.gitignore` に入れる。

---

## `make serve` の前に既存プロセスを停止する

`make serve` を実行する前に、同じワークツリーで起動中のプロセスがないか確認し、あれば停止すること。

```bash
# 現在のワークツリーのリース情報を JSON で取得
portman list -c --json

# PID が返ってきたらプロセスを kill してから起動
kill <PID>
make serve
```

`portman list -c --json` は以下のような JSON を返す:

```json
[{"name":"api","project":"org/repo","worktree":"main","port":8100,"hostname":"api--main--repo","expose":true,"status":"listening","pid":12345,"url":"https://api--main--repo.example.com"}]
```

- `pid` が存在し、`status` が `"listening"` なら、そのプロセスは起動中
- `kill <pid>` で停止してから `make serve` を実行する
- `pid` が省略されている（0）か `status` が `"not listening"` / `"stale"` ならそのまま `make serve` してよい

---

## リリース・解放

```bash
# 手動でリースを解放する
portman release --name api

# 全リースの状態を確認する
portman list

# 使われていないリースを回収する
portman gc
```

通常の開発ではリースの手動解放は不要。GC が自動で回収する。
