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
	rf.mu.Lock()
	defer rf.mu.Unlock()
	defer rf.persist()

	// 0.1 自己term小，转为follower(并重置timeout[删除])，然后继续研判candidate的投票请求
	if rf.currentTerm < args.Term {
		rf.PrintLog(fmt.Sprintf("cur server has lower term. Server term: [Term %d], Candidate term: [Term %d]", rf.currentTerm, args.Term), "default")
		rf.toFollower(args.Term)
		// rf.lastHeartbeatTime = time.Now().UnixMilli() 这里重置时钟是不必要的，votedFor重置为-1，接下来grantVote的时候自然会重置时钟
	}

	// 0.2 自己是leader，冷处理
	if rf.role == 2 {
		reply.VoteGranted = false
		reply.Term = rf.currentTerm
	}

	// 1. 如果自己term更大，告诉Candidate false，让对方转为follower
	if rf.currentTerm > args.Term {
		rf.PrintLog(fmt.Sprintf("RV RPC Resp ------> [Candidate %d] REJECT due to a higher term. [Server Term %d], [Candidate Term %d]", args.CandidateId, rf.currentTerm, args.Term), "default")
		reply.Term = rf.currentTerm
		reply.VoteGranted = false
		return
	}

	// 2.1 已有votedFor，礼貌拒绝
	// votedFor == CandidateId可以继续，考虑persist
	// TODO 当前rf如何知道自己是否会重复投票，其实应该无所谓，candidate只发送了1次投票请求
	// if rf.votedFor != -1 && rf.votedFor != args.CandidateId {
	if rf.votedFor != -1 {
		rf.PrintLog(fmt.Sprintf("RV RPC Resp ------> [Candidate %d] REJECT due to already voted. [Server Term %d], [VotedFor: %d], [Candidate Term %d]", args.CandidateId, rf.currentTerm, rf.votedFor, args.Term), "default")
		reply.Term = rf.currentTerm
		reply.VoteGranted = false
		return
	}

	// 2.2 election restriction
	lastLogIndex := len(rf.log) - 1
	lastLogTerm := 0
	if lastLogIndex != -1 {
		lastLogTerm = rf.log[lastLogIndex].Term
	}
	if lastLogTerm > args.LastLogTerm || (lastLogTerm == args.LastLogTerm && lastLogIndex > args.LastLogIndex) {
		rf.PrintLog(fmt.Sprintf("RV RPC Resp ------> [Candidate %d] REJECT due to election restriction. [Term %d], [VotedFor: %d], [lastLogTerm %d], [lastLogIndex %d], [Candidate Term %d] [Candidate lastLogTerm %d], [Candidate lastLogIndex %d]", args.CandidateId, rf.currentTerm, rf.votedFor, lastLogTerm, lastLogIndex, args.Term, args.LastLogTerm, args.LastLogIndex), "default")
		reply.Term = rf.currentTerm
		reply.VoteGranted = false
		return
	}

	// 3. 批准投票，更新时钟
	rf.lastHeartbeatTime = time.Now().UnixMilli()
	rf.votedFor = args.CandidateId
	reply.Term = rf.currentTerm
	reply.VoteGranted = true
	rf.PrintLog(fmt.Sprintf("RV RPC Resp ------> [Candidate %d] APPROVED, [Candidate Term %d]", args.CandidateId, args.Term), "default")
}

// RequestVoteResponseHandler Candidate用于处理投票响应
func (rf *Raft) RequestVoteResponseHandler(args *RequestVoteArgs, reply *RequestVoteReply) {
	rf.mu.Lock()
	// 1. 确保自己仍然是Candidate
	if rf.role != 1 {
		rf.mu.Unlock()
		return
	}

	// 2.忽略过期resp
	if rf.currentTerm > reply.Term || rf.currentTerm > args.Term {
		rf.mu.Unlock()
		return
	}

	// 3.自己term比对方低
	if rf.currentTerm < reply.Term {
		rf.toFollower(reply.Term)
		rf.mu.Unlock()
		return
	}

	// 4.批准投票
	if reply.VoteGranted {
		rf.voteCnt += 1
		rf.PrintLog(fmt.Sprintf("RV RPC GRANTED from [Server ?], [VoteCnt %d]", rf.voteCnt), "default")
		if rf.voteCnt >= (len(rf.peers)/2 + 1) {
			rf.mu.Unlock()
			rf.toLeader()
			return
		}
	}

	// 5.其他情况，比如对方已投票，不管
	rf.mu.Unlock()
}

func (rf *Raft) sendRequestVote(server int, args *RequestVoteArgs, reply *RequestVoteReply) bool {
	ok := rf.peers[server].Call("Raft.RequestVoteRequestHandler", args, reply)
	return ok
}
