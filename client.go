package main

import (
	"context"
	"fmt"
	"github.com/sourcegraph/conc/pool"
	"math/rand"
	"time"
)

const (
	VALUE_MAX           = 1500
	CLIENT_START        = 4000 * time.Millisecond
	EXPERIMENT_DURATION = 10000 * time.Millisecond
	YCSB_A              = 50
	YCSB_B              = 5
	YCSB_C              = 0
)

type Response struct {
	success bool
	value   string
}

type WorkerResult struct {
	count    int
	duration time.Duration
}

type Client struct {
	sendCh        chan []byte
	internalState map[string]string
}

func (c *Client) randomKey() string {
	keys := []string{"x", "y", "z", "a", "b", "c"}
	return keys[rand.Intn(len(keys))]
}

func (c *Client) randomValue() string {
	return fmt.Sprintf("value%d", rand.Intn(VALUE_MAX))
}

func (c *Client) createYCSBCommand(writeRatio int) []byte {
	opRand := rand.Intn(100)
	if opRand < writeRatio {
		key := c.randomKey()
		value := c.randomValue()
		commandString := fmt.Sprintf("SET %s %s", key, value)
		return []byte(commandString)
	} else {
		key := c.randomKey()
		commandString := fmt.Sprintf("GET %s", key)
		return []byte(commandString)
	}
}

func (c *Client) validateResponse(command []byte, resp Response) bool {
	cmdStr := string(command)
	var key, value string

	if len(cmdStr) >= 3 && cmdStr[:3] == "SET" {
		fmt.Sscanf(cmdStr, "SET %s %s", &key, &value)
		storedValue, exists := c.internalState[key]
		return exists && storedValue == value && resp.success
	} else if len(cmdStr) >= 3 && cmdStr[:3] == "GET" {
		fmt.Sscanf(cmdStr, "GET %s", &key)
		storedValue, exists := c.internalState[key]
		if exists {
			return resp.success && resp.value == storedValue
		} else {
			return !resp.success
		}
	} else if len(cmdStr) >= 6 && cmdStr[:6] == "DELETE" {
		fmt.Sscanf(cmdStr, "DELETE %s", &key)
		_, exists := c.internalState[key]
		return !exists && resp.success
	}
	return false
}

func (r *Raft) concClient() {
	for r.state != LEADER {
	}
	fmt.Println("ConcClient starting experiment...")

	ctx, cancel := context.WithTimeout(context.Background(), EXPERIMENT_DURATION)
	defer cancel()

	p := pool.NewWithResults[WorkerResult]().WithErrors().WithMaxGoroutines(r.workers)
	for i := 0; i < r.workers; i++ {
		p.Go(func() (WorkerResult, error) { return concClientWorker(ctx, r) })
	}
	results, err := p.Wait()
	//cal throughput
	if err != nil {
		fmt.Println("ConcClient encountered error:", err)
		return
	}
	totalCommands := 0
	var totalDuration time.Duration
	for _, res := range results {
		totalCommands += res.count
		totalDuration += res.duration
	}
	throughput := float64(totalCommands) / EXPERIMENT_DURATION.Seconds()
	avgLatency := float64(0)
	if totalCommands > 0 {
		avgLatency = float64(totalDuration.Milliseconds()) / float64(totalCommands)
	}

	fmt.Printf("ConcClient total commands processed: %d\n", totalCommands)
	fmt.Printf("ConcClient throughput: %.2f commands/second\n", throughput)
	fmt.Printf("ConcClient average latency: %.2f ms\n", avgLatency)

	// CSV output for makefile to capture
	workloadName := "unknown"
	switch r.workload {
	case 50:
		workloadName = "ycsb-a"
	case 5:
		workloadName = "ycsb-b"
	case 0:
		workloadName = "ycsb-c"
	}
	fmt.Printf("RESULT:%s,%d,%d,%d,%.2f,%.2f\n", workloadName, r.readBatchSize, r.writeBatchSize, r.workers, throughput, avgLatency)
}

func concClientWorker(ctx context.Context, r *Raft) (WorkerResult, error) {
	client := &Client{
		internalState: make(map[string]string),
	}
	res := WorkerResult{}
	for {
		select {
		case <-ctx.Done():
			return res, nil
		default:
		}

		if r.state == LEADER {
			command := client.createYCSBCommand(r.workload)
			req := ClientRequest{
				Command: command,
				RespCh:  make(chan Response, 1),
			}

			start := time.Now()
			select {
			case <-ctx.Done():
				return res, nil
			case r.ReqCh <- req:
			}

			select {
			case <-ctx.Done():
				return res, nil
			case resp := <-req.RespCh:
				if resp.success {
					res.count += 1
					res.duration += time.Since(start)
				} else {
					fmt.Println("ConcClient command failed:", string(command))
					return res, fmt.Errorf("command failed")
				}
			}
		} else {
			select {
			case <-ctx.Done():
				return res, nil
			case <-time.After(10 * time.Millisecond):
			}
		}
	}
}
