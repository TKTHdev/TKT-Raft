package main

import "fmt"

func (r *Raft) handleClientRequest() {
	for {
		command := <-r.ReqCh
		r.appendToLog(command)
	}
}

func (r *Raft) appendToLog(command []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	log := LogEntry{
		Command: command,
		Term:    r.currentTerm,
	}
	r.log = append(r.log, log)
	if err := r.storage.AppendEntry(log); err != nil {
		fmt.Printf("Error appending to log storage: %v\n", err)
	}
}
