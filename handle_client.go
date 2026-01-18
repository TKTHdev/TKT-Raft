package main

import (
	"fmt"
	"strings"
	"time"
)

const (
	READ_LINGER_TIME  = 15 * time.Millisecond
	WRITE_LINGER_TIME = 15 * time.Millisecond
)

func (r *Raft) handleClientRequest() {
	writeBatchSize := r.writeBatchSize
	readBatchSize := r.readBatchSize

	var writeReqs []ClientRequest
	var readReqs []ClientRequest

	var writeTimer *time.Timer
	var readTimer *time.Timer
	var writeTimerCh <-chan time.Time
	var readTimerCh <-chan time.Time

	flushWrites := func() {
		if len(writeReqs) > 0 {
			r.appendEntriesToLog(writeReqs)
			writeReqs = nil
		}
	}

	flushReads := func() {
		if len(readReqs) > 0 {
			r.ReadCh <- readReqs
			readReqs = nil
		}
	}

	stopTimer := func(t *time.Timer) {
		if !t.Stop() {
			select {
			case <-t.C:
			default:
			}
		}
	}

	for {
		select {
		case req := <-r.ReqCh:
			if strings.HasPrefix(string(req.Command), "GET") {
				readReqs = append(readReqs, req)
				if len(readReqs) >= readBatchSize {
					flushReads()
					if readTimer != nil {
						stopTimer(readTimer)
						readTimer = nil
						readTimerCh = nil
					}
				} else if readTimer == nil {
					readTimer = time.NewTimer(READ_LINGER_TIME)
					readTimerCh = readTimer.C
				}
			} else {
				writeReqs = append(writeReqs, req)
				if len(writeReqs) >= writeBatchSize {
					flushWrites()
					if writeTimer != nil {
						stopTimer(writeTimer)
						writeTimer = nil
						writeTimerCh = nil
					}
				} else if writeTimer == nil {
					writeTimer = time.NewTimer(WRITE_LINGER_TIME)
					writeTimerCh = writeTimer.C
				}
			}
		case <-writeTimerCh:
			flushWrites()
			writeTimer = nil
			writeTimerCh = nil
		case <-readTimerCh:
			flushReads()
			readTimer = nil
			readTimerCh = nil
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
