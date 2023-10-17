package raft

import (
	"fmt"
	"sort"
)

// locked，此方法执行前已获得锁，执行后caller会释放锁
func (rf *Raft) followerHandleLog(args *AppendEntriesArgs, reply *AppendEntriesReply) {

	// 1. 日志长度不够
	if len(rf.log)-1 < args.PrevLogIndex {
		reply.XTerm, reply.XIndex, reply.XLen = -1, len(rf.log), len(rf.log)
		reply.Success = false
		rf.PrintLog(fmt.Sprintf("log comapre FAILED, EMPTY ENTRY at PrevLogIndex, "+getAppendEntriesRPCStr(args, reply)), "purple")
		rf.PrintRfLog()
		return
	}

	// 2. 日志冲突，截断，XIndex为冲突日志串的第一个entry的index，XTerm即为冲突的Term
	// PrevLogIndex为-1的时候一定匹配成功
	if args.PrevLogIndex != -1 && rf.log[args.PrevLogIndex].Term != args.PrevLogTerm {
		tarTerm := rf.log[args.PrevLogIndex].Term
		startIndex := 0
		for ; startIndex <= args.PrevLogIndex; startIndex++ {
			if rf.log[startIndex].Term == tarTerm {
				break
			}
		}

		// 截断日志
		rf.log = rf.log[:args.PrevLogIndex]
		reply.XTerm, reply.XIndex, reply.XLen = tarTerm, startIndex, len(rf.log)
		reply.Success = false
		rf.PrintLog(fmt.Sprintf("log comapre FAILED, CONFLICT at PrevLogIndex, "+getAppendEntriesRPCStr(args, reply)), "purple")
		rf.PrintRfLog()
		return
	}

	// 3. 匹配成功
	if args.PrevLogIndex == -1 || rf.log[args.PrevLogIndex].Term == args.PrevLogTerm {
		// append arg中所有尚未append的日志(从PrevLogIndex开始，长度比较)
		// 同一个Term下的AE RPC，不可能有先后2个RPC在同一prevLogIndex位置加了长度相同、内容不同的entries
		if (len(rf.log)-1)-args.PrevLogIndex < len(args.Entries) {
			rf.log = append(rf.log[:args.PrevLogIndex+1], args.Entries...)
		}
		reply.XTerm, reply.XIndex, reply.XLen = -1, -1, len(rf.log)
		reply.Success = true
		rf.PrintLog(fmt.Sprintf("log comapre SUCCESS, "+getAppendEntriesRPCStr(args, reply)), "purple")
		rf.PrintRfLog()
		return
	}
}

// locked，此方法执行前已获得锁，执行后caller会释放锁
func (rf *Raft) leaderHandleLog(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	// 此方法中，args, reply, rf.currentTerm中的term均相等
	// 处理陈旧的reply，如果args中prevLogIndex和当前要发送的不相符，就认为是陈旧的
	if args.PrevLogIndex != rf.nextIndex[reply.ServerId]-1 {
		return
	}

	// 1. 对方日志太短
	if !reply.Success && reply.XTerm == -1 {
		rf.nextIndex[reply.ServerId] = reply.XIndex
		rf.PrintLog(fmt.Sprintf("Handle AE RPC resp from [Server %d], log compare failed, log too short", reply.ServerId), "purple")
		// rf.PrintServerState("purple")
		return
	}
	// 2. 日志冲突，根据leader是否持有XTerm分情况讨论
	if !reply.Success {
		indexOfLastEntryWithXTerm := len(rf.log) - 1
		for ; indexOfLastEntryWithXTerm != -1; indexOfLastEntryWithXTerm-- {
			if rf.log[indexOfLastEntryWithXTerm].Term == reply.XTerm {
				break
			}
		}
		// 2.1 leader持有XTerm，将目标rf的nextIndex设置为XTerm串最后一个entry的index+1
		if indexOfLastEntryWithXTerm != -1 {
			rf.nextIndex[reply.ServerId] = indexOfLastEntryWithXTerm + 1
			rf.PrintLog(fmt.Sprintf("Handle AE RPC resp from [Server %d], log compare FAILED, log mismatch", reply.ServerId), "purple")
			return
		}
		// 2.2 leader未持有XTerm，将目标rf的nextIndex设置为XIndex
		if indexOfLastEntryWithXTerm == -1 {
			rf.nextIndex[reply.ServerId] = reply.XIndex
			rf.PrintLog(fmt.Sprintf("Handle AE RPC resp from [Server %d], log compare FAILED, log mismatch", reply.ServerId), "purple")
			return
		}
	}

	// 3. 日志匹配成功
	// 将目标rf的nextIndex设置为PrevLogIndex + len(Entries) + 1
	// 并更新目标rf的matchIndex
	if reply.Success {
		rf.nextIndex[reply.ServerId] = args.PrevLogIndex + len(args.Entries) + 1
		rf.matchIndex[reply.ServerId] = rf.nextIndex[reply.ServerId] - 1
		rf.PrintLog(fmt.Sprintf("Handle AE RPC resp from [Server %d], log compare SUCCESS", reply.ServerId), "purple")
		rf.PrintServerState("purple")
		return
	}

	// leader检查和更新commitIndex定期触发
}

// 定期执行，leader检查和更新commit情况
func (rf *Raft) leaderUpdateCommitIndex() {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	if len(rf.log) == 0 {
		return
	}

	sortedMatchIndex := make([]int, len(rf.matchIndex))
	copy(sortedMatchIndex, rf.matchIndex)
	sort.Ints(sortedMatchIndex)

	// 滑动窗口查找matchIndex中大多数
	majorityIndex := -1
	i, windowLen := 0, 0
	for {
		if i+windowLen > len(sortedMatchIndex)-1 {
			break
		}
		if windowLen+1 >= (len(rf.peers)+1)/2 {
			majorityIndex = sortedMatchIndex[i]
		}
		if sortedMatchIndex[i] != sortedMatchIndex[i+windowLen] {
			i = i + windowLen
			windowLen = 0
		}
		windowLen += 1
	}

	//rf.PrintLog(fmt.Sprintf("Leader update commitIndex, majorityIndex: %d", majorityIndex), "yellow")
	//rf.PrintServerState("yellow")

	// leader认为大多数rf日志为空，无需提交
	if majorityIndex == -1 {
		return
	}

	// leader更新commitIndex的条件
	cond1 := majorityIndex > rf.commitIndex               // 条件1: 大多数server的日志都和leader同步了
	cond2 := rf.log[majorityIndex].Term == rf.currentTerm // 条件2: 大多数server的日志都是当前term的
	if cond1 && cond2 {
		prevCommitIndex := rf.commitIndex
		rf.commitIndex = majorityIndex
		rf.lastApplied = rf.commitIndex
		// 向chan发送消息，传入prevCommitIndex和curCommitIndex
		rf.sendCommittedLogoChannel(prevCommitIndex, rf.commitIndex)
	}
}

// locked
func (rf *Raft) followerUpdateCommitIndex(leaderCommit int) {
	if rf.commitIndex >= leaderCommit {
		return
	}

	prevCommitIndex := rf.commitIndex
	minCommitIndex := leaderCommit
	if len(rf.log) < leaderCommit {
		minCommitIndex = len(rf.log) - 1
	}
	rf.commitIndex = minCommitIndex
	rf.sendCommittedLogoChannel(prevCommitIndex, rf.commitIndex)
}

// locked
func (rf *Raft) sendCommittedLogoChannel(prevCommitIndex int, curCommitIndex int) {
	for i := prevCommitIndex + 1; i <= curCommitIndex; i++ {
		rf.PrintLog(fmt.Sprintf("Send newly commited log, [Index: %d]", i), "yellow")
		rf.PrintServerState("yellow")
		rf.applyCh <- ApplyMsg{CommandValid: true, Command: rf.log[i].Command, CommandIndex: i + 1}
	}
}
