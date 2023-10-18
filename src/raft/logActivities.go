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
		rf.PrintLog(fmt.Sprintf("AE RPC Resp -----> [Leader %d], log comapre FAILED,, EMPTY ENTRY at PrevLogIndex, ", args.LeaderId)+getAppendEntriesRPCStr(args, reply), "purple")
		rf.PrintServerState("purple")
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
		rf.PrintLog(fmt.Sprintf("AE RPC Resp -----> [Leader %d], log comapre FAILED, CONFLICT at PrevLogIndex, ", args.LeaderId)+getAppendEntriesRPCStr(args, reply), "purple")
		rf.PrintServerState("purple")
		return
	}

	// 3. 匹配成功
	if args.PrevLogIndex == -1 || rf.log[args.PrevLogIndex].Term == args.PrevLogTerm {
		// append arg中所有尚未append的日志(从PrevLogIndex开始，长度比较)
		// 同一个Term下的AE RPC，不可能有先后2个RPC在同一prevLogIndex位置加了长度相同、内容不同的entries

		//if (len(rf.log)-1)-args.PrevLogIndex <= len(args.Entries) {
		//	rf.log = append(rf.log[:args.PrevLogIndex+1], args.Entries...)
		//}

		// fixed: 考虑以下情况：
		// [f1 1 2] [f2 1 2] [l1 1 2] leader被分区，接受另一个2，其他2个接受3，并选出新leader l2，l1认为自己仍是leader
		// [f1 1 2 3] [l2 1 2 3] | [l1 1 2 2]
		// 分区愈合，l2发送给l1新加的3，prevLogIndex=1 prevLogTerm=2 entries=3 leaderCommit=2
		// 修改前，l1只接受拼接之后比自己长的日志，这会导致l1不覆盖日志并更新commitIndex
		// 最终导致l1把原来应该覆盖掉的最后一个"2"给提交了
		// 所以把上面判断条件"<"改为"<="
		// TestRejoin2B发现了这个bug

		// fixed2: 考虑以下情况
		// 上面例子分区愈合之后，l1自认为leader，append了一条新的日志
		// 这时候prevLogIndex + len(entries) < len(rf.log) - 1，然而仍然是entries日志更新，所以还是要逐个比较
		// 分区愈合之前，且数据都已提交
		// l1 1 2 2 2 少数rf所在分区，term=2，认为自己是leader
		// l2 1 2 2 2 多数rf所在分区，term=3，一直是leader，真正的leader
		// 分区愈合瞬间
		// l1 1 2 2 2 | 2 2 接受了client输入
		// l2 1 2 2 2 |
		// l1下台，l2收到client输入
		// l1 1 2 2 2 | 2 2
		// l2 1 2 2 2 | 3
		// l2发给l1 prevLogIndex=3 prevLogTerm=2 entries=[3] 因为l2当选leader的时候nextIndex数组初始化为4
		// l1会认为日志匹配并保留自己的日志，使得l2误以为日志已复制，实际上却没有
		// TestBackup2B

		// 在rf.log[prevLogIndex + 1]直到rf日志末尾，如果出现和entries不匹配的情况，截断
		if (len(rf.log)-1)-args.PrevLogIndex <= len(args.Entries) {
			rf.log = append(rf.log[:args.PrevLogIndex+1], args.Entries...)
		} else { // rf日志比prevLogIndex + 1 + len(entries)更长，但并不意味着rf的日志更新
			for i := args.PrevLogIndex + 1; i < args.PrevLogIndex+1+len(args.Entries); i++ {
				if rf.log[i].Term != args.Entries[i-args.PrevLogIndex-1].Term {
					remainingEntries := args.Entries[i-args.PrevLogIndex-1:]
					rf.log = append(rf.log[:i], remainingEntries...)
				}
			}
		}

		reply.XTerm, reply.XIndex, reply.XLen = -1, -1, len(rf.log)
		reply.Success = true
		rf.PrintLog(fmt.Sprintf("AE RPC Resp -----> [Leader %d], log comapre SUCCESS, ", args.LeaderId)+getAppendEntriesRPCStr(args, reply), "purple")
		rf.PrintServerState("purple")
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
	// fixed: 无法处理[1 2 3]这种情况
	// 排序，取数组最后(n + 1)/2个元素的第一个
	majorityIndex := sortedMatchIndex[(len(rf.peers)+1)/2-1]

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
		rf.PrintLog(fmt.Sprintf("Leader update commitIndex, [PrevCommitIndex %d] [CurCommitIndex %d]", prevCommitIndex, rf.commitIndex), "yellow")
		rf.PrintServerState("yellow")
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
	if len(rf.log)-1 < leaderCommit {
		minCommitIndex = len(rf.log) - 1
	}
	rf.commitIndex = minCommitIndex
	rf.sendCommittedLogoChannel(prevCommitIndex, rf.commitIndex)
}

// locked
func (rf *Raft) sendCommittedLogoChannel(prevCommitIndex int, curCommitIndex int) {
	applyMsgLs := make([]ApplyMsg, 0)
	for i := prevCommitIndex + 1; i <= curCommitIndex; i++ {
		applyMsgLs = append(applyMsgLs, ApplyMsg{CommandValid: true, Command: rf.log[i].Command, CommandIndex: i + 1})
	}
	go rf.putApplyChBuffer(applyMsgLs)
}
