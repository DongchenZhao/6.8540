package raft

import (
	"fmt"
	"time"
)

type RequestVoteArgs struct {
	Term         int
	CandidateId  int
	LastLogIndex int
	LastLogTerm  int
}

type RequestVoteReply struct {
	Term        int
	VoteGranted bool
}

// RequestVoteRequestHandler 处理Candidate发来的投票请求
func (rf *Raft) RequestVoteRequestHandler(args *RequestVoteArgs, reply *RequestVoteReply) {
	rf.mu.RUnlock()
	currentTerm := rf.currentTerm
	votedFor := rf.votedFor
	role := rf.role
	lastLogIndex := len(rf.log) - 1
	lastLogTerm := 0
	if lastLogIndex != -1 {
		lastLogTerm = rf.log[lastLogIndex].Term
	}
	rf.mu.RLock()

	// 0.1 自己term小，转为follower，然后继续研判candidate的投票请求
	if currentTerm < args.Term {
		rf.PrintLog(fmt.Sprintf("cur server has lower term. Server term: [Term %d], Candidate term: [Term %d]", currentTerm, args.Term), "default")
		rf.toFollower(args.Term)
	}

	// 0.2 自己是leader，冷处理
	if role == 2 {
		reply.VoteGranted = false
		reply.Term = currentTerm
	}

	// 1. 如果自己term更大，告诉Candidate false
	if currentTerm > args.Term {
		rf.PrintLog(fmt.Sprintf("RV RPC Resp ------> [Candidate %d] REJECT due to a higher term. [Server Term %d], [Candidate Term %d]", args.CandidateId, currentTerm, args.Term), "default")
		reply.Term = currentTerm
		reply.VoteGranted = false
		return
	}

	// 2.1 已有votedFor
	// votedFor == CandidateId可以继续，考虑persist
	if votedFor != -1 && votedFor != args.CandidateId {
		rf.PrintLog(fmt.Sprintf("RV RPC Resp ------> [Candidate %d] REJECT due to already voted. [Server Term %d], [VotedFor: %d], [Candidate Term %d]", args.CandidateId, currentTerm, votedFor, args.Term), "default")
		reply.Term = currentTerm
		reply.VoteGranted = false
		return
	}

	// 2.2 election restriction
	if lastLogIndex > args.LastLogIndex || (lastLogTerm == args.LastLogTerm && lastLogIndex > args.LastLogIndex) {
		rf.PrintLog(fmt.Sprintf("RV RPC Resp ------> [Candidate %d] REJECT due to election restriction. [Term %d], [VotedFor: %d], [lastLogTerm %d], [lastLogIndex %d], [Candidate Term %d] [Candidate lastLogTerm %d], [Candidate lastLogIndex %d]", args.CandidateId, currentTerm, votedFor, lastLogTerm, lastLogIndex, args.Term, args.LastLogTerm, args.LastLogIndex), "default")
		reply.Term = currentTerm
		reply.VoteGranted = false
		return
	}

	// 3. 批准投票，更新时钟
	rf.mu.Lock()
	rf.lastHeartbeatTime = time.Now().UnixMilli()
	rf.votedFor = args.CandidateId
	rf.mu.Unlock()

	reply.Term = currentTerm
	reply.VoteGranted = true
	rf.PrintLog(fmt.Sprintf("RV RPC Resp ------> [Candidate %d] APPROVED, [Candidate Term %d]", args.CandidateId, args.Term), "default")
}

// RequestVoteResponseHandler Candidate用于处理投票响应
func (rf *Raft) RequestVoteResponseHandler(args *RequestVoteArgs, reply *RequestVoteReply) {
	rf.mu.RLock()
	role := rf.role
	currentTerm := rf.currentTerm
	rf.mu.RUnlock()

	// 1. 确保自己仍然是Candidate
	if role != 1 {
		return
	}

	// 2.忽略过期resp
	if currentTerm > reply.Term {
		return
	}

	// 3.自己term比对方低
	if currentTerm < reply.Term {
		rf.toFollower(reply.Term)
		return
	}

	// 4.批准投票
	if reply.VoteGranted {
		rf.mu.Lock()
		rf.voteCnt += 1
		rf.PrintLog(fmt.Sprintf("receive granted RV RPC from [Server ?], [VoteCnt %d]", rf.voteCnt), "default")
		if rf.voteCnt >= (len(rf.peers)/2 + 1) {
			rf.mu.Unlock()
			rf.toLeader()
		} else {
			rf.mu.Unlock()
		}
	}

	// 5.其他情况，比如对方已投票，不管
}

// example code to send a RequestVote RPC to a server.
// server is the index of the target server in rf.peers[].
// expects RPC arguments in args.
// fills in *reply with RPC reply, so caller should
// pass &reply.
// the types of the args and reply passed to Call() must be
// the same as the types of the arguments declared in the
// handler function (including whether they are pointers).
//
// The labrpc package simulates a lossy network, in which servers
// may be unreachable, and in which requests and replies may be lost.
// Call() sends a request and waits for a reply. If a reply arrives
// within a timeout interval, Call() returns true; otherwise
// Call() returns false. Thus Call() may not return for a while.
// A false return can be caused by a dead server, a live server that
// can't be reached, a lost request, or a lost reply.
//
// Call() is guaranteed to return (perhaps after a delay) *except* if the
// handler function on the server side does not return.  Thus there
// is no need to implement your own timeouts around Call().
//
// look at the comments in ../labrpc/labrpc.go for more details.
//
// if you're having trouble getting RPC to work, check that you've
// capitalized all field names in structs passed over RPC, and
// that the caller passes the address of the reply struct with &, not
// the struct itself.
func (rf *Raft) sendRequestVote(server int, args *RequestVoteArgs, reply *RequestVoteReply) bool {
	ok := rf.peers[server].Call("Raft.RequestVoteRequestHandler", args, reply)
	return ok
}
