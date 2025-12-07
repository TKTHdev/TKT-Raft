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
	Command []byte
	Term    int
}

type Raft struct {
	//net rpc conn
	currentTerm    int
	votedFor       int
	log            []LogEntry
	commitIndex    int
	lastApplied    int
	nextIndex      map[int]int
	matchIndex     map[int]int
	me             int
	state          int
	rpcConns       map[int]*rpc.Client
	heartBeatCh    chan bool
	clusterSize    int32
	ClientCh       chan []byte
	StateMachineCh chan []byte
	StateMachine   map[string]string
}

func NewRaft(id int, confPath string) *Raft {
	peerIPPort := parseConfig(confPath)
	r := &Raft{
		currentTerm:    -1,
		votedFor:       -2,
		log:            []LogEntry{{Command: nil, Term: -1}},
		commitIndex:    -1,
		lastApplied:    -1,
		nextIndex:      make(map[int]int),
		matchIndex:     make(map[int]int),
		me:             id,
		state:          FOLLOWER,
		rpcConns:       make(map[int]*rpc.Client),
		heartBeatCh:    make(chan bool),
		clusterSize:    int32(len(peerIPPort)),
		ClientCh:       make(chan []byte),
		StateMachineCh: make(chan []byte),
		StateMachine:   make(map[string]string),
	}
	for peerID, _ := range peerIPPort {
		r.nextIndex[peerID] = 1
		r.matchIndex[peerID] = 0
	}

	go r.listenRPC(peerIPPort)
	go r.initConns(peerIPPort)
	go r.internalClient()
	go r.runReplication()
	go r.waitForApply()
	return r
}
