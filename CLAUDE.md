# CLAUDE.md - portman 開発ガイド

## プロジェクト概要

portman は開発環境向けDHCPライクなポート管理CLIツール。
Go + cobra + SQLite で実装する。

## 実装ロードマップ

Issue #14 に依存関係を含む実装順序を記載。Phase順に進めること。

## 実装の進め方

- Issue #14 のロードマップに従い、Phase順に実装すること
- 各Issueの要件を必ず読み、要件に従って実装すること
- 不明点があれば docs/DESIGN.md を参照すること
- 各パッケージ実装後に `go test ./...` を実行して全テストが通ることを確認してから次のIssueに進むこと
- `go vet ./...` もパスすること
- コミットはIssue単位で行い、コミットメッセージに `refs #N` を含めること（例: `feat: internal/config 実装 refs #2`）
- Phase内のIssueは番号順に実装すること

### Phase別の指示テンプレート

Phase 1:
```
GitHub Issue #14 の実装ロードマップを読んで、Phase 1 の3つのIssue (#2, #4, #7) を順番に実装してください。
各Issueの要件を読み、CLAUDE.mdの規約に従うこと。
各パッケージ実装後に go test ./... が通ることを確認してから次に進むこと。
コミットはIssue単位で refs #N を含めること。
```

Phase 2:
```
Issue #14 のPhase 2 (#3, #5, #6, #8) を順番に実装してください。
Phase 1で作ったパッケージを使うこと。各Issue完了ごとにテストを通すこと。
コミットはIssue単位で refs #N を含めること。
```

Phase 3:
```
Issue #14 のPhase 3 (#9, #10, #11) を実装してください。
internal/以下のパッケージを組み合わせてcobraコマンドを実装すること。
コミットはIssue単位で refs #N を含めること。
```

Phase 4:
```
Issue #14 のPhase 4 (#13, #12) を実装してください。
#13: .github/workflows/ にCI/CDを構築、.goreleaser.yml を作成。
#12: deploy/ansible/roles/portman/ にAnsible roleを実装。
コミットはIssue単位で refs #N を含めること。
```

## 技術スタック

- **言語**: Go 1.22+
- **CLI フレームワーク**: cobra
- **DB**: SQLite — **`modernc.org/sqlite` を使うこと**（CGO不要、クロスコンパイル対応。`mattn/go-sqlite3` は使わない）
- **設定**: TOML (`github.com/BurntSushi/toml`)
- **JSON**: 標準ライブラリ `encoding/json`

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
portman lease [--name <service>] [--expose] [--worktree <n>]
portman release [--name <service>] [--worktree <n>]
portman exec [--name <service>] [--expose] [--worktree <n>] -- <command> {}
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

### env動作
- 複数 `--name` を受け取り、それぞれにポートをリース
- `NAME_PORT=XXXX` 形式で出力（name: 小文字→大文字、ハイフン→アンダースコア、末尾`_PORT`）
- `--output <path>` でファイル書き出し、なければstdout

### Caddy連携
- Admin API (`localhost:2019`) で動的にルート追加・削除
- `--expose` のもののみCaddyに登録。内部ポートはCaddyに触らない
- Caddy APIが落ちている場合はリースだけ記録し、次回sync時に回復
- Caddy操作は `internal/proxy/caddy.go` に集約すること（将来の差し替え考慮）
- ルートの `@id` は `portman-{hostname}` 形式で付与（削除・sync時に使用）

### ダッシュボード
- `dashboard.auto_update = true` の場合、expose/release/sync/gc後に静的HTML再生成
- Caddy自身が静的ファイルを配信

### エラー処理方針
- Caddy APIエラー: リースだけ記録、sync時に回復（graceful degradation）
- ホスト名衝突: エラーを出して --name での区別を促す
- 非gitディレクトリで --worktree なし: エラー

## Makefile統合パターン

各プロジェクトのMakefileにportmanを組み込む。Claude Codeは `make serve` を叩くだけでよい。

```makefile
.PHONY: serve
serve:
	portman exec --name api --expose -- go run main.go --port {}

.PHONY: serve-frontend
serve-frontend:
	portman exec --name frontend --expose -- npx vite --port {}
```

## 各プロジェクトのCLAUDE.mdに書くべき内容

```markdown
## サーバー起動
テストサーバーは `make serve` で起動すること。ポート番号を直接指定しない。

## Docker Compose
portman env --expose --name api --name db --output .env
docker compose up

## やってはいけないこと
- ソースコードやdocker-compose.ymlにポート番号をハードコードしない
- .envファイルをgit commitしない（.gitignoreに追加すること）
```

## ビルド・テスト

```bash
go build -o portman .
go test ./...
go vet ./...
```

## リリース

GoReleaserでクロスコンパイル（linux/amd64, linux/arm64）。
GitHub Releasesにバイナリをアップロード。
Ansible roleはGitHub ReleasesのURLからバイナリを取得する。

## コーディング規約

- Go標準のフォーマッティング (`gofmt`)
- エラーは適切にラップして返す (`fmt.Errorf("...: %w", err)`)
- ログ出力は stderr に（stdout はコマンド結果のみ）
- テストは `_test.go` に記述、主要ロジックはユニットテスト必須
