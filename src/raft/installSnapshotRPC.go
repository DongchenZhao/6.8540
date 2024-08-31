package raft

import (
	"fmt"
	"time"
)

type InstallSnapshotArgs struct {
	Term              int
	LeaderId          int
	LastIncludedIndex int
	LastIncludedTerm  int
	Snapshot          []byte
}

type InstallSnapshotReply struct {
	Term     int
	ServerId int
}

func (rf *Raft) InstallSnapshotRequestHandler(args *InstallSnapshotArgs, reply *InstallSnapshotReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	defer rf.persist()

	reply.ServerId = rf.me

	if rf.snapshotIndex == args.LastIncludedIndex && rf.snapshotTerm != args.LastIncludedTerm {
		panic("InstallSnapshotRequestHandler ERROR: snapshot inconsistent!")
	}

	// 处理陈旧请求，如果已经install(陈旧请求)，或者leader term太小(陈旧leader)，直接返回
	if rf.currentTerm > args.Term {
		rf.PrintLog(fmt.Sprintf("REJECT InstallSnapshot RPC Req from [Leader %d], REJECT due to higher, [LeaderTerm %d] [CurTerm %d]", args.LeaderId, args.Term, rf.currentTerm), "blue")
		reply.Term = rf.currentTerm
		return
	}

	// 重置心跳时间
	rf.lastHeartbeatTime = time.Now().UnixMilli()

	// 已经install(陈旧请求)
	if rf.snapshotIndex >= args.LastIncludedIndex {
		rf.PrintLog(fmt.Sprintf("REJECT InstallSnapshot RPC Req from [Leader %d], REJECT due to already snapshoted", args.LeaderId), "blue")
		reply.Term = rf.currentTerm
		return
	}

	// 如果当前rf term过小，先增加term
	if rf.currentTerm < args.Term {
		rf.toFollower(args.Term)
	}

	// 创建快照
	trueIndex := rf.findIndex(args.LastIncludedIndex)
	// 1. 当前rf过于落后，日志未达到snapshot的部分
	if trueIndex == -1 {
		rf.snapshot = args.Snapshot
		rf.snapshotIndex = args.LastIncludedIndex
		rf.snapshotTerm = args.LastIncludedTerm
		rf.log = make([]LogEntry, 0)

		// fixed 2d: 安装完日志之后更新commitIndex
		prevCommitIndex := rf.commitIndex
		rf.commitIndex = args.LastIncludedIndex
		if rf.commitIndex < prevCommitIndex {
			panic("InstallSnapshotRequestHandler ERROR: commitIndex inconsistent!")
		}

		// fix 2d: 此处需要向client报告已安装快照
		go rf.putApplyChBuffer([]ApplyMsg{ApplyMsg{CommandValid: false, Command: nil, CommandIndex: -1, SnapshotValid: true, SnapshotIndex: rf.snapshotIndex + 1, SnapshotTerm: rf.snapshotTerm, Snapshot: rf.snapshot}})

		reply.Term = rf.currentTerm
		rf.PrintLog(fmt.Sprintf("Handle InstallSnapshot RPC Req from [Leader %d], SUCCESS", args.LeaderId), "blue")
		rf.PrintServerState("blue")
		return
	}

	if rf.log[trueIndex].Term != args.LastIncludedTerm {
		panic("InstallSnapshotRequestHandler ERROR: log inconsistent!")
	}

	// 2. rf当前日志正好被完全快照或者部分被快照
	rf.snapshot = args.Snapshot
	rf.snapshotIndex = rf.log[trueIndex].Index
	rf.snapshotTerm = rf.log[trueIndex].Term
	rf.log = rf.log[trueIndex+1:]

	// fixed 2d: 安装完日志之后更新commitIndex
	prevCommitIndex := rf.commitIndex
	if prevCommitIndex < args.LastIncludedIndex {
		rf.commitIndex = args.LastIncludedIndex
	}

	// fix 2d: 此处需要向client报告已安装快照
	go rf.putApplyChBuffer([]ApplyMsg{ApplyMsg{CommandValid: false, Command: nil, CommandIndex: -1, SnapshotValid: true, SnapshotIndex: rf.snapshotIndex + 1, SnapshotTerm: rf.snapshotTerm, Snapshot: rf.snapshot}})

	reply.Term = rf.currentTerm
	rf.PrintLog(fmt.Sprintf("Handle InstallSnapshot RPC Req from [Leader %d], SUCCESS", args.LeaderId), "blue")
	rf.PrintServerState("blue")

	//applyMsgLs := make([]ApplyMsg, 0)
	//applyMsgLs = append(applyMsgLs, ApplyMsg{CommandValid: false, Command: nil, CommandIndex: -1, SnapshotValid: true, SnapshotIndex: rf.snapshotIndex + 1, SnapshotTerm: rf.snapshotTerm, Snapshot: rf.snapshot})
	//go rf.putApplyChBuffer(applyMsgLs)

	return
}

func (rf *Raft) InstallSnapshotResponseHandler(args *InstallSnapshotArgs, reply *InstallSnapshotReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	defer rf.persist() // pre-lab3增加持久化

	// fix 2d: 过滤过期请求(arg中term和当前不符)
	if rf.currentTerm != args.Term || rf.currentTerm > reply.Term {
		return
	}

	// 如果对方term更大，自己转为follower
	if rf.currentTerm < reply.Term {
		rf.toFollower(reply.Term)
		return
	}
	// leader更新对应rf的entries同步状态，注意不能backwards
	if rf.matchIndex[reply.ServerId] >= args.LastIncludedIndex {
		rf.PrintLog(fmt.Sprintf("Leader handle InstallSnapshot RPC resp from [Server %d], expired resp", reply.ServerId), "blue")
		rf.PrintServerState("blue")
		return
	}

	rf.nextIndex[reply.ServerId] = args.LastIncludedIndex + 1
	rf.matchIndex[reply.ServerId] = args.LastIncludedIndex
	rf.PrintLog(fmt.Sprintf("Leader handle InstallSnapshot RPC resp from [Server %d], SUCCESS", reply.ServerId), "blue")
	rf.PrintServerState("blue")
}

func (rf *Raft) sendInstallSnapshot(server int, args *InstallSnapshotArgs, reply *InstallSnapshotReply) bool {
	ok := rf.peers[server].Call("Raft.InstallSnapshotRequestHandler", args, reply)
	return ok
}

// SnapShot入口，client实际调用，用于leader或follower自己快照
func (rf *Raft) installCurRfSnapShot(index int, snapshot []byte) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	defer rf.persist()
	rf.PrintLog(fmt.Sprintf("Client Install Snapshot, [Index %d]", index), "blue")
	trueIndex := rf.findIndex(index)
	if trueIndex == -1 { // 已经安装快照
		return
	}
	rf.snapshot = snapshot
	rf.snapshotIndex = rf.log[trueIndex].Index
	rf.snapshotTerm = rf.log[trueIndex].Term
	rf.log = rf.log[trueIndex+1:]
	rf.PrintServerState("blue")

	applyMsgLs := make([]ApplyMsg, 0)
	applyMsgLs = append(applyMsgLs, ApplyMsg{CommandValid: false, Command: nil, CommandIndex: -1, SnapshotValid: true, SnapshotIndex: rf.snapshotIndex + 1, SnapshotTerm: rf.snapshotTerm, Snapshot: rf.snapshot})

	if rf.commitIndex > rf.snapshotIndex {
		trueIndex2 := rf.findIndex(rf.commitIndex)
		// 提交剩余的日志
		for i := 0; i <= trueIndex2; i++ {
			applyMsgLs = append(applyMsgLs, ApplyMsg{CommandValid: true, Command: rf.log[i].Command, CommandIndex: rf.log[i].Index + 1, SnapshotValid: false, SnapshotIndex: -1, SnapshotTerm: -1, Snapshot: nil})
		}
	}

	go rf.putApplyChBuffer(applyMsgLs)
}
