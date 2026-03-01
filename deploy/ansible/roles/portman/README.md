# Ansible Role: portman

portman のインストール・設定・systemd連携をセットアップするAnsible role。

## 使い方

### git submodule で参照

```bash
git submodule add https://github.com/tjst-t/port-manager.git vendor/port-manager
```

```ini
# ansible.cfg
[defaults]
roles_path = vendor/port-manager/deploy/ansible/roles
```

```yaml
# playbook.yml
- hosts: devservers
  roles:
    - portman
```

## Role Variables

| 変数 | デフォルト | 説明 |
|---|---|---|
| `portman_binary_url` | (未定義) | ビルド済みバイナリのURL。未定義ならソースからビルド |
| `portman_port_range_start` | `8200` | ポートレンジ開始 |
| `portman_port_range_end` | `8999` | ポートレンジ終了 |
| `portman_stale_ttl_hours` | `24` | staleリースのTTL（時間） |
| `portman_domain_suffix` | `cdev.vm.tjstkm.net` | ホスト名サフィックス |
| `portman_dashboard_enabled` | `true` | ダッシュボード有効化 |
| `portman_services` | (reserved: 80,443,2019) | 保護ポート・常設サービス定義 |
| `portman_gc_interval` | `hourly` | GC実行間隔 |

## 前提条件

- Caddy がインストール・起動済みであること
- Go がインストール済みであること（ソースビルドの場合）
- ワイルドカードDNSが設定済みであること
