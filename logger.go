package main

import (
	"fmt"
	"log"
)

const (
	BLUE = iota
	GREEN
	RED
	YELLOW
	WHITE
)

func (r *Raft) logPut(msg string, colour int) {
	colors := map[int]string{
		BLUE:   "\033[34m",
		GREEN:  "\033[32m",
		RED:    "\033[31m",
		YELLOW: "\033[33m",
		WHITE:  "\033[37m",
	}
	color, ok := colors[colour]
	if !ok {
		color = "\033[0m"
	}
	reset := "\033[0m"

	logPrefix := fmt.Sprintf("[Node: %d Term: %d] ", r.me, r.currentTerm)
	log.Printf("%s%s%s", color, logPrefix+msg, reset)
}
