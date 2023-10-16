package raft

import "fmt"

func (rf *Raft) updateCommitIndex() {
	return
}

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
		if len(rf.log)-(args.PrevLogIndex+1) < len(args.Entries) {
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

}
