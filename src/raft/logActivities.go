package raft

import (
	"fmt"
	"sort"
	"strconv"
)

// locked，此方法执行前已获得锁，执行后caller会释放锁
func (rf *Raft) followerHandleLog(args *AppendEntriesArgs, reply *AppendEntriesReply) {

	prevLogIndex, _ := rf.getLastLogIndexAndTerm()

	// 1. 日志长度不够
	if prevLogIndex < args.PrevLogIndex {
		reply.XTerm, reply.XIndex, reply.XLen = -1, prevLogIndex+1, len(rf.log) // 此处只有XIndex有用
		reply.Success = false
		rf.PrintLog(fmt.Sprintf("AE RPC Resp -----> [Leader %d], log comapre FAILED,, EMPTY ENTRY at PrevLogIndex, ", args.LeaderId)+getAppendEntriesRPCStr(args, reply), "purple")
		rf.PrintServerState("purple")
		return
	}

	actualIndex := rf.findIndex(args.PrevLogIndex) // 排除日志长度不够，除非follower快照，否则一定找得到

	// 2d: follower过快拍摄快照是否会导致follower日志长度不够，即leader再次匹配已commit的日志
	if actualIndex == -1 && args.PrevLogIndex != -1 { // fixed 2d: 此处需要考虑初始情况
		// follower已压缩，相当于直接匹配成功，因为以任何已压缩的日志都是commit的
		// 1. 找出entries中未commit的部分
		entriesActualStartIndex := 0
		foundUnCommittedLog := false
		for ; entriesActualStartIndex < len(args.Entries); entriesActualStartIndex++ {
			if args.Entries[entriesActualStartIndex].Index == rf.snapshotIndex+1 {
				foundUnCommittedLog = true
				break
			}
		}
		// 2. leader那边来的entries中全部在快照中
		if !foundUnCommittedLog {
			reply.XTerm, reply.XIndex, reply.XLen = -1, -1, len(rf.log)
			reply.Success = true
			rf.PrintLog(fmt.Sprintf("AE RPC Resp -----> [Leader %d], log comapre SUCCESS, ALL IN SNAPSHOT", args.LeaderId)+getAppendEntriesRPCStr(args, reply), "purple")
			rf.PrintServerState("purple")
			return
		}
		// 3. leader那边来的entries部分在快照中，看情况append这些newEntries
		if foundUnCommittedLog {
			newEntries := args.Entries[entriesActualStartIndex:]
			if len(rf.log) <= len(newEntries) {
				rf.log = newEntries
			} else {
				for i := 0; i < len(newEntries); i++ {
					if rf.log[i].Term != newEntries[i].Term {
						rf.log = newEntries
					}
				}
			}
			reply.XTerm, reply.XIndex, reply.XLen = -1, -1, len(rf.log)
			reply.Success = true
			rf.PrintLog(fmt.Sprintf("AE RPC Resp -----> [Leader %d], log comapre SUCCESS, PATRIALLY IN SNAPSHOT", args.LeaderId)+getAppendEntriesRPCStr(args, reply), "purple")
			rf.PrintServerState("purple")
			return
		}
	}

	// 2. 日志冲突，截断，XIndex为冲突日志串的第一个entry的index，XTerm即为冲突的Term
	// PrevLogIndex为-1的时候一定匹配成功
	if args.PrevLogIndex != -1 && rf.log[actualIndex].Term != args.PrevLogTerm {
		rf.PrintLog("conflict at prevLogIndex, cur rf term: "+strconv.Itoa(rf.log[actualIndex].Term), "red")
		tarTerm := rf.log[actualIndex].Term
		startIndex := 0
		for ; startIndex <= actualIndex; startIndex++ {
			if rf.log[startIndex].Term == tarTerm {
				break
			}
		}

		// 截断日志
		// fixed 2D: 如果冲突日志位置仅为单独的日志串（例如 1 1 1 2最后一个2冲突），则截断后再返回冲突位置的第一个entry的index实际上是会触发panic的，因为这个entry已经被截断了，rf.log[startIndex].Index会造成溢出
		// 此处修改为在截断之前保存XIndex
		targetIndex := rf.log[startIndex].Index
		rf.log = rf.log[:actualIndex]

		reply.XTerm, reply.XIndex, reply.XLen = tarTerm, targetIndex, len(rf.log)
		reply.Success = false
		rf.PrintLog(fmt.Sprintf("AE RPC Resp -----> [Leader %d], log comapre FAILED, CONFLICT at PrevLogIndex, ", args.LeaderId)+getAppendEntriesRPCStr(args, reply), "purple")
		rf.PrintServerState("purple")
		return
	}

	// 3. 匹配成功
	if args.PrevLogIndex == -1 || rf.log[actualIndex].Term == args.PrevLogTerm {
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
		if (len(rf.log)-1)-actualIndex <= len(args.Entries) { // leader日志更长，则leader日志一定更新，如果leader日志旧，在匹配成功的前提下就意味着rf已经append更新leader的日志了，如果这样，rf的日志会比prevIndex + len(entries)更长
			rf.log = append(rf.log[:actualIndex+1], args.Entries...)
		} else { // rf日志比prevLogIndex + 1 + len(entries)更长，但并不意味着rf的日志更新
			for i := actualIndex + 1; i < actualIndex+1+len(args.Entries); i++ {
				if rf.log[i].Term != args.Entries[i-actualIndex-1].Term {
					remainingEntries := args.Entries[i-actualIndex-1:]
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
		actualIndexOfLastEntryWithXTerm := len(rf.log) - 1
		for ; actualIndexOfLastEntryWithXTerm != -1; actualIndexOfLastEntryWithXTerm-- {
			if rf.log[actualIndexOfLastEntryWithXTerm].Term == reply.XTerm {
				break
			}
		}
		// 2.1 leader持有XTerm，将目标rf的nextIndex设置为XTerm串最后一个entry的index+1
		if actualIndexOfLastEntryWithXTerm != -1 {
			rf.nextIndex[reply.ServerId] = rf.log[actualIndexOfLastEntryWithXTerm].Index + 1
			rf.PrintLog(fmt.Sprintf("Handle AE RPC resp from [Server %d], log compare FAILED, log mismatch", reply.ServerId), "purple")
			return
		}
		// 2.2 leader未持有XTerm，将目标rf的nextIndex设置为XIndex
		if actualIndexOfLastEntryWithXTerm == -1 {
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
	defer rf.persist() // pre-lab3增加持久化

	if len(rf.log) == 0 && rf.snapshotIndex == -1 {
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
	cond1 := majorityIndex > rf.commitIndex // 条件1: 大多数server的日志都和leader同步了
	if !cond1 {
		return
	}
	cond2 := rf.log[rf.findIndex(majorityIndex)].Term == rf.currentTerm // 条件2: 大多数server的日志都是当前term的，即：leader只能提交当前term的日志（figure 8）
	if cond1 && cond2 {
		prevCommitIndex := rf.commitIndex
		rf.commitIndex = majorityIndex
		rf.lastApplied = rf.commitIndex
		rf.PrintLog(fmt.Sprintf("Leader update commitIndex, [PrevCommitIndex %d] [CurCommitIndex %d]", prevCommitIndex, rf.commitIndex), "yellow")
		rf.PrintServerState("yellow")
		// 向chan发送消息，传入prevCommitIndex和curCommitIndex
		rf.sendCommittedLogChannel(prevCommitIndex, rf.commitIndex)
	}
}

// locked
func (rf *Raft) followerUpdateCommitIndex(leaderCommit int) {
	if rf.commitIndex >= leaderCommit { // 如果当前rf commitIndex更大，无视(可能是leader陈旧rpc)
		return
	}

	// 否则，取当前最大logIndex和leaderCommit中较小的那个当做自己的commitIndex
	prevCommitIndex := rf.commitIndex
	minCommitIndex := leaderCommit
	lastLogIndex, _ := rf.getLastLogIndexAndTerm()

	if lastLogIndex < leaderCommit {
		minCommitIndex = lastLogIndex
	}
	rf.commitIndex = minCommitIndex
	rf.lastApplied = rf.commitIndex
	rf.PrintLog(fmt.Sprintf("Follower update commitIndex, [PrevCommitIndex %d] [CurCommitIndex %d]", prevCommitIndex, rf.commitIndex), "yellow")
	rf.sendCommittedLogChannel(prevCommitIndex, rf.commitIndex)
}

// locked
func (rf *Raft) sendCommittedLogChannel(prevCommitIndex int, curCommitIndex int) {
	applyMsgLs := make([]ApplyMsg, 0)
	// 这里的待提交日志尚未告诉client，因此不存在被快照的可能
	startIndex := 0

	found := false
	if prevCommitIndex == -1 || prevCommitIndex == rf.snapshotIndex {
		startIndex = -1
		found = true
	} else {
		for ; startIndex < len(rf.log); startIndex++ {
			if rf.log[startIndex].Index == prevCommitIndex {
				found = true
				break
			}
		}
	}

	if !found {
		panic("should found startIndex")
	}

	for i := startIndex + 1; i <= startIndex+(curCommitIndex-prevCommitIndex); i++ {
		applyMsgLs = append(applyMsgLs, ApplyMsg{CommandValid: true, Command: rf.log[i].Command, CommandIndex: rf.log[i].Index + 1})
	}
	go rf.putApplyChBuffer(applyMsgLs)
}
