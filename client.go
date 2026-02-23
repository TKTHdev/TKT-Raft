package main

import (
	"context"
	"fmt"
	"math/rand"
	"net/rpc"
	"sort"
	"sync"
	"time"
)

const (
	EXPERIMENT_DURATION = 10 * time.Second
	VALUE_MAX           = 1500
)

type WorkerResult struct {
	count    int
	duration time.Duration
}

type Client struct {
	peers    map[int]string
	peerIDs  []int
	conns    map[int]*rpc.Client
	mu       sync.Mutex
	leaderID int
	workers  int
	numKeys  int
	workload int
	debug    bool
}

func NewClient(confPath string, workers, numKeys, workload int, debug bool) *Client {
	peers := parseConfig(confPath)
	ids := make([]int, 0, len(peers))
	for id := range peers {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	return &Client{
		peers:    peers,
		peerIDs:  ids,
		conns:    make(map[int]*rpc.Client),
		leaderID: -1,
		workers:  workers,
		numKeys:  numKeys,
		workload: workload,
		debug:    debug,
	}
}

func (c *Client) getConn(id int) *rpc.Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conns[id] == nil {
		conn, err := rpc.Dial("tcp", c.peers[id])
		if err != nil {
			return nil
		}
		c.conns[id] = conn
	}
	return c.conns[id]
}

func (c *Client) invalidateConn(id int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conns[id] != nil {
		c.conns[id].Close()
		c.conns[id] = nil
	}
	if c.leaderID == id {
		c.leaderID = -1
	}
}

func (c *Client) execute(command []byte) (string, bool) {
	c.mu.Lock()
	leader := c.leaderID
	c.mu.Unlock()

	// Try cached leader first, then all others
	tryList := make([]int, 0, len(c.peerIDs))
	if leader != -1 {
		tryList = append(tryList, leader)
	}
	for _, id := range c.peerIDs {
		if id != leader {
			tryList = append(tryList, id)
		}
	}

	for _, id := range tryList {
		conn := c.getConn(id)
		if conn == nil {
			continue
		}
		args := &ExecuteArgs{Command: command}
		reply := &ExecuteReply{}
		if err := conn.Call(Execute, args, reply); err != nil {
			c.invalidateConn(id)
			continue
		}
		if !reply.IsLeader {
			continue
		}
		c.mu.Lock()
		c.leaderID = id
		c.mu.Unlock()
		return reply.Value, reply.Success
	}
	return "", false
}

func (c *Client) Run() {
	workloadName := map[int]string{50: "ycsb-a", 5: "ycsb-b", 0: "ycsb-c"}[c.workload]
	fmt.Printf("[Client] Peers: %v\n", c.peers)
	fmt.Printf("[Client] Workload: %s, Workers: %d, Keys: %d\n", workloadName, c.workers, c.numKeys)

	time.Sleep(2 * time.Second)
	c.runBenchmark()
}

func (c *Client) runBenchmark() {
	ctx, cancel := context.WithTimeout(context.Background(), EXPERIMENT_DURATION)
	defer cancel()

	resultCh := make(chan WorkerResult, c.workers)
	var wg sync.WaitGroup
	for i := 0; i < c.workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resultCh <- c.worker(ctx)
		}()
	}
	wg.Wait()
	close(resultCh)

	var totalCount int
	var totalDuration time.Duration
	for res := range resultCh {
		totalCount += res.count
		totalDuration += res.duration
	}

	throughput := float64(totalCount) / EXPERIMENT_DURATION.Seconds()
	avgLatency := float64(0)
	if totalCount > 0 {
		avgLatency = float64(totalDuration.Milliseconds()) / float64(totalCount)
	}

	workloadName := map[int]string{50: "ycsb-a", 5: "ycsb-b", 0: "ycsb-c"}[c.workload]
	fmt.Printf("Benchmark completed\n")
	fmt.Printf("Total ops: %d\n", totalCount)
	fmt.Printf("Throughput: %.2f ops/sec\n", throughput)
	fmt.Printf("Avg latency: %.2f ms\n", avgLatency)
	fmt.Printf("RESULT:%s,%d,%d,%.2f,%.2f\n", workloadName, c.workers, c.numKeys, throughput, avgLatency)
}

func (c *Client) worker(ctx context.Context) WorkerResult {
	res := WorkerResult{}
	for {
		select {
		case <-ctx.Done():
			return res
		default:
		}

		key := fmt.Sprintf("k%d", rand.Intn(c.numKeys))
		start := time.Now()
		var ok bool

		if rand.Intn(100) < c.workload {
			value := randomValue(VALUE_MAX)
			_, ok = c.execute([]byte(fmt.Sprintf("SET %s %s", key, value)))
		} else {
			_, ok = c.execute([]byte(fmt.Sprintf("GET %s", key)))
		}

		if ok {
			res.count++
			res.duration += time.Since(start)
		}
	}
}

func randomValue(size int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, size)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}
