package main

func (r *Raft) runReplication() {
	for {
		command := <-r.ClientCh
		r.mu.Lock()
		log := LogEntry{
			Command: command,
			Term:    r.currentTerm,
		}
		r.log = append(r.log, log)
		r.mu.Unlock()
	}
}
