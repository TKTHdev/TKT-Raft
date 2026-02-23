# TKT-Raft

[English](README.md) | [日本語](README.ja.md)

## 概要

- Raftコンセンサスアルゴリズムの簡易実装
- Go言語で記述
- `package raft` のGoライブラリとして利用可能。`StateMachine` インターフェースで状態管理をカスタマイズできる

## 機能

- リーダー選出 (Leader election)
- ログ複製 (Log replication)
- 安全性 (Safety: term, commit index など)
- プラガブルなステートマシン — `Apply`/`Query` を自前で実装して差し込める
- 組み込みKVストア (`KVStore`) — SET / GET / DELETE ワークロード用
- 永続化ストレージ (ログ用WAL、term/votedFor 用バイナリファイル)
- クォーラムリードバッチングによる読み取りパス最適化

---

### ディレクトリ構成

```
raft/                  ← package raft  (ライブラリ)
  raft.go              ← Config struct、New()、Raft struct
  consensus.go         ← Run()、選挙、複製ループ
  rpc.go               ← RPCの型とハンドラ
  handle_client.go     ← リクエストバッチング、Response型
  statemachine.go      ← StateMachine インターフェース + KVStore
  storage.go           ← WAL / 状態の永続化
  conns.go             ← TCP RPCリスナー & ダイアラー
  config.go            ← cluster.conf パーサー (ParseConfig)
  logger.go            ← デバッグロギング
  cmd/                 ← package main  (バイナリ)
    main.go            ← CLIエントリポイント (urfave/cli)
    client.go          ← ベンチマーククライアント
```

---

### 主要モジュール

| ファイル | 役割 |
|---|---|
| `raft.go` | `Config`、`New()`、`Raft` struct |
| `consensus.go` | `Run()`、`doFollower`、`doLeader`、`startElection`、`processReadBatch` |
| `rpc.go` | `AppendEntries`、`RequestVote`、`Execute`、`Read` RPCハンドラ & 送信 |
| `handle_client.go` | `handleClientRequest` — 書き込みをログへ、読み取りをクォーラムパスへバッチ処理 |
| `statemachine.go` | `StateMachine` インターフェース、`KVStore` 実装、`applyCommand` |
| `storage.go` | ログエントリ用バイナリWAL、term/votedFor 用バイナリファイル |
| `conns.go` | `listenRPC`、`dialRPCToPeer` |
| `config.go` | `ParseConfig` — `cluster.conf` のJSON読み込み |

### StateMachine インターフェース

```go
type StateMachine interface {
    Apply(cmd []byte) []byte  // ログエントリがコミットされた後に呼ばれる（書き込みパス）
    Query(cmd []byte) []byte  // クォーラム確認後に呼ばれる（読み取りパス、ログに記録しない）
}
```

`GET` で始まるコマンドはクォーラムリードパス（`Query`）にルーティングされ、それ以外はRaftログ経由（`Apply`）で処理される。

---

## ライブラリとして使う

```go
import "raft"

// 組み込みKVストアを使う場合
node := raft.New(raft.Config{
    ID:       1,
    ConfPath: "cluster.conf",
}, raft.NewKVStore())
go node.Run()
```

カスタムステートマシンの例:

```go
type MembershipSM struct{ members map[int]string }

func (m *MembershipSM) Apply(cmd []byte) []byte {
    // ADD_MEMBER / REMOVE_MEMBER を処理
    return nil
}
func (m *MembershipSM) Query(cmd []byte) []byte {
    // 現在のメンバーリストを返す
    return nil
}

node := raft.New(raft.Config{
    ID:       myID,
    ConfPath: "raft.conf",
}, &MembershipSM{members: make(map[int]string)})
go node.Run()
```

---

## ビルドと実行

### バイナリのビルド

```bash
go build -o raft_server ./cmd
```

### 単一ノードの実行

```bash
./raft_server start --id 1 --conf cluster.conf
```

### 利用可能なフラグ

| フラグ | デフォルト | 説明 |
|---|---|---|
| `--id` | (必須) | ノードID |
| `--conf` | `cluster.conf` | 設定ファイルのパス |
| `--write-batch-size` | `128` | 1回のfsyncにまとめる最大ログエントリ数 |
| `--read-batch-size` | `128` | 1回のクォーラムラウンドにまとめる最大読み取り数 |
| `--debug` | `false` | カラー付きデバッグログを有効にする |
| `--async-log` | `false` | 書き込みごとのfsyncをスキップ（高速だが耐久性が下がる） |

---

### Makefileによるクラスタ管理

`makefile` はSSH越しのデプロイを自動化する。

**前提条件:**
1. `cluster.conf` に記載されたすべてのIPへのパスワードなしSSHアクセス。
2. `makefile` 冒頭の `USER` と `PROJECT_DIR` を環境に合わせて更新する。

**主要コマンド:**

| コマンド | 説明 |
|---|---|
| `make deploy` | `cluster.conf` を全ノードに配布 |
| `make send-bin` | ローカルでクロスコンパイル（Linux/AMD64）して全ノードにバイナリを送信 |
| `make build` | リモートノード上でビルド（Goのインストールが必要） |
| `make start` | 全ノードでRaftサーバーをバックグラウンド起動（ログ → `logs/node_<ID>.ans`） |
| `make kill` | 全ノードのRaftサーバープロセスを停止 |
| `make clean` | ノードからバイナリとログを削除 |
| `make benchmark` | ワークロード × バッチサイズ × ワーカー数でスイープし、CSV出力 |
| `make get-metrics` | クラスタノードのディスク・ネットワークレイテンシを計測 |

**ワークフロー例:**

```bash
make deploy       # cluster.confを配布
make send-bin     # バイナリを送信
make start        # 全ノードを起動
make benchmark TYPE=ycsb-a WORKERS="1 4 16" READ_BATCH="1 32" WRITE_BATCH="1 32"
make kill
```

**手動起動（デバッグ時）:**

```bash
./raft_server start --id 1 --conf cluster.conf  # ターミナル1
./raft_server start --id 2 --conf cluster.conf  # ターミナル2
./raft_server start --id 3 --conf cluster.conf  # ターミナル3
```

---

## 制限事項

1. **静的なクラスタ構成** — クラスタサイズは起動時に `cluster.conf` で固定される。動的なメンバーシップ変更は未対応。
2. **ログ圧縮なし** — ログは無限に増加する。スナップショット機能は未実装。
3. **KV読み取りルーティングはプレフィックスベース** — `GET` で始まるコマンドがクォーラムリードパスに回る。カスタムステートマシンで `Query` を使いたい場合も同じ命名規則に従う必要がある。
4. **外部APIなし** — 外部との通信はGo RPC（`net/rpc`）のみ。
