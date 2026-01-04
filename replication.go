package main

import "fmt"

func (r *Raft) handleClientRequest() {
	for {
		req := <-r.ReqCh
		index := r.appendToLog(req.Command)
		r.mu.Lock()
		r.pendingResponses[index] = req.RespCh
		r.mu.Unlock()
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
