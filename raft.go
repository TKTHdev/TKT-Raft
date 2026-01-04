package main

import (
	"net/rpc"
	"sync"
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
	StateMachineCh chan []byte
	StateMachine   map[string]string
	ReqCh          chan []byte
	RespCh         chan Response
	mu             sync.RWMutex
	peerIPPort     map[int]string
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
		StateMachineCh: make(chan []byte),
		StateMachine:   make(map[string]string),
		ReqCh:       make(chan []byte),
		RespCh:       make(chan Response),
		mu:             sync.RWMutex{},
		peerIPPort:     peerIPPort,
	}
	for peerID, _ := range peerIPPort {
		r.nextIndex[peerID] = 1
		r.matchIndex[peerID] = 0
	}

	go r.listenRPC(peerIPPort)
	go r.internalClient()
	go r.handleClientRequest()
	return r
}
