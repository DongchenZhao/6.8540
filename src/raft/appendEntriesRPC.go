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

	if args.Term < rf.currentTerm { // 1. 自己term比leader大，拒绝leader
		reply.Term = rf.currentTerm
		reply.Success = false
		return
	} else {
		if args.Term > rf.currentTerm { // 2. 自己term比leader小，转follower以刷新term
			rf.toFollower(args.Term)
		}
		// 3. 自己term和leader相等，比较日志
		rf.lastHeartbeatTime = time.Now().UnixMilli()
		reply.Term = args.Term
		reply.Success = true
		// TODO compare and handle log
		return
	}
}

func (rf *Raft) AppendEntriesResponseHandler(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	if reply.Term > rf.currentTerm && (!reply.Success) {
		rf.toFollower(reply.Term)
	}
	// TODO 处理日志agreement
}

func (rf *Raft) sendAppendEntries(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	ok := rf.peers[server].Call("Raft.AppendEntriesRequestHandler", args, reply)
	return ok
}

func (rf *Raft) leaderSendAppendEntriesRPC() {
	rf.mu.RLock()
	currentTerm := rf.currentTerm
	log := make([]LogEntry, len(rf.log))
	for i := 0; i < len(rf.log); i++ {
		log[i].Term = rf.log[i].Term
		log[i].Command = rf.log[i].Command
	}
	leaderCommit := rf.commitIndex
	rf.mu.RUnlock()

	for i := 0; i < len(rf.peers); i++ {
		curI := i
		go func() {
			if curI == rf.me {
				return
			}
			//TODO 获取要发给每个Server的prevLogTerm, prevLogIndex和Entries[]
			prevLogIndex := -1
			prevLogTerm := -1
			entries := make([]LogEntry, 0)

			appendEntriesArgs := AppendEntriesArgs{currentTerm, rf.me, prevLogIndex, prevLogTerm, entries, leaderCommit}
			appendEntriesReply := AppendEntriesReply{}

			entriesStr := getLogStr(entries)

			rf.PrintLog("AE RPC -----> [Server "+strconv.Itoa(curI)+"], "+"[Term "+strconv.Itoa(currentTerm)+"]"+" [prevLogIndex "+strconv.Itoa(prevLogIndex)+"]"+" [prevLogTerm"+strconv.Itoa(prevLogTerm)+"]"+" [LeaderCommit"+strconv.Itoa(leaderCommit)+"]"+" [Entries"+entriesStr+"]", "purple")
			ok := rf.sendAppendEntries(curI, &appendEntriesArgs, &appendEntriesReply)
			if !ok {
				// rf.PrintLog("AE RPC -----> [Server "+strconv.Itoa(curI)+"] Failed", "yellow")
			}
			// 处理心跳包的reply
			rf.AppendEntriesResponseHandler(&appendEntriesArgs, &appendEntriesReply)
		}()
	}
}
