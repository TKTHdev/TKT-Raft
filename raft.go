package main

import "net/rpc"

const (
	LEADER = iota
	FOLLOWER
	CANDIDATE
)

type LogEntry struct {
	command interface{}
	Term    int
}

type Raft struct {
	//net rpc conn
	currentTerm int
	votedFor    int
	log         []LogEntry
	commitIndex int
	lastApplied int
	nextIndex   map[int]int
	matchIndex  map[int]int
	me          int
	state       int
	rpcConns    map[int]*rpc.Client
}

func NewRaft(id int, confPath string) *Raft {
	peerIPPort := parseConfig(confPath)
	r := &Raft{
		currentTerm: -1,
		votedFor:    -2,
		log:         make([]LogEntry, 0),
		commitIndex: -1,
		lastApplied: -1,
		nextIndex:   make(map[int]int),
		matchIndex:  make(map[int]int),
		me:          id,
		state:       FOLLOWER,
		rpcConns:    make(map[int]*rpc.Client),
	}
	go r.listenRPC(peerIPPort)
	go r.initConns(peerIPPort)
	return r
}
