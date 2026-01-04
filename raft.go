package main

import (
	"fmt"
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
	storage        *Storage
}

func NewRaft(id int, confPath string) *Raft {
	peerIPPort := parseConfig(confPath)
	storage, err := NewStorage(id)
	if err != nil {
		panic(err)
	}
	term, votedFor, err := storage.LoadState()
	if err != nil {
		panic(err)
	}
	logs, err := storage.LoadLog()
	if err != nil {
		panic(err)
	}
	// Prepend dummy entry
	fullLog := []LogEntry{{Command: nil, Term: 0}}
	fullLog = append(fullLog, logs...)

	r := &Raft{
		currentTerm:    term,
		votedFor:       votedFor,
		log:            fullLog,
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
		ReqCh:          make(chan []byte),
		RespCh:         make(chan Response),
		mu:             sync.RWMutex{},
		peerIPPort:     peerIPPort,
		storage:        storage,
	}
	for peerID, _ := range peerIPPort {
		r.nextIndex[peerID] = len(fullLog)
		r.matchIndex[peerID] = 0
	}

	go r.listenRPC(peerIPPort)
	go r.internalClient()
	go r.handleClientRequest()
	go r.runApplier()
	return r
}

func (r *Raft) persistState() {
	if err := r.storage.SaveState(r.currentTerm, r.votedFor); err != nil {
		fmt.Printf("Error persisting state: %v\n", err)
	}
}
