# CLAUDE.md - portman 開発ガイド

## プロジェクト概要

portman は開発環境向けDHCPライクなポート管理CLIツール。
Go + cobra + SQLite で実装する。

## 技術スタック

- **言語**: Go 1.22+
- **CLI フレームワーク**: cobra
- **DB**: SQLite (github.com/mattn/go-sqlite3 または modernc.org/sqlite)
- **設定**: TOML (github.com/BurntSushi/toml)
- **JSON**: 標準ライブラリ encoding/json

## ディレクトリ構成

```
portman/
├── cmd/              -- cobra CLIコマンド定義
│   ├── root.go
│   ├── lease.go
│   ├── release.go
│   ├── exec.go
│   ├── env.go
│   ├── list.go
│   ├── gc.go
│   ├── audit.go
│   ├── sync.go
│   └── reserve.go
├── internal/
│   ├── config/       -- TOML + JSON設定読み込み
│   ├── db/           -- SQLiteリース管理
│   ├── proxy/        -- Caddy API操作（1モジュールに集約）
│   ├── port/         -- ポート割当・GC・auditロジック
│   ├── dashboard/    -- 静的HTML生成
│   ├── exec/         -- 子プロセス実行・シグナル伝搬
│   └── git/          -- git情報取得（project, worktree）
├── deploy/
│   └── ansible/
│       └── roles/
│           └── portman/
├── configs/          -- 設定ファイルのサンプル
├── docs/             -- 設計ドキュメント
├── main.go
├── go.mod
└── go.sum
```

## CLIコマンド一覧

```bash
portman lease [--name <service>] [--expose] [--worktree <name>]
portman release [--name <service>] [--worktree <name>]
portman exec [--name <service>] [--expose] [--worktree <name>] -- <command> {}
portman env [--name <service>]... [--expose] [--output <path>]
portman list
portman gc
portman audit
portman sync
portman reserve <port> [--description <desc>]
```

## 重要な設計判断

### 識別キー
```
{project}:{worktree}:{name}
```
- project: `git remote get-url origin` から導出（例: `tjst-t/palmux`）
- worktree: `git branch --show-current`（非gitでは--worktree必須、なければエラー）
- name: `--name`で指定（省略時: `default`）

### ホスト名生成
```
{name}--{worktree}--{repo}.cdev.vm.tjstkm.net
```
- 区切りは `--`（ブランチ名のハイフンと区別するため）
- projectのorg部分は使わない。repo名のみ使用
- スラッシュ等のDNS不正文字はハイフンに置換
- ホスト名衝突時はleaseでエラーを出す

### データ管理の二層構造
- **静的設定（JSON）**: 常設サービス・保護ポート → Ansibleで冪等に管理
- **動的データ（SQLite）**: リース情報 → portmanが管理
- **アプリ設定（TOML）**: ポートレンジ・Caddy接続先等 → Ansibleで管理

### GC（3種類の回収）
1. **worktree存在チェック**: worktree_pathが存在しない → 即解放
2. **listen状態チェック**: activeリース → ポートlistenなし → staleにマーク
3. **TTL超過チェック**: staleリース → stale_ttl_hours超過 → 解放

`portman lease` 実行時にも軽量GC（前回GCから1時間以上経過していれば実行）。

### stale状態のポート
- stale状態のポートは新規割当対象外
- 同じキーからleaseすればactiveに復帰（同じポートを返す）

### exec動作
- `{}` をリースしたポート番号に置換してコマンド実行
- SIGTERM/SIGINTを子プロセスに伝搬すること
- 子プロセス終了後の自動releaseはしない（再起動時に同じポートを返すため）

### Caddy連携
- Admin API (`localhost:2019`) で動的にルート追加・削除
- `--expose` のもののみCaddyに登録。内部ポートはCaddyに触らない
- Caddy APIが落ちている場合はリースだけ記録し、次回sync時に回復
- Caddy操作は `internal/proxy/caddy.go` に集約すること（将来の差し替え考慮）

### ダッシュボード
- `dashboard.auto_update = true` の場合、expose/release/sync/gc後に静的HTML再生成
- Caddy自身が静的ファイルを配信

### エラー処理方針
- Caddy APIエラー: リースだけ記録、sync時に回復（graceful degradation）
- ホスト名衝突: エラーを出して --name での区別を促す
- 非gitディレクトリで --worktree なし: エラー

## ビルド・テスト

```bash
go build -o portman .
go test ./...
```

## コーディング規約

- Go標準のフォーマッティング (`gofmt`)
- エラーは適切にラップして返す (`fmt.Errorf("...: %w", err)`)
- ログ出力は stderr に（stdout はコマンド結果のみ）
- テストは `_test.go` に記述、主要ロジックはユニットテスト必須
