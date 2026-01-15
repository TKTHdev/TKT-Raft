package main

import (
	"fmt"
	"log"
	"sort"
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
	r.mu.RLock()
	defer r.mu.RUnlock()
	r.logPutLocked(msg, colour)
}

func (r *Raft) logPutLocked(msg string, colour int) {
	if !r.debug {
		return
	}
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
	//print Node ID, Term, State, leaderID, votedFor
	stateStr := ""
	switch r.state {
	case LEADER:
		stateStr = "LEADER"
	case FOLLOWER:
		stateStr = "FOLLOWER"
	case CANDIDATE:
		stateStr = "CANDIDATE"
	default:
		stateStr = "UNKNOWN"
	}
	logPrefix := fmt.Sprintf("[Node %d | Term %d | SM %s | Role %s | VotedFor %d] ", r.me, r.currentTerm, r.printStateMachineAsStringLocked(), stateStr, r.votedFor)
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

func (r *Raft) printStateMachineAsStringLocked() string {
	smStr := "{"
	keys := make([]string, 0, len(r.StateMachine))
	for k := range r.StateMachine {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for i, k := range keys {
		v := r.StateMachine[k]
		smStr += fmt.Sprintf("%s: %s", k, v)
		if i != len(keys)-1 {
			smStr += ", "
		}
	}
	smStr += "}"
	return smStr
}