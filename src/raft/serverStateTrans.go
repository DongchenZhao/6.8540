package raft

import (
	"fmt"
	"math/rand"
	"time"
)

// locked
// 确保进入这个方法之前，获取了mu.lock
func (rf *Raft) toFollower(term int) {
	prevTerm := rf.currentTerm
	prevRole := rf.role
	prevRoleStr := getRoleStr(prevRole)

	if prevTerm < term || prevRole != 0 {
		rf.PrintLog(fmt.Sprintf("Role [%s] ---> [Follower], [PrevTerm %d] [CurTerm %d] [PrevRole %s]", prevRoleStr, rf.currentTerm, term, prevRoleStr), "green")
	}

	rf.role = 0
	if prevTerm < term {
		// rf.voteCnt = 0  voteCnt在转为candidate的时候会自动设置为1
		rf.votedFor = -1 // 每个server在每个term只能投1次
		rf.currentTerm = term
	}
}

func (rf *Raft) toCandidate() {

	// 1.增加Term  2.vote给自己  3.重置计时器
	// fixed：重置选举超时时间，否则可能会出现某个candidate长时间压制的问题
	rf.mu.Lock()
	roleStr := getRoleStr(rf.role)
	rf.role = 1
	rf.currentTerm += 1
	rf.votedFor = rf.me
	rf.voteCnt = 1
	rf.electionTimeout = 200 + rand.Intn(100)
	rf.lastHeartbeatTime = time.Now().UnixMilli()
	rf.PrintLog(fmt.Sprintf("Role [%s]---> [Candidate], update term from [Term %d] to [Term %d]", roleStr, rf.currentTerm-1, rf.currentTerm), "green")

	// 4.发送投票请求
	// 发送请求之前获取当前状态的副本
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
				// rf.PrintLog(fmt.Sprintf("RV RPC -----> [Server %d] Failed", curI), "yellow")
			}
			// rf.PrintLog("Call RV RPC Handler--"+strconv.Itoa(curI), "red")
			rf.RequestVoteResponseHandler(&requestVoteArgs, &requestVoteReply)
		}()
	}
}

func (rf *Raft) toLeader() {

	rf.mu.Lock()

	roleStr := getRoleStr(rf.role)
	rf.PrintLog(fmt.Sprintf("Role ["+roleStr+"] ---> [Leader], [Term: %d]", rf.currentTerm), "green")
	rf.PrintServerState("green")
	rf.role = 2
	rf.nextIndex = make([]int, len(rf.peers))
	rf.matchIndex = make([]int, len(rf.peers))
	for i := 0; i < len(rf.peers); i++ {
		rf.nextIndex[i] = len(rf.log)
		rf.matchIndex[i] = -1 // 表示当前leader认为其他server的log为空，(选择初始化为https://thesquareplanet.com/blog/students-guide-to-raft/中的-1而不是论文中的0,貌似因为论文中log index从1开始)
	}
	// leader更新自己的nextIndex和matchIndex
	rf.nextIndex[rf.me] = len(rf.log)
	rf.matchIndex[rf.me] = len(rf.log) - 1

	rf.votedFor = -1 // 持久化之后至少不会产生歧义，毕竟是上一个term投的票了
	rf.persist()

	rf.mu.Unlock()

	go rf.leaderTicker()
}

func (rf *Raft) leaderTicker() {
	for rf.killed() == false {
		rf.mu.RLock()
		role := rf.role
		rf.mu.RUnlock()

		if role != 2 {
			rf.PrintLog("No longer leader", "green")
			return
		}

		rf.leaderUpdateCommitIndex()
		rf.leaderSendAppendEntriesRPC()

		time.Sleep(100 * time.Millisecond)
	}
}

func (rf *Raft) ticker() {
	for rf.killed() == false {

		// 检查是否需要发起选举
		rf.mu.RLock()
		role := rf.role
		lastHBTime := rf.lastHeartbeatTime
		electionTimeout := rf.electionTimeout
		rf.mu.RUnlock()

		// leader不需要定期检查心跳是否超时
		if role == 2 {
			continue
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
