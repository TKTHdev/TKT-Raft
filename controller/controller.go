package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

// 設定ファイルの構造体
type Node struct {
	ID   int    `json:"id"`
	IP   string `json:"ip"`
	Port int    `json:"port"`
}

// 定数（環境に合わせて変更してください）
const (
	User       = "tkt"             // SSHユーザー名
	ProjectDir = "~/study/raft"    // リモートのプロジェクトディレクトリ
	BinaryName = "raft_server"     // 生成されるバイナリ名
	ConfigFile = "../cluster.conf" // 設定ファイル名
)

func main() {
	// コマンドライン引数のパース
	deployCmd := flag.NewFlagSet("deploy", flag.ExitOnError)
	buildCmd := flag.NewFlagSet("build", flag.ExitOnError)
	startCmd := flag.NewFlagSet("start", flag.ExitOnError)
	killCmd := flag.NewFlagSet("kill", flag.ExitOnError) // ★追加: killコマンド用

	if len(os.Args) < 2 {
		// ★利用方法の表示を更新
		fmt.Println("Usage: go run ops_tool.go [deploy|build|start|kill]")
		os.Exit(1)
	}

	nodes := loadConfig(ConfigFile)

	switch os.Args[1] {
	case "deploy":
		deployCmd.Parse(os.Args[2:])
		runParallel(nodes, distributeConfig)
	case "build":
		buildCmd.Parse(os.Args[2:])
		runParallel(nodes, buildProject)
	case "start":
		startCmd.Parse(os.Args[2:])
		runParallel(nodes, startRaft)
	case "kill": // ★追加: killコマンドの処理
		killCmd.Parse(os.Args[2:])
		runParallel(nodes, killRaftProcess)
	default:
		fmt.Println("Unknown command. Use: deploy, build, start, kill") // ★メッセージを更新
		os.Exit(1)
	}
}

// 設定ファイルの読み込み
func loadConfig(path string) []Node {
	file, err := ioutil.ReadFile(path)
	if err != nil {
		log.Fatalf("Failed to read config: %v", err)
	}
	var nodes []Node
	if err := json.Unmarshal(file, &nodes); err != nil {
		log.Fatalf("Failed to parse json: %v", err)
	}
	return nodes
}

// 並列実行用のヘルパー関数
func runParallel(nodes []Node, job func(Node)) {
	var wg sync.WaitGroup
	for _, node := range nodes {
		wg.Add(1)
		go func(n Node) {
			defer wg.Done()
			job(n)
		}(node)
	}
	wg.Wait()
	fmt.Println("All tasks completed.")
}

// 1. 設定ファイルの配布 (SCP)
func distributeConfig(node Node) {
	fmt.Printf("[%s] Distributing config...\n", node.IP)

	// scp cluster.conf user@ip:~/path/cluster.conf
	remotePath := filepath.Join(ProjectDir, ConfigFile)
	cmd := exec.Command("scp", ConfigFile, fmt.Sprintf("%s@%s:%s", User, node.IP, remotePath))

	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Printf("[%s] SCP Error: %v\nOutput: %s\n", node.IP, err, string(out))
	} else {
		fmt.Printf("[%s] Config distributed.\n", node.IP)
	}
}

// 2. リモートでのビルド (SSH)
func buildProject(node Node) {
	fmt.Printf("[%s] Building project...\n", node.IP)

	// ssh user@ip "cd ~/path && go build -o raft_server"
	// 依存関係解決のため go mod tidy も念のため実行
	buildCmd := fmt.Sprintf("cd %s && go mod tidy && go build -o %s", ProjectDir, BinaryName)
	cmd := exec.Command("ssh", fmt.Sprintf("%s@%s", User, node.IP), buildCmd)

	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Printf("[%s] Build Error: %v\nOutput: %s\n", node.IP, err, string(out))
	} else {
		fmt.Printf("[%s] Build successful.\n", node.IP)
	}
}

// 3. Raftの開始 (SSH + nohup)
func startRaft(node Node) {
	fmt.Printf("[%s] Starting Raft Node (Port: %d)...\n", node.IP, node.Port)

	// 既に起動しているプロセスがあればkillし、nohupでバックグラウンド起動
	// ログは node_ID.log に書き出す
	logFile := fmt.Sprintf("node_%d.log", node.ID)

	// コマンド: pkill (古いプロセス停止) + nohup (新規起動)
	// 自身のIDとPort、設定ファイルのパスを引数に渡す想定
	// pkillは`raft_server`の古いインスタンスを停止するために使用される
	startCmd := fmt.Sprintf(
		"cd %s && pkill %s; nohup ./%s start --id %d --port %d --conf %s > %s 2>&1 &",
		ProjectDir, BinaryName, BinaryName, node.ID, node.Port, ConfigFile, logFile,
	)

	cmd := exec.Command("ssh", fmt.Sprintf("%s@%s", User, node.IP), startCmd)

	if err := cmd.Run(); err != nil {
		// nohupを使う場合、sshが即切断されるとexit statusが変わることがあるが、ここでは簡易エラーハンドリング
		fmt.Printf("[%s] Start Command Sent (Check logs manually if needed). Error: %v\n", node.IP, err)
	} else {
		fmt.Printf("[%s] Start command executed.\n", node.IP)
	}
}

// 4. Raftプロセスを停止 (SSH + pkill) ★追加
func killRaftProcess(node Node) {
	fmt.Printf("[%s] Killing %s process...\n", node.IP, BinaryName)

	// ssh user@ip "pkill -9 raft_server"
	// pkill -9 でプロセスを強制終了する
	killCmd := fmt.Sprintf("pkill -9 %s", BinaryName)
	cmd := exec.Command("ssh", fmt.Sprintf("%s@%s", User, node.IP), killCmd)

	// CombinedOutputを実行し、エラーをチェック
	out, err := cmd.CombinedOutput()

	if err != nil {
		// pkillは、マッチするプロセスが見つからなかった場合(Exit Code 1)、エラーを返します。
		// プロセスが停止していることが目的なので、Exit Code 1は成功と見なします。
		if exitError, ok := err.(*exec.ExitError); ok && exitError.ExitCode() == 1 {
			fmt.Printf("[%s] %s process was already stopped or not found (Exit Code 1).\n", node.IP, BinaryName)
			return // 成功と見なして終了
		}
		// その他のエラー (SSH接続失敗、権限不足など)
		fmt.Printf("[%s] Kill Error: %v\nOutput: %s\n", node.IP, err, string(out))
	} else {
		fmt.Printf("[%s] %s process killed successfully.\n", node.IP, BinaryName)
	}
}
