package main

import "fmt"

const (
	AppendEntries = "Raft.AppendEntries"
	RequestVote   = "Raft.RequestVote"
	Read          = "Raft.Read"
)

const (
	NOTVOTED = -2
)

type AppendEntriesArgs struct {
	Term         int
	LeaderID     int
	PrevLogIndex int
	PrevLogTerm  int
	Entries      []LogEntry
	LeaderCommit int
}

type AppendEntriesReply struct {
	Term    int
	Success bool
}

type RequestVoteArgs struct {
	Term         int
	CandidateID  int
	LastLogIndex int
	LastLogTerm  int
}

type RequestVoteReply struct {
	Term        int
	VoteGranted bool
}

type ReadArgs struct {
	Term     int
	LeaderID int
}

type ReadReply struct {
	Term    int
	Success bool
}

func (r *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	//0. If term > currentTerm, set currentTerm = term, convert to follower
	if r.currentTerm < args.Term {
		r.currentTerm = args.Term
		r.state = FOLLOWER
		r.votedFor = NOTVOTED
		r.persistState()
	}
	//1. Reply false if term < currentTerm
	if args.Term < r.currentTerm {
		reply.Term = r.currentTerm
		reply.Success = false
		return nil
	}
	//2. Reply false if log doesn't contain an entry at prevLogIndex whose term matches prevLogTerm
	if len(r.log) <= args.PrevLogIndex {
		reply.Term = r.currentTerm
		reply.Success = false
		r.heartBeatCh <- true
		return nil
	}
	if r.log[args.PrevLogIndex].Term != args.PrevLogTerm {
		reply.Term = r.currentTerm
		reply.Success = false
		r.heartBeatCh <- true
		return nil
	}

	//3. If an existing entry conflicts with a new one (same index but different terms), delete the existing entry and all that follow it
	for i, entry := range args.Entries {
		logIndex := args.PrevLogIndex + 1 + i
		if logIndex < len(r.log) {
			if r.log[logIndex].Term != entry.Term {
				r.log = r.log[:logIndex]
				// storage index is logIndex - 1 because r.log has dummy entry at 0
				if err := r.storage.TruncateLog(logIndex - 1); err != nil {
					fmt.Printf("Error truncating log: %v\n", err)
				}
				break
			}
		}
	}
	//4. Append any new entries not already in the log
	var newEntries []LogEntry
	for i, entry := range args.Entries {
		logIndex := args.PrevLogIndex + 1 + i
		if len(r.log) <= logIndex {
			r.log = append(r.log, entry)
			newEntries = append(newEntries, entry)
		}
	}
	if len(newEntries) > 0 {
		if err := r.storage.AppendEntries(newEntries); err != nil {
			fmt.Printf("Error appending entries: %v\n", err)
		}
	}
	//5. If leaderCommit > commitIndex, set commitIndex = min(leaderCommit, index of last new entry)
	if r.commitIndex < args.LeaderCommit {
		lastNewEntryIndex := args.PrevLogIndex + len(args.Entries)
		if args.LeaderCommit < lastNewEntryIndex {
			r.commitIndex = args.LeaderCommit
		} else {
			r.commitIndex = lastNewEntryIndex
		}
		r.commitCond.Broadcast()
	}
	reply.Term = r.currentTerm
	reply.Success = true
	r.heartBeatCh <- true
	return nil
}

func (r *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logPutLocked("Received RequestVote RPC", CYAN)
	//0. If term > currentTerm, set currentTerm = term, convert to follower
	//1. Reply false if term < currentTerm
	if args.Term < r.currentTerm {
		reply.Term = r.currentTerm
		reply.VoteGranted = false
		return nil
	} else if r.currentTerm < args.Term {
		r.votedFor = NOTVOTED
		r.currentTerm = args.Term
		r.state = FOLLOWER
	}

	if r.currentTerm == args.Term && r.votedFor != NOTVOTED && r.votedFor != args.CandidateID {
		reply.Term = r.currentTerm
		reply.VoteGranted = false
		return nil
	}

	//2. If votedFor is null or candidateId, and candidate's log is at least as up-to-date as receiver's log, grant vote
	lastLogIndex := len(r.log) - 1
	lastLogTerm := r.log[lastLogIndex].Term
	upToDate := (lastLogTerm < args.LastLogTerm) || (args.LastLogTerm == lastLogTerm && lastLogIndex <= args.LastLogIndex)
	if (r.votedFor == NOTVOTED || r.votedFor == args.CandidateID) && upToDate {
		r.votedFor = args.CandidateID
		r.state = FOLLOWER
		reply.VoteGranted = true
	} else {
		reply.VoteGranted = false
	}
	reply.Term = r.currentTerm
	return nil

}

func (r *Raft) Read(args *ReadArgs, reply *ReadReply) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if args.Term < r.currentTerm {
		reply.Term = r.currentTerm
		reply.Success = false
		return nil
	}
	if args.Term > r.currentTerm {
		r.currentTerm = args.Term
		r.state = FOLLOWER
		r.votedFor = NOTVOTED
		r.persistState()
	}

	reply.Term = r.currentTerm
	reply.Success = true
	r.heartBeatCh <- true
	return nil
}

func (r *Raft) sendAppendEntries(server int) bool {
	r.mu.Lock()
	if r.rpcConns[server] == nil {
		r.mu.Unlock()
		r.dialRPCToPeer(server)
		return false
	}
	client := r.rpcConns[server]
	prevLogIndex := r.nextIndex[server] - 1
	entriesRaw := r.log[r.nextIndex[server]:]
	entries := make([]LogEntry, len(entriesRaw))
	copy(entries, entriesRaw)

	args := &AppendEntriesArgs{
		Term:         r.currentTerm,
		LeaderID:     r.me,
		PrevLogIndex: prevLogIndex,
		PrevLogTerm:  r.log[prevLogIndex].Term,
		Entries:      entries,
		LeaderCommit: r.commitIndex,
	}
	r.mu.Unlock()

	reply := &AppendEntriesReply{}
	if err := client.Call(AppendEntries, args, reply); err != nil {
		r.mu.Lock()
		logMsg := fmt.Sprintf("Error sending AppendEntries RPC to node %d: %v", server, err)
		r.logPutLocked(logMsg, PURPLE)
		r.rpcConns[server] = nil
		r.mu.Unlock()
		r.dialRPCToPeer(server)
		return false
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.state != LEADER || r.currentTerm != args.Term {
		return false
	}

	if reply.Success {
		r.nextIndex[server] = args.PrevLogIndex + len(args.Entries) + 1
		r.matchIndex[server] = r.nextIndex[server] - 1
		r.updateCommitIndex()
	} else {
		r.nextIndex[server] = max(1, r.nextIndex[server]-1)
	}
	if r.currentTerm < reply.Term {
		r.currentTerm = reply.Term
		r.state = FOLLOWER
		r.votedFor = NOTVOTED
	}
	return reply.Success
}

func (r *Raft) sendRequestVote(server int) bool {
	r.mu.Lock()
	if r.rpcConns[server] == nil {
		r.mu.Unlock()
		r.dialRPCToPeer(server)
		return false
	}
	client := r.rpcConns[server]
	args := &RequestVoteArgs{
		Term:         r.currentTerm,
		CandidateID:  r.me,
		LastLogIndex: len(r.log) - 1,
		LastLogTerm:  r.log[len(r.log)-1].Term,
	}
	r.mu.Unlock()

	reply := &RequestVoteReply{}
	if err := client.Call(RequestVote, args, reply); err != nil {
		r.mu.Lock()
		logMsg := fmt.Sprintf("Error sending RequestVote RPC to node %d: %v", server, err)
		r.logPutLocked(logMsg, PURPLE)
		r.rpcConns[server] = nil
		r.mu.Unlock()
		r.dialRPCToPeer(server)
		return false
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.state != CANDIDATE || r.currentTerm != args.Term {
		return false
	}

	if r.currentTerm < reply.Term {
		r.currentTerm = reply.Term
		r.state = FOLLOWER
		r.votedFor = NOTVOTED
	}
	return reply.VoteGranted
}

func (r *Raft) sendReadRPC(server int) bool {
	r.mu.Lock()
	if r.rpcConns[server] == nil {
		r.mu.Unlock()
		r.dialRPCToPeer(server)
		return false
	}
	client := r.rpcConns[server]
	args := &ReadArgs{
		Term:     r.currentTerm,
		LeaderID: r.me,
	}
	r.mu.Unlock()

	reply := &ReadReply{}
	if err := client.Call(Read, args, reply); err != nil {
		r.mu.Lock()
		logMsg := fmt.Sprintf("Error sending Read RPC to node %d: %v", server, err)
		r.logPutLocked(logMsg, PURPLE)
		r.rpcConns[server] = nil
		r.mu.Unlock()
		r.dialRPCToPeer(server)
		return false
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.currentTerm < reply.Term {
		r.currentTerm = reply.Term
		r.state = FOLLOWER
		r.votedFor = NOTVOTED
		r.persistState()
	}

	return reply.Success
}