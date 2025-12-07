package main

func (r *Raft) runReplication() {
	for {
		command := <-r.ClientCh
		log := LogEntry{
			Command: command,
			Term:    r.currentTerm,
		}
		r.log = append(r.log, log)
	}
}
