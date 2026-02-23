# TKT-Raft
この文章はすべてgemini-cli(Gemini 3)で書かれました
[English](README.md) | [日本語](README.ja.md)

## 概要 (Overview)
- Raftコンセンサスアルゴリズムの簡易実装
- Go言語で記述

## 機能 (Features)
- 実装済み
    - リーダー選出 (Leader election)
    - ログ複製 (Log replication)
    - 安全性 (Safety: term, commit index, etc.)

### Raftノードの概要
システムの核心は `raft.go` で定義されている `Raft` 構造体です。各ノードはスタンドアロンサーバーとして動作し、RPC を介してピアと通信します。
- **状態管理 (State Management):** ノードは標準的なRaftの状態 (`currentTerm`, `votedFor`, `log`, `commitIndex`, `lastApplied`) を保持します。
- **役割 (Roles):** ノードは `FOLLOWER`、`CANDIDATE`、`LEADER` の状態を遷移します。これは `consensus.go` のメインイベントループによって管理されます。

### 主要モジュールと関係性
*   **`raft.go`**: `Raft` 構造体を定義し、ノードを初期化します (`NewRaft`)。ログやRPC接続を含む中心的な状態を保持します。
*   **`consensus.go`**: メインの `Run()` ループを含みます。中心的なライフサイクルを処理します:
    *   **Follower:** ハートビートまたは選挙タイムアウトを待ちます (`doFollower`)。
    *   **Leader:** 定期的にハートビートを送信し (`doLeader`)、`commitIndex` を更新します。
    *   **Candidate:** 選挙プロセスを管理します (`startElection`)。
*   **`rpc.go`**: Raft RPCハンドラ (`RequestVote`, `AppendEntries`) と、これらのRPCをピアに送信するためのクライアント側ロジックを実装します。
*   **`replication.go`**: 内部クライアントからの新しいコマンドの取り込み (`handleClientRequest`) を処理し、ローカルログに追加します。
*   **`statemachine.go`**: アプリケーションの状態ロジック (`applyCommand`) を実装します。コマンド (`SET`, `GET`, `DELETE`) を解析し、インメモリの `map[string]string` を更新します。
*   **`conns.go`**: ネットワーク接続を管理します。TCPリスナー (`listenRPC`) を処理し、送信RPC接続を確立します (`dialRPCToPeer`)。
*   **`config.go`**: `cluster.conf` ファイルを解析して、ノードIDをIP:Portアドレスにマッピングします。
*   **`client.go`**: **内部負荷ジェネレータ** です。ランダムにコマンドを作成し、リーダーの処理チャネルに送信することでクライアントトラフィックをシミュレートします。

### ストレージとトランスポートの抽象化
*   **ストレージ:** **インメモリのみ。** Raftログ (`[]LogEntry`) とステートマシン (`map[string]string`) は、`Raft` 構造体内のメモリに完全に格納されます。ディスクへの永続化は実装されていません（WALやスナップショットはありません）。
*   **トランスポート:** **Go `net/rpc`.** 実装はTCP上のGo標準 `net/rpc` パッケージを使用します。ノードは `cluster.conf` で定義されたアドレスに基づいて相互にダイヤルします。

## ビルドと実行方法 (How to build and run)

### ビルド
標準のGoツールチェーンを使用してプロジェクトをビルドできます。
```bash
go build -o raft_server .
```

### 単一ノードの実行
ローカルで単一ノードを実行するには、有効な `cluster.conf`（ルートに提供されています）が必要であり、ノードIDを渡す必要があります。

```bash
# cluster.confの内容が以下であると仮定: [{"id": 1, "ip": "localhost", "port": 5000}]
./raft_server start --id 1 --conf cluster.conf
```

### Makefileによるクラスタ管理
付属の `makefile` は、`ssh` と `scp` を使用してクラスタのデプロイ、ビルド、ライフサイクル管理を自動化します。これは `cluster.conf` で定義されたノードで動作するように設計されています。

**前提条件:**
1.  **SSHアクセス:** `cluster.conf` に記載されているすべてのIP（`localhost` を含む）に対して、パスワードなしのSSHアクセスが必要です。Makefileがパスワード入力を求めずにリモートノードでコマンドを実行できるように、`ssh-agent` と `ssh-add` を使用することを推奨します。
2.  **設定:**
    *   **`cluster.conf`**: ノード（ID, IP, Port）を定義します。
    *   **`makefile`**: ファイルを開き、環境に合わせて `USER`（デフォルト: `tkt`）と `PROJECT_DIR`（デフォルト: `~/proj/raft`）を更新します。

**主要コマンド:**

*   **`make deploy`**
    `cluster.conf` ファイルを設定内の全ノードに配布します。

*   **`make send-bin`**
    バイナリをローカルでクロスコンパイル（Linux/AMD64）し、すべてのリモートノードに転送します。コードを更新する場合はこれが推奨される方法です。

*   **`make build`**
    リモートノード*上で* `go build` コマンドをトリガーします。リモートノードにGoがインストールされており、リモートでのコンパイルを好む場合に使用してください。

*   **`make start`**
    すべてのノードでRaftサーバーをバックグラウンドで開始します。ログは `logs/node_<ID>.ans` にリダイレクトされます。

*   **`make kill`**
    すべてのノード上のRaftサーバープロセスを停止します。

*   **`make clean`**
    ノードからバイナリとログファイルを削除します。

**ベンチマークとメトリクス:**

*   **`make benchmark`**
    さまざまなワークロードとバッチサイズでスループットとレイテンシを測定する包括的なベンチマークスイートを実行します。

*   **`make get-metrics`**
    `bench-disk-remote` と `bench-net-remote` を実行し、クラスタ環境の基盤となるディスクとネットワークのパフォーマンスを測定します。

**ワークフロー例:**

1.  **設定:** `cluster.conf` と `makefile` の変数を編集します。
2.  **設定のデプロイ:** `make deploy`
3.  **コードの更新:** `make send-bin`
4.  **クラスタの開始:** `make start`
5.  **監視:** ノード上のログを確認します（例: `tail -f logs/node_1.ans`）。
6.  **停止:** `make kill`

**手動開始（デバッグ）:**
MakefileやSSHを使用したくない場合は、別々のターミナルでノードを手動実行できます。

```bash
# ターミナル 1
./raft_server start --id 1 --conf cluster.conf

# ターミナル 2
./raft_server start --id 2 --conf cluster.conf

# ターミナル 3
./raft_server start --id 3 --conf cluster.conf
```

### 最小サンプルコード
このプロジェクトはライブラリではなくスタンドアロンバイナリとして設計されていますが、以下は `main` 関数がどのようにノードを起動するかの基本です（`init.go` に基づく）:

```go
package main

func main() {
    // 1. 設定パスとノードIDを定義
    nodeID := 1
    confPath := "cluster.conf"

    // 2. Raftインスタンスを初期化
    raftNode := NewRaft(nodeID, confPath)

    // 3. メインイベントループを開始（永久にブロック）
    raftNode.Run()
}
```

## API / 使用法

### エントリポイント
エントリポイントは `init.go` で定義されたCLIコマンドで、`urfave/cli` を利用しています。
*   コマンド: `start`
*   フラグ: `--id <int>`, `--conf <string>`

### アプリケーションインターフェース
*   **外部APIなし:** 外部クライアントがコマンドを送信するためのHTTPやgRPCインターフェースはありません。
*   **内部クライアント:** 「使用法」は現在 `client.go` によってシミュレートされています。これは `Raft` プロセス内でゴルーチンを実行します。ランダムな `SET`, `GET`, `DELETE` コマンドを生成し、`ClientCh` に送信します。
*   **ステートマシンフック:** 実際のアプリケーション用にこれを変更する場合は、`statemachine.go` を編集します:
    ```go
    // statemachine.go 内
    func (r *Raft) applyCommand(command []byte) {
        // ここでコマンドを解析
        // アプリケーションの状態を更新
    }
    ```

## テスト (Testing)

### 戦略
*   **シミュレーション/負荷テスト:** プロジェクトはランダムなトラフィック (`randomOperation`, `createRandomCommand`) を生成するために内部の `client.go` に依存しています。これはクラスタ実行時の継続的な統合テストとして機能します。
*   **ユニットテスト:** トップレベルディレクトリに標準的なGoユニットテスト（`_test.go` ファイル）はありません。
*   **シナリオ:** 実装は暗黙的に **リーダー選出**（`consensus.go` のタイムアウト経由）と **複製**（内部クライアントがログをプッシュすることによる）をテストしています。`makefile` は、実際のネットワーク分散をテストするために複数のホストにデプロイするワークフローを提案しています。

## 制限事項 (Limitations)

1.  **固定設定:** クラスタメンバーシップは静的であり、起動時に `cluster.conf` で定義されます。動的なメンバーシップ変更はサポートされていません。
2.  **スナップショットなし:** ログは無制限に増加します。ログを圧縮したりスナップショットを作成したりするメカニズムはありません。
3.  **内部専用クライアント:** 外部アプリケーションはクラスタと対話できません。`client` ロジックはサーバーバイナリ内にハードコードされています。
4.  **基本的なエラー処理:** ネットワークエラーはログに記録されますが、複雑な回復シナリオやバックオフ戦略は最小限である可能性があります。
5.  **TODO（推測）:**
    *   クライアント対話用の外部API（HTTP/gRPC）を追加する。
    *   ログ圧縮/スナップショットを実装する。
    *   正式なユニットテストを追加する。
