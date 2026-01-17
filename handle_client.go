package main

import (
	"fmt"
	"strings"
	"time"
)

const (
	lingerTime = 5 * time.Millisecond
)

func (r *Raft) handleClientRequest() {
	writeBatchSize := r.writeBatchSize
	readBatchSize := r.readBatchSize
	for {
		var writeReqs []ClientRequest
		var readReqs []ClientRequest

		process := func(req ClientRequest) {
			if strings.HasPrefix(string(req.Command), "GET") {
				readReqs = append(readReqs, req)
			} else {
				writeReqs = append(writeReqs, req)
			}
		}

		select {
		case req := <-r.ReqCh:
			process(req)
		}

		timer := time.NewTimer(lingerTime)
	loop:
		for {
			if len(writeReqs) >= writeBatchSize || len(readReqs) >= readBatchSize {
				break loop
			}
			select {
			case req := <-r.ReqCh:
				process(req)
			case <-timer.C:
				break loop
			}
		}
		timer.Stop()

		if len(writeReqs) > 0 {
			r.appendEntriesToLog(writeReqs)
		}
		if len(readReqs) > 0 {
			r.ReadCh <- readReqs
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
