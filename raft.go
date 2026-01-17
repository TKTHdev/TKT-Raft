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

type ClientRequest struct {
	Command []byte
	RespCh  chan Response
}

type Raft struct {
	//net rpc conn
	currentTerm      int
	votedFor         int
	log              []LogEntry
	commitIndex      int
	lastApplied      int
	nextIndex        map[int]int
	matchIndex       map[int]int
	me               int
	state            int
	rpcConns         map[int]*rpc.Client
	heartBeatCh      chan bool
	clusterSize      int32
	StateMachineCh   chan []byte
	StateMachine     map[string]string
	ReqCh            chan ClientRequest
	ReadCh           chan []ClientRequest
	pendingResponses map[int]chan Response
	mu               sync.RWMutex
	peerIPPort       map[int]string
	storage          *Storage
	commitCond       *sync.Cond
	replicating      map[int]bool
	newLogEntryCh    chan bool
	writeBatchSize   int
	readBatchSize    int
	workers          int
	debug            bool
	workload         int
}

func NewRaft(id int, confPath string, writeBatchSize int, readBatchSize int, workers int, debug bool, workload int) *Raft {
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
		currentTerm:      term,
		votedFor:         votedFor,
		log:              fullLog,
		commitIndex:      -1,
		lastApplied:      -1,
		nextIndex:        make(map[int]int),
		matchIndex:       make(map[int]int),
		me:               id,
		state:            FOLLOWER,
		rpcConns:         make(map[int]*rpc.Client),
		heartBeatCh:      make(chan bool, 1),
		clusterSize:      int32(len(peerIPPort)),
		StateMachineCh:   make(chan []byte),
		StateMachine:     make(map[string]string),
		ReqCh:            make(chan ClientRequest),
		ReadCh:           make(chan []ClientRequest),
		pendingResponses: make(map[int]chan Response),
		mu:               sync.RWMutex{},
		peerIPPort:       peerIPPort,
		storage:          storage,
		replicating:      make(map[int]bool),
		newLogEntryCh:    make(chan bool, 1),
		writeBatchSize:   writeBatchSize,
		readBatchSize:    readBatchSize,
		workers:          workers,
		debug:            debug,
		workload:         workload,
	}
	r.commitCond = sync.NewCond(&r.mu)
	for peerID, _ := range peerIPPort {
		r.nextIndex[peerID] = len(fullLog)
		r.matchIndex[peerID] = 0
	}

	go r.listenRPC(peerIPPort)
	//go r.internalClient()
	go r.concClient()
	go r.handleClientRequest()
	go r.runApplier()
	return r
}

func (r *Raft) sendRead(server int) bool {
	r.mu.Lock()
	if r.rpcConns[server] == nil {
		r.mu.Unlock()
		r.dialRPCToPeer(server)
		return false
	}
	client := r.rpcConns[server]
	args := &ReadArgs{
		Term: r.currentTerm,
	}
	r.mu.Unlock()

	reply := &ReadReply{}
	if err := client.Call(Read, args, reply); err != nil {
		r.mu.Lock()
		logMsg := fmt.Sprintf("Error sending Read RPC to node %d: %v", server, err)
		r.logPutLocked(logMsg, PURPLE)
		r.rpcConns[server] = nil
		r.mu.Unlock()
		r.dialRPCToPeer(server)
		return false
	}

	return reply.Success
}

func (r *Raft) persistState() {
	if err := r.storage.SaveState(r.currentTerm, r.votedFor); err != nil {
		fmt.Printf("Error persisting state: %v\n", err)
	}
}
