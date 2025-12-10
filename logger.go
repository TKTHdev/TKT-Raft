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
	CYAN
	PURPLE
	MAGENTA
	ORANGE
	EMERALD
)

func (r *Raft) logPut(msg string, colour int) {
	colors := map[int]string{
		BLUE:    "\033[34m",
		GREEN:   "\033[32m",
		RED:     "\033[31m",
		YELLOW:  "\033[33m",
		WHITE:   "\033[37m",
		CYAN:    "\033[36m",
		PURPLE:  "\033[35m",
		MAGENTA: "\033[95m",
	}
	color, ok := colors[colour]
	if !ok {
		color = "\033[0m"
	}
	reset := "\033[0m"
	logPrefix := fmt.Sprintf("Node %d | Term %d | State %s | ", r.me, r.currentTerm, r.printStateMachineAsString())
	log.Printf("%s%s%s", color, logPrefix+msg, reset)
}

func (r *Raft) printLogEntriesAsString() string {
	logStr := "["
	for i, entry := range r.log {
		logStr += fmt.Sprintf("{Cmd: %v, Term: %d}", entry.Command, entry.Term)
		if i != len(r.log)-1 {
			logStr += ", "
		}
	}
	logStr += "]"
	return logStr
}

func (r *Raft) printStateMachineAsString() string {
	smStr := "{"
	i := 0
	for k, v := range r.StateMachine {
		smStr += fmt.Sprintf("%s: %s", k, v)
		if i != len(r.StateMachine)-1 {
			smStr += ", "
		}
		i++
	}
	smStr += "}"
	return smStr
}
