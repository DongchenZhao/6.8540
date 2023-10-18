package raft

import (
	"strconv"
	"time"
)

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
	rf.mu.Lock()
	defer rf.mu.Unlock()
	defer rf.persist()

	// 1. 自己term比leader大，拒绝leader
	if args.Term < rf.currentTerm {
		reply.Term = rf.currentTerm
		reply.Success = false
		return
	} else { // 2. 自己term更小或相等，都转为和leader相同term的follower，接受心跳，重置超时
		rf.toFollower(args.Term)
		rf.lastHeartbeatTime = time.Now().UnixMilli()
		reply.Term = args.Term
		reply.Success = true
		reply.ServerId = rf.me

		// follower进行AE RPC的log匹配
		rf.followerHandleLog(args, reply)
		if !reply.Success {
			return
		}

		// follower更新commitIndex
		// fixed: 日志匹配失败不应该更新commitIndex
		rf.followerUpdateCommitIndex(args.LeaderCommit)
		return
	}

}

func (rf *Raft) AppendEntriesResponseHandler(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	// 自己term更高，忽略过期AE RPC Resp
	if rf.currentTerm > reply.Term || rf.currentTerm > args.Term {
		return
	}

	// 对方term更高，转为Follower
	if rf.currentTerm < reply.Term {
		if reply.Success {
			rf.PrintLog("ERROR", "red")
		}
		rf.toFollower(reply.Term)
		return
	}

	// leader 处理日志agreement
	rf.leaderHandleLog(args, reply)
}

func (rf *Raft) sendAppendEntries(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	ok := rf.peers[server].Call("Raft.AppendEntriesRequestHandler", args, reply)
	return ok
}

func (rf *Raft) leaderSendAppendEntriesRPC() {
	rf.mu.RLock()
	currentTerm := rf.currentTerm
	leaderCommit := rf.commitIndex
	log := make([]LogEntry, len(rf.log))
	for i := 0; i < len(rf.log); i++ {
		log[i].Term = rf.log[i].Term
		log[i].Command = rf.log[i].Command
	}
	nextIndex := make([]int, len(rf.nextIndex))
	copy(nextIndex, rf.nextIndex)
	rf.mu.RUnlock()

	for i := 0; i < len(rf.peers); i++ {
		curI := i
		go func() {

			// rf.lockTest += 1

			if curI == rf.me {
				return
			}

			// 获取要发给每个Server的prevLogTerm, prevLogIndex和Entries[]
			prevLogIndex := nextIndex[curI] - 1
			prevLogTerm := 0
			if prevLogIndex != -1 {
				prevLogTerm = log[prevLogIndex].Term
			}
			entries := make([]LogEntry, len(log)-(prevLogIndex+1))
			for j := 0; j < len(log)-(prevLogIndex+1); j++ {
				entries[j].Term = log[j+prevLogIndex+1].Term
				entries[j].Command = log[j+prevLogIndex+1].Command
			}

			appendEntriesArgs := AppendEntriesArgs{currentTerm, rf.me, prevLogIndex, prevLogTerm, entries, leaderCommit}
			appendEntriesReply := AppendEntriesReply{}

			rf.PrintLog("AE RPC -----> [Server "+strconv.Itoa(curI)+"], "+getAppendEntriesRPCStr(&appendEntriesArgs, &appendEntriesReply), "purple")
			ok := rf.sendAppendEntries(curI, &appendEntriesArgs, &appendEntriesReply)
			if !ok {
				// rf.PrintLog("AE RPC -----> [Server "+strconv.Itoa(curI)+"] Failed", "yellow")
			}
			// 处理心跳包的reply
			rf.AppendEntriesResponseHandler(&appendEntriesArgs, &appendEntriesReply)
		}()
	}
}
