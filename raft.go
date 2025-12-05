package main

import "net/rpc"

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
	peerIPPort  map[int]string
}

type HogeArgs struct {
}

type HogeReply struct {
	Message string
}

func (r *Raft) HogeRPC(args *HogeArgs, reply *HogeReply) error {
	reply.Message = "Hello from node " + string(r.me)
	return nil
}

func NewRaft(id int, confPath string) *Raft {

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
		peerIPPort:  parseConfig(confPath),
	}
	go r.listenRPC()
	go r.initConns()
	return r
}
