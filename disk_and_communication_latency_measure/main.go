package main

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"time"
)

// Config definition based on cluster.conf
type NodeConfig struct {
	ID   int    `json:"id"`
	IP   string `json:"ip"`
	Port int    `json:"port"`
}

func main() {
	mode := flag.String("mode", "disk", "Mode: disk, server, client")
	nodeID := flag.Int("id", 1, "Node ID (for server mode)")
	targetID := flag.Int("target", 1, "Target Node ID (for client mode)")
	configPath := flag.String("config", "../cluster.conf", "Path to cluster.conf")
	flag.Parse()

	switch *mode {
	case "disk":
		fmt.Println("=== Starting Disk Benchmark ===")
		measureDiskLatency()
	case "server":
		fmt.Println("=== Starting Network Server ===")
		runServer(*nodeID, *configPath)
	case "client":
		fmt.Println("=== Starting Network Client ===")
		runClient(*targetID, *configPath)
	default:
		fmt.Println("Invalid mode. Use disk, server, or client.")
		os.Exit(1)
	}
}

// --- Disk Measurement ---

type LogEntry struct {
	Term    int
	Command []byte
}

func measureDiskLatency() {
	filename := "benchmark_test_wal.bin"
	defer os.Remove(filename)

	batchSizes := []int{1, 2, 4, 8, 16, 32, 64, 128}
	payloadSize := 100
	payload := make([]byte, payloadSize) // 100 bytes payload
	entrySize := 8 + 8 + payloadSize     // Term(8) + CmdLen(8) + Command(payloadSize)
	fmt.Printf("Entry Size: %d bytes (Term: 8, CmdLen: 8, Payload: %d)\n\n", entrySize, payloadSize)

	for _, batchSize := range batchSizes {
		// Clean up file for each batch size test to ensure fair ground
		os.Remove(filename)
		f, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0644)
		if err != nil {
			fmt.Printf("Error opening file: %v\n", err)
			return
		}

		// Use bufio as seen in storage.go
		writer := bufio.NewWriter(f)

		iterations := 50 // Perform 50 batch writes to get an average
		var totalDuration time.Duration

		for i := 0; i < iterations; i++ {
			// Prepare batch entries
			entries := make([]LogEntry, batchSize)
			for j := 0; j < batchSize; j++ {
				entries[j] = LogEntry{Term: 1, Command: payload}
			}

			start := time.Now()

			// Simulate Storage.AppendEntries logic
			for _, entry := range entries {
				binary.Write(writer, binary.LittleEndian, int64(entry.Term))
				binary.Write(writer, binary.LittleEndian, int64(len(entry.Command)))
				writer.Write(entry.Command)
			}

			writer.Flush()
			f.Sync()

			duration := time.Since(start)
			totalDuration += duration
		}

		avgLatency := totalDuration / time.Duration(iterations)
		totalBatchSize := entrySize * batchSize
		fmt.Printf("Batch Size: %3d | Total Size: %5d bytes | Avg Latency: %v\n", batchSize, totalBatchSize, avgLatency)

		f.Close()
	}
}

// --- Network Measurement ---

func loadConfig(path string) ([]NodeConfig, error) {
	configFile, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var nodes []NodeConfig
	if err := json.Unmarshal(configFile, &nodes); err != nil {
		return nil, err
	}
	return nodes, nil
}

func runServer(id int, configPath string) {
	nodes, err := loadConfig(configPath)
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		return
	}

	var myConfig NodeConfig
	found := false
	for _, n := range nodes {
		if n.ID == id {
			myConfig = n
			found = true
			break
		}
	}
	if !found {
		fmt.Printf("Node ID %d not found in config\n", id)
		return
	}

	// For listening, we usually bind to :Port or 0.0.0.0:Port to accept from anywhere,
	// but using the IP from config if it's specific.
	address := fmt.Sprintf(":%d", myConfig.Port) // Listen on all interfaces for that port

	listener, err := net.Listen("tcp", address)
	if err != nil {
		fmt.Printf("Failed to bind to %s: %v\n", address, err)
		return
	}
	defer listener.Close()

	fmt.Printf("Server listening on %s (ID: %d)\n", address, id)

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Printf("Accept error: %v\n", err)
			continue
		}
		go handleConnection(conn)
	}
}

func handleConnection(c net.Conn) {
	defer c.Close()
	// Echo server
	buf := make([]byte, 4096)
	for {
		n, err := c.Read(buf)
		if err != nil {
			if err != io.EOF {
				fmt.Printf("Read error: %v\n", err)
			}
			return
		}
		// Write back
		if _, err := c.Write(buf[:n]); err != nil {
			fmt.Printf("Write error: %v\n", err)
			return
		}
	}
}

func runClient(targetID int, configPath string) {
	nodes, err := loadConfig(configPath)
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		return
	}

	var targetNode NodeConfig
	found := false
	for _, n := range nodes {
		if n.ID == targetID {
			targetNode = n
			found = true
			break
		}
	}
	if !found {
		fmt.Printf("Target Node ID %d not found in config\n", targetID)
		return
	}

	address := fmt.Sprintf("%s:%d", targetNode.IP, targetNode.Port)
	fmt.Printf("Connecting to Node %d at %s...\n", targetID, address)

	conn, err := net.DialTimeout("tcp", address, 5*time.Second)
	if err != nil {
		fmt.Printf("Failed to connect: %v\n", err)
		return
	}
	defer conn.Close()

	fmt.Println("Connected. Starting ping-pong latency test...")

	iterations := 100
	var totalDuration time.Duration
	msg := []byte("ping")
	recvBuf := make([]byte, 1024)

	for i := 0; i < iterations; i++ {
		start := time.Now()

		if _, err := conn.Write(msg); err != nil {
			fmt.Printf("Write error: %v\n", err)
			return
		}

		if _, err := io.ReadFull(conn, recvBuf[:len(msg)]); err != nil {
			fmt.Printf("Read error: %v\n", err)
			return
		}

		duration := time.Since(start)
		totalDuration += duration

		// Optional small sleep? No, we want raw latency.
	}

	avgLatency := totalDuration / time.Duration(iterations)
	fmt.Printf("Average Round-Trip Latency: %v\n", avgLatency)
	fmt.Printf("(Measured over %d iterations)\n", iterations)
}
