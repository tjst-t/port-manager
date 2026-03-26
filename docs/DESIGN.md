# portman 設計ドキュメント

## 1. 背景・課題

複数プロジェクト（および同一プロジェクトの複数 worktree）で開発を行う際、以下の問題が発生する：

1. **ポート衝突** — 各プロジェクトが好き勝手にポートを使い、衝突する
2. **プロセス殺害** — 衝突時に既存プロセスを止めてポートを奪ってしまう
3. **常時稼働デーモンの破壊** — 最悪のケースで Caddy 等のシステムデーモンを停止してしまう
4. **アクセスの面倒さ** — `localhost:????` のポート番号を覚えておく必要がある

## 2. 基本コンセプト

開発環境向けDHCPライクなポート管理CLI + Caddyリバースプロキシ自動設定ツール。

| DHCPの概念 | portman |
|---|---|
| クライアントID (MACアドレス) | worktree path + name |
| リース (IPアドレス) | ポート番号 |
| 静的予約 | `portman reserve` (保護ポート) |
| 常設割当 | `services.json` の permanent |
| スコープ (サブネット) | ポートレンジ (config.toml) |
| リース期限 | stale検出 + TTL |

## 3. 識別キー

```
{project}:{worktree}:{name}
```

- **project**: `git remote get-url origin` から導出（例: `tjst-t/palmux`）
  - SSH: `git@github.com:tjst-t/palmux.git` → `tjst-t/palmux`
  - HTTPS: `https://github.com/tjst-t/palmux.git` → `tjst-t/palmux`
- **worktree**: `git branch --show-current` で取得
  - 非gitディレクトリでは `--worktree` 引数が必須（なければエラー）
  - detached HEADの場合も `--worktree` 必須
- **name**: `--name` オプションで指定（省略時: `default`）

## 4. ホスト名生成

```
{name}--{worktree}--{repo}.example.com
```

- 区切りは `--`（ブランチ名のハイフンと区別するため）
- projectのうちrepo名のみ使用（org部分は使わない）
  - 理由: スラッシュがDNSホスト名で不正。org名を含めると衝突回避のエスケープが複雑化
  - 内部識別キーではフルのproject名を保持し一意性を保証
  - ホスト名衝突時はleaseでエラーを出して `--name` での区別を促す
- DNS不正文字（スラッシュ、アンダースコア等）はハイフンに置換
- ワイルドカードDNS: `*.example.com` → サーバIPへのAレコード
- TLS: 利用者の選択（Let's Encrypt DNS-01チャレンジ推奨）

name=defaultの場合:
```
default--feature-xyz--palmux.example.com
```

## 5. ポート割当

### 5.1 レンジ

`config.toml` の `[ports]` セクションで定義。デフォルト: 8200-8999。

### 5.2 割当ロジック

`portman lease` 時:

1. reserved（保護ポート）、permanent（常設サービス）のポートは割当対象外
2. 既存リース(active)あり → 同じポート返す、`last_used` 更新
3. 既存リース(stale)あり → 同じポート返す、activeに戻す
4. リースなし → range内から空きポート割当（**staleは新規割当対象外**）
5. `--expose` 時はCaddy API登録（失敗時はリースのみ記録、sync時に回復）
6. `dashboard.auto_update = true` ならダッシュボード再生成
7. 前回GCから1時間以上経過していれば軽量GCを実行

### 5.3 永続性

一度割り当てたポートは永続的に保持する（再起動しても同じ番号）。
明示的な `portman release` またはGCによってのみ解放される。

## 6. データ管理

### 6.1 静的設定（JSON）— Ansibleで管理

```json
// /etc/portman/services.json
{
  "reserved": [
    { "port": 80, "description": "caddy http" },
    { "port": 443, "description": "caddy https" },
    { "port": 2019, "description": "caddy admin api" }
  ],
  "permanent": [
    {
      "name": "grafana",
      "port": 3000,
      "expose": true
    },
    {
      "name": "prometheus",
      "port": 9090,
      "expose": false
    }
  ]
}
```

- **reserved**: portmanが触ってはいけないポート
- **permanent**: portmanが管理する常設サービス（GC対象外）

### 6.2 アプリ設定（TOML）— Ansibleで管理

```toml
# /etc/portman/config.toml

[general]
db_path = "/var/lib/portman/portman.db"

[ports]
range_start = 8200
range_end = 8999
stale_ttl_hours = 24

[proxy]
type = "caddy"
caddy_api = "http://localhost:2019"
domain_suffix = "example.com"
host_pattern = "{name}--{worktree}--{repo}"

[dashboard]
enabled = true
host = "portal.example.com"
output_dir = "/var/lib/portman/portal"
auto_update = true
```

### 6.3 動的データ（SQLite）— portmanが管理

```sql
CREATE TABLE leases (
    id INTEGER PRIMARY KEY,
    port INTEGER UNIQUE NOT NULL,
    project TEXT NOT NULL,         -- 例: tjst-t/palmux
    worktree TEXT NOT NULL,        -- 例: local-service
    worktree_path TEXT NOT NULL,   -- 例: /home/user/repos/palmux-local-service
    repo TEXT NOT NULL,            -- 例: palmux
    name TEXT NOT NULL,            -- 例: api
    hostname TEXT UNIQUE NOT NULL, -- 例: api--local-service--palmux
    expose BOOLEAN DEFAULT FALSE,
    state TEXT DEFAULT 'active',   -- active / stale
    stale_since TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_used TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(project, worktree, name)
);

CREATE TABLE gc_state (
    key TEXT PRIMARY KEY,
    value TEXT
);
-- last_gc_at を記録（軽量GCの判定用）
```

## 7. GC（ガベージコレクション）

### 7.1 `portman gc` の動作（3段階）

1. **worktree存在チェック**（最優先）
   - `worktree_path` のディレクトリが存在しない → 即解放 + Caddyルート削除

2. **listen状態チェック**
   - activeリース → `ss -tlnp` 等でポートlisten確認 → listenなし → `state='stale'`, `stale_since=now()`

3. **TTL超過チェック**
   - staleリース → `stale_ttl_hours` 超過 → 解放 + Caddyルート削除

### 7.2 軽量GC

`portman lease` 実行時、前回GCから1時間以上経過していれば自動実行。

### 7.3 定期GC

systemd timerで1時間ごとに `portman gc` を実行。

## 8. CLI インターフェース

### 8.1 コマンド一覧

```bash
# ポート取得
portman lease [--name <service>] [--expose] [--worktree <name>]
# → stdout にポート番号を出力

# ラップ実行（{}をポート番号に置換）
portman exec [--name <service>] [--expose] [--worktree <name>] -- <command> {}
# → SIGTERM/SIGINT を子プロセスに伝搬
# → 子プロセス終了後の自動releaseはしない

# 解放
portman release [--name <service>] [--worktree <name>]

# Docker Compose連携
portman env [--name <service>[:expose]]... [--expose] [--output <path>]
# → NAME_PORT=XXXX 形式で出力
# → --name dashboard:expose のように :expose を付けるとそのサービスのみCaddy登録
# → --expose フラグは全サービスに一括適用

# 一覧
portman list

# GC
portman gc

# リース外ポート使用の検出
portman audit

# Caddyへの全リース再登録 + ダッシュボード再生成
portman sync

# 保護ポート登録（CLIからの追加用）
portman reserve <port> [--description <desc>]
```

### 8.2 list出力例

```
PROJECT              WORKTREE         NAME       PORT   EXPOSE  STATUS
tjst-t/palmux        local-service    api        8234   yes     ● listening
tjst-t/palmux        local-service    frontend   8235   yes     ● listening
tjst-t/palmux        feature-xyz      api        8236   yes     ○ stale
tjst-t/other-repo    main             default    8237   no      ● listening

PERMANENT:
NAME                 PORT   EXPOSE
grafana              3000   yes
prometheus           9090   no

RESERVED:
PORT   DESCRIPTION
80     caddy http
443    caddy https
2019   caddy admin api
```

## 9. Caddy連携

- Admin API (`localhost:2019`) で動的にルート追加・削除
- `--expose` のもののみ登録。内部ポートはCaddyに触らない
- Caddy APIが落ちている場合: リースだけ記録し警告を出す（graceful degradation）
- Caddy再起動時: systemd連携で `portman sync` が自動実行し全exposeリースを再登録
- Caddy操作コードは `internal/proxy/caddy.go` に集約

### 9.1 Caddyへのルート追加例

```bash
curl localhost:2019/config/apps/http/servers/srv0/routes -X POST \
  -H "Content-Type: application/json" \
  -d '{"match":[{"host":["api--local-service--palmux.example.com"]}],"handle":[{"handler":"reverse_proxy","upstreams":[{"dial":"localhost:8234"}]}]}'
```

## 10. ダッシュボード

- `dashboard.auto_update = true` の場合、expose/release/register/sync/gc後に静的HTML再生成
- `/var/lib/portman/portal/index.html` にリース一覧のリンク集を生成
- Caddy自身が `portal.example.com` で静的ファイルを配信
- active/stale/permanent/exposeの状態を表示

## 11. Makefile統合

各プロジェクトのMakefileにportmanを組み込む:

```makefile
.PHONY: serve
serve:
	portman exec --name api --expose -- go run main.go --port {}

.PHONY: serve-frontend
serve-frontend:
	portman exec --name frontend --expose -- npx vite --port {}
```

各プロジェクトのCLAUDE.md:

```markdown
## サーバー起動
テストサーバーは `make serve` で起動すること。ポート番号を直接指定しない。
```

## 12. Docker Compose連携

```yaml
# docker-compose.yml
services:
  api:
    ports:
      - "${API_PORT}:3000"
  db:
    ports:
      - "${DB_PORT}:5432"
```

```bash
# 全サービスをexpose
portman env --expose --name api --name db --output .env
# 特定のサービスだけexpose
portman env --name api:expose --name db --output .env
docker compose up
```

`.env` ファイルは `.gitignore` に追加すること。

## 13. systemd連携

### portman-sync.service

Caddy再起動時に全リースを再登録:

```ini
[Unit]
Description=Sync portman leases to Caddy
After=caddy.service
BindsTo=caddy.service

[Service]
Type=oneshot
ExecStart=/usr/local/bin/portman sync

[Install]
WantedBy=caddy.service
```

### portman-gc.timer

1時間ごとにGC実行:

```ini
# portman-gc.service
[Unit]
Description=portman garbage collection

[Service]
Type=oneshot
ExecStart=/usr/local/bin/portman gc
```

```ini
# portman-gc.timer
[Unit]
Description=Run portman gc periodically

[Timer]
OnCalendar=hourly
Persistent=true

[Install]
WantedBy=timers.target
```

## 14. Ansible Role

リポジトリの `deploy/ansible/roles/portman/` に同梱。
git submoduleで参照して利用:

```bash
git submodule add https://github.com/tjst-t/port-manager.git vendor/port-manager
```

```ini
# ansible.cfg
[defaults]
roles_path = vendor/port-manager/deploy/ansible/roles
```

## 15. ポート使用の強制について

ポート使用をiptables/BPF等で技術的に強制する仕組みは実装しない。
DHCPの世界で固定IPを付けられるのと同様、portmanを通さないポート使用は許容する。
`portman audit` でリース外のポート使用を検出・警告するにとどめる。
