package main

import (
	"net/rpc"
)

const (
	LEADER = iota
	FOLLOWER
	CANDIDATE
)

type LogEntry struct {
	command interface{}
}

type Raft struct {
	//net rpc conn
	conns       map[string]*rpc.Client
	currentTerm int
	votedFor    int
	log         []LogEntry
	commitIndex int
	lastApplied int
	nextIndex   map[int]int
	matchIndex  map[int]int
	id          string
	state       int
}

func main() {

}
