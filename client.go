package main

import (
	"fmt"
	"math/rand"
	"time"
)

const (
	VALUE_MAX       = 1500
	CLIENT_INTERVAL = 500 * time.Millisecond
)

type Client struct {
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

func (c *Client) sendClientRequest(ch chan []byte) {
	command := c.createRandomCommand()
	ch <- command
}

func (r *Raft) internalClient() {
	client := &Client{}
	for {
		if r.state == LEADER {
			client.sendClientRequest(r.ClientCh)
			time.Sleep(CLIENT_INTERVAL)
		}
	}
}
