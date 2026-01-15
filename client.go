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

	p := pool.NewWithResults[int]().WithErrors().WithMaxGoroutines(r.workers)
	for i := 0; i < r.workers; i++ {
		p.Go(func() (int, error) { return concClientWorker(ctx, r) })
	}
	results, err := p.Wait()
	//cal throughput
	if err != nil {
		fmt.Println("ConcClient encountered error:", err)
		return
	}
	totalCommands := 0
	for _, cnt := range results {
		totalCommands += cnt
	}
	throughput := float64(totalCommands) / EXPERIMENT_DURATION.Seconds()
	fmt.Printf("ConcClient total commands processed: %d\n", totalCommands)
	fmt.Printf("ConcClient throughput: %.2f commands/second\n", throughput)
}

func concClientWorker(ctx context.Context, r *Raft) (int, error) {
	client := &Client{
		internalState: make(map[string]string),
	}
	cnt := 0
	for {
		select {
		case <-ctx.Done():
			return cnt, nil
		default:
		}

		if r.state == LEADER {
			command := client.createYCSBCommand(YCSB_A)
			req := ClientRequest{
				Command: command,
				RespCh:  make(chan Response, 1),
			}

			select {
			case <-ctx.Done():
				return cnt, nil
			case r.ReqCh <- req:
			}

			select {
			case <-ctx.Done():
				return cnt, nil
			case resp := <-req.RespCh:
				if resp.success {
					cnt += 1
				} else {
					fmt.Println("ConcClient command failed:", string(command))
					return cnt, fmt.Errorf("command failed")
				}
			}
		} else {
			select {
			case <-ctx.Done():
				return cnt, nil
			case <-time.After(10 * time.Millisecond):
			}
		}
	}
}
