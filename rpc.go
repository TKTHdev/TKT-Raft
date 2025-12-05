package main

const (
	AppendEntries = "Raft.AppendEntries"
	RequestVote   = "Raft.RequestVote"
)

const (
	NOTVOTED = -2
	NOLEADER = -1
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

func (r *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) error {
	//1. Reply false if term < currentTerm
	if args.Term < r.currentTerm {
		reply.Term = r.currentTerm
		reply.Success = false
		return nil
	}
	//2. Reply false if log doesn't contain an entry at prevLogIndex whose term matches prevLogTerm
	if args.PrevLogIndex >= len(r.log) || r.log[args.PrevLogIndex].Term != args.PrevLogTerm {
		reply.Term = r.currentTerm
		reply.Success = false
		return nil
	}
	//3. If an existing entry conflicts with a new one (same index but different terms), delete the existing entry and all that follow it
	for i, entry := range args.Entries {
		logIndex := args.PrevLogIndex + 1 + i
		if logIndex < len(r.log) {
			if r.log[logIndex].Term != entry.Term {
				r.log = r.log[:logIndex]
				break
			}
		}
	}
	//4. Append any new entries not already in the log
	for i, entry := range args.Entries {
		logIndex := args.PrevLogIndex + 1 + i
		if logIndex >= len(r.log) {
			r.log = append(r.log, entry)
		}
	}
	//5. If leaderCommit > commitIndex, set commitIndex = min(leaderCommit, index of last new entry)
	if args.LeaderCommit > r.commitIndex {
		lastNewEntryIndex := args.PrevLogIndex + len(args.Entries)
		if args.LeaderCommit < lastNewEntryIndex {
			r.commitIndex = args.LeaderCommit
		} else {
			r.commitIndex = lastNewEntryIndex
		}
	}
	reply.Term = r.currentTerm
	reply.Success = true
	return nil
}

func (r *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) error {
	//1. Reply false if term < currentTerm
	if args.Term < r.currentTerm {
		reply.Term = r.currentTerm
		reply.VoteGranted = false
		return nil
	} else if args.Term > r.currentTerm {
		r.votedFor = NOTVOTED
		r.currentTerm = args.Term
	}

	//2. If votedFor is null or candidateId, and candidate's log is at least as up-to-date as receiver's log, grant vote
	lastLogIndex := len(r.log) - 1
	lastLogTerm := r.log[lastLogIndex].Term
	upToDate := (args.LastLogTerm > lastLogTerm) || (args.LastLogTerm == lastLogTerm && args.LastLogIndex >= lastLogIndex)
	if (r.votedFor == NOTVOTED || r.votedFor == args.CandidateID) && upToDate {
		r.votedFor = args.CandidateID
		reply.VoteGranted = true
	} else {
		reply.VoteGranted = false
	}
	reply.Term = r.currentTerm
	return nil

}

func (r *Raft) sendAppendEntries(server int) bool {
	args := &AppendEntriesArgs{
		Term:         r.currentTerm,
		LeaderID:     r.me,
		PrevLogIndex: r.nextIndex[server] - 1,
		PrevLogTerm:  r.log[r.nextIndex[server]-1].Term,
		Entries:      r.log[r.nextIndex[server]:],
		LeaderCommit: r.commitIndex,
	}
	reply := &AppendEntriesReply{}
	if err := r.rpcConns[server].Call(AppendEntries, args, reply); err != nil {
		r.rpcConns[server] = nil
		return false
	}
	r.nextIndex[server] = r.nextIndex[server] + len(args.Entries)
	return reply.Success
}

func (r *Raft) sendRequestVote(server int) bool {
	args := &RequestVoteArgs{
		Term:         r.currentTerm,
		CandidateID:  r.me,
		LastLogIndex: len(r.log) - 1,
		LastLogTerm:  r.log[len(r.log)-1].Term,
	}
	reply := &RequestVoteReply{}
	if err := r.rpcConns[server].Call(RequestVote, args, reply); err != nil {
		r.rpcConns[server] = nil
		return false
	}
	return reply.VoteGranted
}
