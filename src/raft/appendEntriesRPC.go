package raft

type AppendEntriesArgs struct {
	Term         int
	LeaderId     int
	PrevLogIndex int
	PrevLogTerm  int
	Entries      []LogEntry
	LeaderCommit int
}

type AppendEntriesReply struct {
	Term     int
	Success  bool
	ServerId int
	XTerm    int
	XIndex   int
	XLen     int
}

func (rf *Raft) AppendEntriesRequestHandler(args *AppendEntriesArgs, reply *AppendEntriesReply) {

}

func (rf *Raft) appendEntriesResponseHandler(args *AppendEntriesArgs, reply *AppendEntriesReply) {

}

func (rf *Raft) sendAppendEntries(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	ok := rf.peers[server].Call("Raft.AppendEntriesRequestHandler", args, reply)
	return ok
}

func (rf *Raft) leaderSendAppendEntriesRPC() {

}
