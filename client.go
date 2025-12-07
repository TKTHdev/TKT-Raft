package main

import (
	"fmt"
	"math/rand"
)

const (
	VALUE_MAX = 1500
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
	return string(rand.Intn(VALUE_MAX))
}

func (c *Client) createRandomCommand() string {
	op := c.randomOperation()
	key := c.randomKey()
	if op == "SET" {
		value := c.randomValue()
		return fmt.Sprintf("%s %s %s", op, key, value)
	} else {
		return fmt.Sprintf("%s %s", op, key)
	}
}

func sendClientRequest(ch chan interface{}) {
	command := createRandomCommand()
	ch <- command
}
