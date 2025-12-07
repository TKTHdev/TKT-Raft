package main

import (
	"fmt"
	"log"
	"math/rand"
	"sync/atomic"
	"time"
)

const (
	MINELECTION_TIMEOUT   = 200 * time.Millisecond
	MAXELECTION_TIMEOUT   = 1200 * time.Millisecond
	COMMUNICATION_LATENCY = 50 * time.Millisecond
)

func (r *Raft) Run() {
	for {
		switch r.state {
		case FOLLOWER:
			if err := r.doFollower(); err != nil {
				r.logPut("I am a follower", GREEN)
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
	// send heartbeats to all followers
	r.mu.Lock()
	ids := make([]int, 0, len(r.rpcConns))
	for id := range r.rpcConns {
		ids = append(ids, id)
	}
	r.mu.Unlock()
	for _, id := range ids {
		if id != r.me {
			msg := fmt.Sprintf("Sending heartbeat to node %d", id)
			r.logPut(msg, WHITE)
			go r.sendAppendEntries(id)
		}
	}
	time.Sleep(COMMUNICATION_LATENCY)
	return nil
}

func (r *Raft) startElection() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state = CANDIDATE
	r.currentTerm++
	r.votedFor = r.me
	var cnt int32 = 1 //vote for self already
	// Send RequestVote RPCs to other nodes (copy keys under lock)
	ids := make([]int, 0, len(r.rpcConns))
	for id := range r.rpcConns {
		ids = append(ids, id)
	}
	for _, id := range ids {
		go func(target int) {
			msg := fmt.Sprintf("Requesting vote from node %d", target)
			r.logPut(msg, BLUE)
			if gotVoted := r.sendRequestVote(target); gotVoted {
				msg := fmt.Sprintf("Received vote from node %d", target)
				r.logPut(msg, BLUE)
				atomic.AddInt32(&cnt, 1)
			}
		}(id)
	}
	time.Sleep(COMMUNICATION_LATENCY)
	if atomic.LoadInt32(&cnt) > r.clusterSize/2 {
		msg := fmt.Sprintf("Won election  with %d votes, becoming leader", cnt)
		r.logPut(msg, YELLOW)
		r.state = LEADER
		// Here you would add code to start sending heartbeats to other nodes
	} else {
		msg := fmt.Sprintf("Lost election with only %d votes, reverting to follower", cnt)
		r.logPut(msg, RED)
		r.state = FOLLOWER
	}
}
