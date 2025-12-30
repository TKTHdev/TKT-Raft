package main

func (r *Raft) handleClientRequest() {
	for {
		command := <-r.ClientCh
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
}
