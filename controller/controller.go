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
	"strconv"
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
	User       = "tkt"               // SSHユーザー名
	ProjectDir = "~/study/raft"      // リモートのプロジェクトディレクトリ
	BinaryName = "raft_server"       // 生成されるバイナリ名
	ConfigFile = "../cluster.conf"   // 設定ファイル名 (controllerから見た相対パス)
	LogDir     = "~/study/raft/logs" // ログディレクトリ
)

func main() {
	// コマンドライン引数のパース
	deployCmd := flag.NewFlagSet("deploy", flag.ExitOnError)
	buildCmd := flag.NewFlagSet("build", flag.ExitOnError)
	sendBinCmd := flag.NewFlagSet("send-bin", flag.ExitOnError)
	startCmd := flag.NewFlagSet("start", flag.ExitOnError)
	killCmd := flag.NewFlagSet("kill", flag.ExitOnError)

	if len(os.Args) < 2 {
		fmt.Println("Usage: go run controller.go [deploy|build|send-bin|start|kill]")
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
	case "send-bin":
		sendBinCmd.Parse(os.Args[2:])
		// 1. まずローカルで一回だけビルドする
		if err := buildLocal(); err != nil {
			log.Fatalf("Local build failed: %v", err)
		}
		// 2. ビルドしたバイナリを並列で配布する
		runParallel(nodes, distributeBinary)
	case "start":
		startCmd.Parse(os.Args[2:])
		runParallel(nodes, startRaft)
	case "kill":
		killCmd.Parse(os.Args[2:])
		args := killCmd.Args()

		var targetNodes []Node

		// 引数がない、または "all" の場合は全ノード対象
		if len(args) == 0 || args[0] == "all" {
			fmt.Println("Target: ALL nodes")
			targetNodes = nodes
		} else {
			// 特定のID指定とみなして数値変換
			targetID, err := strconv.Atoi(args[0])
			if err != nil {
				log.Fatalf("Invalid argument: '%s'. Use a specific Node ID (e.g., 0) or 'all'.", args[0])
			}

			// ノードリストから該当IDを探す
			found := false
			for _, node := range nodes {
				if node.ID == targetID {
					targetNodes = append(targetNodes, node)
					found = true
					break
				}
			}
			if !found {
				log.Fatalf("Node ID %d not found in config.", targetID)
			}
			fmt.Printf("Target: Node ID %d only\n", targetID)
		}

		runParallel(targetNodes, killRaftProcess)

	default:
		fmt.Println("Unknown command. Use: deploy, build, send-bin, start, kill")
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

// ---------------------------------------------------------
//  タスク関数群
// ---------------------------------------------------------

// ローカルでのクロスコンパイル
func buildLocal() error {
	fmt.Println("[Local] Building binary for Linux/amd64...")

	targetPath := ".." // mainパッケージがある場所（親ディレクトリ）

	// ★修正: 出力先を親ディレクトリの raft_server に指定
	outputPath := filepath.Join("..", BinaryName)

	cmd := exec.Command("go", "build", "-o", outputPath, targetPath)

	// クロスコンパイル環境変数の設定
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64")

	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Printf("[Local] Build Error: %s\n", string(out))
		return err
	}
	fmt.Println("[Local] Build successful.")
	return nil
}

// バイナリの配布 (SCP)
func distributeBinary(node Node) {
	fmt.Printf("[%s] Sending binary...\n", node.IP)

	// ★修正: 送信元バイナリパスを親ディレクトリのものに指定
	localBinPath := filepath.Join("..", BinaryName)
	remoteDest := filepath.Join(ProjectDir, BinaryName)

	cmd := exec.Command("scp", localBinPath, fmt.Sprintf("%s@%s:%s", User, node.IP, remoteDest))

	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Printf("[%s] SCP Error: %v\nOutput: %s\n", node.IP, err, string(out))
	} else {
		// 実行権限を付与
		chmodCmd := exec.Command("ssh", fmt.Sprintf("%s@%s", User, node.IP), "chmod +x "+remoteDest)
		chmodCmd.Run()
		fmt.Printf("[%s] Binary sent and chmod +x applied.\n", node.IP)
	}
}

// 設定ファイルの配布
func distributeConfig(node Node) {
	fmt.Printf("[%s] Distributing config...\n", node.IP)
	remotePath := filepath.Join(ProjectDir, ConfigFile)
	cmd := exec.Command("scp", ConfigFile, fmt.Sprintf("%s@%s:%s", User, node.IP, remotePath))

	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Printf("[%s] SCP Error: %v\nOutput: %s\n", node.IP, err, string(out))
	} else {
		fmt.Printf("[%s] Config distributed.\n", node.IP)
	}
}

// リモートでのビルド (SSH)
func buildProject(node Node) {
	fmt.Printf("[%s] Building project remotely...\n", node.IP)
	buildCmd := fmt.Sprintf("cd %s && go mod tidy && go build -o %s", ProjectDir, BinaryName)
	cmd := exec.Command("ssh", fmt.Sprintf("%s@%s", User, node.IP), buildCmd)

	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Printf("[%s] Build Error: %v\nOutput: %s\n", node.IP, err, string(out))
	} else {
		fmt.Printf("[%s] Remote build successful.\n", node.IP)
	}
}

// Raftの開始
func startRaft(node Node) {
	fmt.Printf("[%s] Starting Raft Node (Port: %d)...\n", node.IP, node.Port)
	logFile := fmt.Sprintf("%s/node_%d.log", LogDir, node.ID)

	startCmd := fmt.Sprintf(
		"cd %s && pkill %s; nohup ./%s start --id %d --conf %s > %s 2>&1 &",
		ProjectDir, BinaryName, BinaryName, node.ID, ConfigFile, logFile,
	)

	cmd := exec.Command("ssh", fmt.Sprintf("%s@%s", User, node.IP), startCmd)

	if err := cmd.Run(); err != nil {
		fmt.Printf("[%s] Start Command Sent (Check logs manually). Error: %v\n", node.IP, err)
	} else {
		fmt.Printf("[%s] Start command executed.\n", node.IP)
	}
}

// Raftプロセス停止
func killRaftProcess(node Node) {
	fmt.Printf("[%s] Killing %s process...\n", node.IP, BinaryName)
	killCmd := fmt.Sprintf("pkill -9 %s", BinaryName)
	cmd := exec.Command("ssh", fmt.Sprintf("%s@%s", User, node.IP), killCmd)

	out, err := cmd.CombinedOutput()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok && exitError.ExitCode() == 1 {
			fmt.Printf("[%s] Process already stopped.\n", node.IP)
			return
		}
		fmt.Printf("[%s] Kill Error: %v\nOutput: %s\n", node.IP, err, string(out))
	} else {
		fmt.Printf("[%s] Killed successfully.\n", node.IP)
	}
}
