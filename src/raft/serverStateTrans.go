package raft

import (
	"fmt"
	"math/rand"
	"time"
)

func (rf *Raft) toFollower(term int) {
	rf.mu.RLock()
	prevTerm := rf.currentTerm
	prevRole := rf.role
	rf.PrintLog(fmt.Sprintf("Role ---> Follower, [PrevTerm %d] [CurTerm %d] [PrevRole %d]", rf.currentTerm, term, prevRole), "green")
	rf.mu.RUnlock()

	rf.mu.Lock()
	rf.role = 0
	if prevTerm < term {
		// rf.voteCnt = 0  voteCnt在转为candidate的时候会自动设置为1
		rf.votedFor = -1 // 每个server在每个term只能投1次
		rf.currentTerm = term
	}
	rf.mu.Unlock()
}

func (rf *Raft) toCandidate() {
	rf.PrintLog("Role ---> Candidate", "green")

	// 1.增加Term  2.vote给自己  3.重置计时器
	rf.mu.Unlock()
	rf.currentTerm += 1
	rf.votedFor = rf.me
	rf.voteCnt = 1
	rf.lastHeartbeatTime = time.Now().UnixMilli()
	rf.mu.Lock()

	// 4.发送投票请求
	// 发送请求之前获取当前状态的副本
	rf.mu.RLock()
	currentTerm := rf.currentTerm
	lastLogIndex := len(rf.log) - 1
	lastLogTerm := 0 // 日志为空，最后一个term为0
	if lastLogIndex != -1 {
		lastLogTerm = rf.log[lastLogIndex].Term
	}
	rf.mu.Unlock()
	for i := 0; i < len(rf.peers); i++ {
		curI := i
		go func() {
			if curI == rf.me {
				return
			}
			rf.PrintLog(fmt.Sprintf("RV RPC ------> [Server %d], [Term %d], [LastLogIndex %d], [LastLogTerm %d]", curI, currentTerm, lastLogIndex, lastLogTerm), "default")
			requestVoteArgs := RequestVoteArgs{currentTerm, rf.me, lastLogIndex, lastLogTerm}
			requestVoteReply := RequestVoteReply{}
			ok := rf.sendRequestVote(curI, &requestVoteArgs, &requestVoteReply)
			if !ok {
				rf.PrintLog(fmt.Sprintf("RV RPC -----> [Server %d] Failed", curI), "yellow")
			}
			rf.RequestVoteResponseHandler(&requestVoteArgs, &requestVoteReply)
		}()
	}
}

func (rf *Raft) toLeader() {
	rf.mu.RLock()
	rf.PrintLog(fmt.Sprintf("Role ---> Leader, [Term: %d]", rf.currentTerm), "green")
	rf.mu.RUnlock()
	rf.PrintState("Leader PrintState")

	rf.mu.Lock()
	rf.nextIndex = make([]int, len(rf.peers))
	rf.matchIndex = make([]int, len(rf.peers))
	for i := 0; i < len(rf.peers); i++ {
		rf.nextIndex[i] = len(rf.log)
		rf.matchIndex[i] = -1 // 表示当前leader认为其他server的log为空，(选择初始化为https://thesquareplanet.com/blog/students-guide-to-raft/中的-1而不是论文中的0,貌似因为论文中log index从1开始)
	}
	// leader更新自己的nextIndex和matchIndex
	rf.nextIndex[rf.me] = len(rf.log)
	rf.matchIndex[rf.me] = len(rf.log) - 1

	rf.mu.Unlock()

	go rf.leaderTicker()
}

func (rf *Raft) leaderTicker() {
	for rf.killed() == false {
		rf.mu.RLock()
		role := rf.role
		rf.mu.Unlock()

		if role != 2 {
			rf.PrintLog("No longer leader", "green")
			return
		}

		rf.leaderSendAppendEntriesRPC()

		time.Sleep(120 * time.Millisecond)
	}
}

func (rf *Raft) ticker() {
	for rf.killed() == false {

		// 检查是否需要发起选举
		rf.mu.RLock()
		role := rf.role
		lastHBTime := rf.lastHeartbeatTime
		electionTimeout := rf.electionTimeout
		rf.mu.Unlock()

		// leader不需要定期检查心跳是否超时
		if role == 2 {
			return
		}

		// follower或candidate检查election timeout
		if time.Now().UnixMilli()-lastHBTime > int64(electionTimeout) {
			rf.toCandidate()
		}

		// pause for a random amount of time between 50 and 350
		// milliseconds.
		ms := 50 + (rand.Int63() % 300)
		time.Sleep(time.Duration(ms) * time.Millisecond)
	}
}
