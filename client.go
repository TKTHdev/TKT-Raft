package main

import (
	"fmt"
	"math/rand"
	"time"
)

const (
	VALUE_MAX       = 1500
)

type Response struct {
	success bool
	value   string
}

type Client struct {
	sendCh        chan []byte
	internalState map[string]string
}

func (c *Client) randomOperation() string {
	operations := []string{"SET", "GET", "DELETE"}
	return operations[rand.Intn(len(operations))]
}

func (c *Client) randomKey() string {
	keys := []string{"x", "y", "z", "a", "b", "c"}
	return keys[rand.Intn(len(keys))]
}

func (c *Client) randomValue() string {
	return fmt.Sprintf("value%d", rand.Intn(VALUE_MAX))
}

func (c *Client) createRandomCommand() []byte {
	op := c.randomOperation()
	key := c.randomKey()
	if op == "SET" {
		value := c.randomValue()
		commandString := fmt.Sprintf("%s %s %s", op, key, value)
		return []byte(commandString)
	} else {
		commandString := fmt.Sprintf("%s %s", op, key)
		return []byte(commandString)
	}
}

func (c *Client) updateInternalState(command []byte) {
	cmdStr := string(command)
	var key, value string

	if len(cmdStr) >= 3 && cmdStr[:3] == "SET" {
		fmt.Sscanf(cmdStr, "SET %s %s", &key, &value)
		c.internalState[key] = value
	} else if len(cmdStr) >= 6 && cmdStr[:6] == "DELETE" {
		fmt.Sscanf(cmdStr, "DELETE %s", &key)
		delete(c.internalState, key)
	}
}

func (r *Raft) internalClient() {
	client := &Client{
		internalState: make(map[string]string),
	}

	for {
		if r.state == LEADER {
			command := client.createRandomCommand()
			
			r.ReqCh <- command
			start := time.Now()
			resp := <-r.RespCh
			end := time.Since(start)

			if resp.success {
				client.updateInternalState(command)
				fmt.Println("Client executed command:", string(command), "time:" ,end)
			} else {
				fmt.Println("Client command failed:", string(command))
			}
		}
	}
}