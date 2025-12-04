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

func NewRaft(id string, peers []string) *Raft {
	r := &Raft{
		conns:       make(map[string]*rpc.Client),
		currentTerm: 0,
		votedFor:    -1,
		log:         make([]LogEntry, 0),
		commitIndex: 0,
		lastApplied: 0,
		nextIndex:   make(map[int]int),
		matchIndex:  make(map[int]int),
		id:          id,
		state:       FOLLOWER,
	}
	go r.listenRPC()
	go r.initConns(peers)
	return r
}

func main() {

}
