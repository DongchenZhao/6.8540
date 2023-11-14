package raft

import (
	"fmt"
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

	// 2d: 执行到此处，发现leader因为快照过快而无法匹配日志，比如可能因为在请求-回复过程中由于网络延迟太久，leader发生快照
	// 冷处理，会直接冲突

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
		log[i].Index = rf.log[i].Index
	}
	nextIndex := make([]int, len(rf.nextIndex))
	copy(nextIndex, rf.nextIndex)
	snapShotIndex := rf.snapshotIndex
	snapShotTerm := rf.snapshotTerm
	// fixed 2d：并发，不长记性
	snapshot := make([]byte, len(rf.snapshot))
	copy(snapshot, rf.snapshot)
	rf.mu.RUnlock()

	for i := 0; i < len(rf.peers); i++ {
		curI := i
		go func() {
			if curI == rf.me {
				return
			}

			// 获取要发给每个Server的prevLogTerm, prevLogIndex和Entries[]
			//prevLogIndex := nextIndex[curI] - 1
			//prevLogTerm := 0
			//if prevLogIndex != -1 {
			//	prevLogTerm = log[prevLogIndex].Term
			//}
			//entries := make([]LogEntry, len(log)-(prevLogIndex+1))
			//for j := 0; j < len(log)-(prevLogIndex+1); j++ {
			//	entries[j].Term = log[j+prevLogIndex+1].Term
			//	entries[j].Command = log[j+prevLogIndex+1].Command
			//}

			// 获取要发给每个Server的prevLogTerm, prevLogIndex和Entries[]
			prevLogIndex := nextIndex[curI] - 1
			prevLogTerm := 0
			var entries []LogEntry

			if prevLogIndex < snapShotIndex { // leader因为快照，日志长度不够，转而发送install snapshot
				// 发送installSnapshot RPC
				installSnapshotArgs := InstallSnapshotArgs{currentTerm, rf.me, snapShotIndex, snapShotTerm, snapshot}
				installSnapshotReply := InstallSnapshotReply{}
				rf.PrintLog(fmt.Sprintf("IS RPC ---> [Server "+strconv.Itoa(curI)+"], [Leader term %d], [Leader Id %d], [LastIncludedIndex %d], [LastIncludedTerm %d]", installSnapshotArgs.Term, installSnapshotArgs.LeaderId, installSnapshotArgs.LastIncludedIndex, installSnapshotArgs.LastIncludedTerm), "blue")
				ok := rf.sendInstallSnapshot(curI, &installSnapshotArgs, &installSnapshotReply)
				if !ok {
					// rf.PrintLog("IS RPC ---> [Server "+strconv.Itoa(curI)+"] Failed", "yellow")
				}
				rf.InstallSnapshotResponseHandler(&installSnapshotArgs, &installSnapshotReply)
				return
			} else if prevLogIndex == snapShotIndex { // leader发送的entries刚好从leader当前log开始，把leader的log全部发过去
				prevLogTerm = snapShotTerm
				entries = make([]LogEntry, len(log))
				for j := 0; j < len(log); j++ {
					entries[j] = LogEntry{Term: log[j].Term, Command: log[j].Command, Index: log[j].Index}
				}
			} else { // 遍历日志串，查找prevLogIndex对应位置
				actualIndex := 0
				found := false
				for ; actualIndex < len(log); actualIndex++ {
					if log[actualIndex].Index == prevLogIndex {
						found = true
						break
					}
				}
				if !found {
					panic("should found prevLogIndex")
				}
				actualIndex += 1
				prevLogTerm = log[actualIndex-1].Term
				entries = make([]LogEntry, len(log)-actualIndex)
				for j := 0; j < len(log)-actualIndex; j++ {
					entries[j] = LogEntry{Term: log[actualIndex+j].Term, Command: log[actualIndex+j].Command, Index: log[actualIndex+j].Index}
				}
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
