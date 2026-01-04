package main

import (
	"fmt"
	"log"
	"math/rand"
	"sync/atomic"
	"time"
)

const (
	MINELECTION_TIMEOUT   = 150 * time.Millisecond
	MAXELECTION_TIMEOUT   = 300 * time.Millisecond
	COMMUNICATION_LATENCY = 100 * time.Millisecond
	AFTER_START_DELAY     = 1000 * time.Millisecond
	HEARTBEAT_INTERVAL    = 10 * time.Millisecond
)

func (r *Raft) Run() {
	time.Sleep(AFTER_START_DELAY) //wait for connections to establish
	r.dialRPCToAllPeers()
	time.Sleep(AFTER_START_DELAY) //wait for connections to establish
	for {
		state := r.state
		switch state {
		case FOLLOWER:
			if err := r.doFollower(); err != nil {
				log.Println("Error in follower state:", err)
			}
		case LEADER:
			if err := r.doLeader(); err != nil {
				r.logPut("I am a leader", YELLOW)
				log.Println("Error in leader state:", err)
			}
		}
	}
}

func (r *Raft) doFollower() error {
	timeout := MINELECTION_TIMEOUT + time.Duration(rand.Intn(int(MAXELECTION_TIMEOUT-MINELECTION_TIMEOUT)))
	timer := time.NewTimer(timeout)
	select {
	case <-timer.C:
		//election timeout
		r.logPut("Haven't received heartbeat, starting election", RED)
		r.startElection()
	case <-r.heartBeatCh:
		r.logPut("Received heartbeat, resetting election timer", WHITE)
		//received heartbeat
	}
	return nil
}

func (r *Raft) doLeader() error {
	ids := make([]int, 0, r.clusterSize)
	for id := range r.rpcConns {
		ids = append(ids, id)
	}
	for _, id := range ids {
		if id != r.me {
			msg := fmt.Sprintf("Sending heartbeat to node %d", id)
			r.logPut(msg, WHITE)
			go r.sendAppendEntries(id)
		}
	}
	time.Sleep(HEARTBEAT_INTERVAL)
	return nil
}

func (r *Raft) runApplier() {
	for {
		time.Sleep(10 * time.Millisecond)
		r.updateCommitIndex()
		r.updateStateMachine()
	}
}

func (r *Raft) updateCommitIndex() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.state != LEADER {
		return
	}
	for i := r.commitIndex + 1; i < len(r.log); i++ {
		var cnt int32 = 1 //count self
		for peerID, matchIdx := range r.matchIndex {
			if peerID != r.me && matchIdx >= i && r.log[i].Term == r.currentTerm {
				atomic.AddInt32(&cnt, 1)
			}
		}
		if atomic.LoadInt32(&cnt) > r.clusterSize/2 {
			r.commitIndex = i
		}
	}
}

func (r *Raft) updateStateMachine() {
	for {
		r.mu.Lock()
		if r.lastApplied >= r.commitIndex {
			r.mu.Unlock()
			return
		}
		idx := r.lastApplied + 1
		entry := r.log[idx]
		r.mu.Unlock()

		//apply to state machine
		r.applyCommand(entry.Command)
		logMsg := fmt.Sprintf("Applied log entry %d to state machine: %s", idx, string(entry.Command))
		r.logPut(logMsg, ORANGE)

		r.mu.Lock()
		r.lastApplied = idx
		r.mu.Unlock()
	}
}

func (r *Raft) startElection() {
	r.state = CANDIDATE
	r.currentTerm++
	r.votedFor = r.me
	r.persistState()
	termBeforeRPC := r.currentTerm
	var cnt int32 = 1 //vote for self already
	ids := make([]int, 0, r.clusterSize)
	for i := 1; i <= 3; i++ {
		if i != r.me {
			ids = append(ids, i)
		}
	}
	for _, id := range ids {
		go func(target int) {
			msg := fmt.Sprintf("Requesting vote from node %d", target)
			r.logPut(msg, MAGENTA)
			if gotVoted := r.sendRequestVote(target); gotVoted {
				msg := fmt.Sprintf("Received vote from node %d", target)
				r.logPut(msg, CYAN)
				atomic.AddInt32(&cnt, 1)
			}
		}(id)
	}
	time.Sleep(COMMUNICATION_LATENCY)
	if atomic.LoadInt32(&cnt) > r.clusterSize/2 && r.state == CANDIDATE && termBeforeRPC == r.currentTerm {
		msg := fmt.Sprintf("Won election  with %d votes, becoming leader", cnt)
		r.logPut(msg, GREEN)
		r.state = LEADER
		// Here you would add code to start sending heartbeats to other nodes
	} else {
		msg := fmt.Sprintf("Lost election with only %d votes, reverting to follower", cnt)
		r.logPut(msg, RED)
		msg = fmt.Sprintf("Current connection is %d", r.rpcConns)
		r.logPut(msg, BLUE)
		r.state = FOLLOWER
	}
}
