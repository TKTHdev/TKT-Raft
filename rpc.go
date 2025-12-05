package main

const (
	AppendEntries = "Raft.AppendEntries"
	RequestVote   = "Raft.RequestVote"
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
	if args.Term < r.currentTerm {
		reply.Term = r.currentTerm
		reply.Success = false
		return nil
	}

}

func (r *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) error {
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
