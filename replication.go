package main

import (
	"fmt"
	"time"
)

func (r *Raft) handleClientRequest() {
	const batchSize = 100
	const lingerTime = 10 * time.Millisecond

	for {
		var reqs []ClientRequest
		select {
		case req := <-r.ReqCh:
			reqs = append(reqs, req)
		}

		timer := time.NewTimer(lingerTime)
	loop:
		for len(reqs) < batchSize {
			select {
			case req := <-r.ReqCh:
				reqs = append(reqs, req)
			case <-timer.C:
				break loop
			}
		}
		timer.Stop()

		if len(reqs) > 0 {
			r.appendEntriesToLog(reqs)
		}
	}
}

func (r *Raft) appendToLog(command []byte) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	log := LogEntry{
		Command: command,
		Term:    r.currentTerm,
	}
	r.log = append(r.log, log)
	index := len(r.log) - 1
	if err := r.storage.AppendEntry(log); err != nil {
		fmt.Printf("Error appending to log storage: %v\n", err)
	}
	return index
}

func (r *Raft) appendEntriesToLog(reqs []ClientRequest) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var logs []LogEntry
	startLogIndex := len(r.log)

	for _, req := range reqs {
		entry := LogEntry{
			Command: req.Command,
			Term:    r.currentTerm,
		}
		logs = append(logs, entry)
		r.log = append(r.log, entry)
	}

	if err := r.storage.AppendEntries(logs); err != nil {
		fmt.Printf("Error appending to log storage: %v\n", err)
	}

	for i, req := range reqs {
		r.pendingResponses[startLogIndex+i] = req.RespCh
	}

	select {
	case r.newLogEntryCh <- true:
	default:
	}
}
