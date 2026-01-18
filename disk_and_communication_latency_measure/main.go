package main

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
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
	fmt.Println("=== Starting Benchmarks ===")

	// 1. Disk Write Latency
	fmt.Println("\n--- Disk Write Latency (Batch sizes: 1, 2, 4, ..., 128) ---")
	measureDiskLatency()

	// 2. Network Latency
	fmt.Println("\n--- Node-to-Node TCP Communication Latency ---")
	measureNetworkLatency()
}

// --- Disk Measurement ---

type LogEntry struct {
	Term    int
	Command []byte
}

func measureDiskLatency() {
	filename := "benchmark_test_wal.bin"
	defer os.Remove(filename)

	batchSizes := []int{1, 2, 4, 8, 16, 32, 64, 128, 256, 512, 1024, 2048}
		payloadSize := 100
		payload := make([]byte, payloadSize) // 100 bytes payload
		entrySize := 8 + 8 + payloadSize // Term(8) + CmdLen(8) + Command(payloadSize)
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
		}}

// --- Network Measurement ---

func measureNetworkLatency() {
	// Read cluster.conf
	configFile, err := os.ReadFile("../cluster.conf")
	if err != nil {
		fmt.Printf("Error reading ../cluster.conf: %v\n", err)
		return
	}

	var nodes []NodeConfig
	if err := json.Unmarshal(configFile, &nodes); err != nil {
		fmt.Printf("Error parsing cluster.conf: %v\n", err)
		return
	}

	if len(nodes) < 2 {
		fmt.Println("Not enough nodes in cluster.conf to test communication.")
		return
	}

	serverNode := nodes[0]
	// We only need the address. Since we are likely on the same machine (localhost),
	// we will try to bind to the port defined in config.
	// If the real server is running, this will fail.
	address := fmt.Sprintf("%s:%d", serverNode.IP, serverNode.Port)

	// Try to start a listener
	listener, err := net.Listen("tcp", address)
	if err != nil {
		fmt.Printf("Could not bind to %s. \nReason: %v\n", address, err)
		fmt.Println("Make sure the raft servers are NOT running.")
		return
	}
	defer listener.Close()

	// Server goroutine (Echo server)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				// Read what is sent and write it back
				buf := make([]byte, 1024)
				for {
					n, err := c.Read(buf)
					if err != nil {
						return
					}
					c.Write(buf[:n])
				}
			}(conn)
		}
	}()

	// Give server a moment to start
	time.Sleep(100 * time.Millisecond)

	// Client (Simulate Node 2 connecting to Node 1)
	clientNode := nodes[1] // Just for log, we connect TO serverNode
	fmt.Printf("Testing connection between Node %d (%s) -> Node %d (%s)\n",
		clientNode.ID, fmt.Sprintf("%s:%d", clientNode.IP, clientNode.Port),
		serverNode.ID, address)

	conn, err := net.Dial("tcp", address)
	if err != nil {
		fmt.Printf("Failed to connect: %v\n", err)
		return
	}
	defer conn.Close()

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
	}

	avgLatency := totalDuration / time.Duration(iterations)
	fmt.Printf("Average Round-Trip Latency: %v\n", avgLatency)
	fmt.Printf("(Measured over %d iterations)\n", iterations)
}
