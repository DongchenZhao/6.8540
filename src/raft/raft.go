package raft

//
// this is an outline of the API that raft must expose to
// the service (or tester). see comments below for
// each of these functions for more details.
//
// rf = Make(...)
//   create a new Raft server.
// rf.Start(command interface{}) (index, term, isleader)
//   start agreement on a new log entry
// rf.GetState() (term, isLeader)
//   ask a Raft for its current term, and whether it thinks it is leader
// ApplyMsg
//   each time a new entry is committed to the log, each Raft peer
//   should send an ApplyMsg to the service (or tester)
//   in the same server.
//

import (
	"6.5840/labgob"
	"bytes"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	//	"6.5840/labgob"
	"6.5840/labrpc"
)

// as each Raft peer becomes aware that successive log entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make(). set
// CommandValid to true to indicate that the ApplyMsg contains a newly
// committed log entry.
//
// in part 2D you'll want to send other kinds of messages (e.g.,
// snapshots) on the applyCh, but set CommandValid to false for these
// other uses.
type ApplyMsg struct {
	CommandValid bool
	Command      interface{}
	CommandIndex int

	// For 2D:
	SnapshotValid bool
	Snapshot      []byte
	SnapshotTerm  int
	SnapshotIndex int
}

type LogEntry struct {
	Term    int
	Index   int
	Command interface{}
}

// A Go object implementing a single Raft peer.
type Raft struct {
	mu        sync.RWMutex        // Lock to protect shared access to this peer's state
	peers     []*labrpc.ClientEnd // RPC end points of all peers
	persister *Persister          // Object to hold this peer's persisted state
	me        int                 // this peer's index into peers[]
	dead      int32               // set by Kill()

	// Your data here (2A, 2B, 2C).
	// Look at the paper's Figure 2 for a description of what
	// state a Raft server must maintain.

	// ------ persistent state on all servers ------
	currentTerm int
	votedFor    int
	log         []LogEntry

	// ------ volatile state on all servers ------
	commitIndex int
	lastApplied int

	// ------ volatile state on leaders ------
	nextIndex  []int
	matchIndex []int

	// ------ others -----
	role              int // 0: follower, 1: candidate, 2: leader
	lastHeartbeatTime int64
	electionTimeout   int
	voteCnt           int
	curLeader         int
	applyCh           chan ApplyMsg
	applyMsgReady     bool
	applyChBuffer     []ApplyMsg
	applyChMu         sync.RWMutex
	applyChCond       sync.Cond

	// ------ snapshot, persistent state on all servers ------
	snapshot      []byte
	snapshotIndex int // -1表示没有快照
	snapshotTerm  int
}

// return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {

	var term int
	var isLeader bool
	// Your code here (2A).
	rf.mu.RLock()
	isLeader = rf.role == 2
	term = rf.currentTerm
	rf.mu.RUnlock()

	return term, isLeader
}

// save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
// before you've implemented snapshots, you should pass nil as the
// second argument to persister.Save().
// after you've implemented snapshots, pass the current snapshot
// (or nil if there's not yet a snapshot).

// locked
func (rf *Raft) persist() {
	// Your code here (2C).
	// Example:
	w := new(bytes.Buffer)
	e := labgob.NewEncoder(w)
	e.Encode(rf.currentTerm)
	e.Encode(rf.votedFor)
	e.Encode(rf.log)
	e.Encode(rf.snapshotIndex)
	e.Encode(rf.snapshotTerm)
	raftState := w.Bytes()
	rf.persister.Save(raftState, rf.snapshot)
}

// restore previously persisted state.
func (rf *Raft) readPersist(data []byte) {
	if data == nil || len(data) < 1 { // bootstrap without any state?
		return
	}
	// Your code here (2C).
	// Example:
	r := bytes.NewBuffer(data)
	d := labgob.NewDecoder(r)
	var currentTerm int
	var votedFor int
	var log []LogEntry
	// snapshot
	var snapshotIndex int
	var snapshotTerm int

	if d.Decode(&currentTerm) != nil || d.Decode(&votedFor) != nil || d.Decode(&log) != nil || d.Decode(&snapshotIndex) != nil || d.Decode(&snapshotTerm) != nil {
		// error...
		rf.PrintLog("DECODE ERROR", "red")
	} else {
		rf.currentTerm = currentTerm
		rf.votedFor = votedFor
		rf.log = log

		rf.snapshotIndex = snapshotIndex
		rf.snapshotTerm = snapshotTerm
	}
}

func (rf *Raft) readPersistSnapshot(data []byte) {
	rf.snapshot = data
}

// the service says it has created a snapshot that has
// all info up to and including index. this means the
// service no longer needs the log through (and including)
// that index. Raft should now trim its log as much as possible.
func (rf *Raft) Snapshot(index int, snapshot []byte) {
	// Your code here (2D).
	targetIndex := index - 1
	rf.installCurRfSnapShot(targetIndex, snapshot)
}

// the service using Raft (e.g. a k/v server) wants to start
// agreement on the next command to be appended to Raft's log. if this
// server isn't the leader, returns false. otherwise start the
// agreement and return immediately. there is no guarantee that this
// command will ever be committed to the Raft log, since the leader
// may fail or lose an election. even if the Raft instance has been killed,
// this function should return gracefully.
//
// the first return value is the index that the command will appear at
// if it's ever committed. the second return value is the current
// term. the third return value is true if this server believes it is
// the leader.
func (rf *Raft) Start(command interface{}) (int, int, bool) {
	rf.mu.Lock()

	isLeader := rf.role == 2
	if isLeader {

		var index int         // 下一条日志的index
		if len(rf.log) != 0 { // 如果日志不为空，取最后一条日志的index + 1
			index = rf.log[len(rf.log)-1].Index + 1
		} else { // 如果日志为空，可能全空或已快照，如果没有快照，index为0(-1 + 1)，否则index仍未下一条日志的index
			index = rf.snapshotIndex + 1
		}

		rf.log = append(rf.log, LogEntry{Term: rf.currentTerm, Command: command, Index: index})
		term := rf.currentTerm

		rf.matchIndex[rf.me] = index
		rf.nextIndex[rf.me] = index + 1

		//	rf.lastApplied
		rf.PrintLog(fmt.Sprintf("Leader accepts a new log, [Term %d] [Index %d]", term, index), "skyblue")
		rf.PrintRfLog()
		rf.PrintServerState("red")

		rf.persist()

		rf.mu.Unlock()
		rf.leaderSendAppendEntriesRPC()

		return index + 1, term, isLeader
	} else {
		rf.mu.Unlock()
		return -1, 0, isLeader
	}
}

// the tester doesn't halt goroutines created by Raft after each test,
// but it does call the Kill() method. your code can use killed() to
// check whether Kill() has been called. the use of atomic avoids the
// need for a lock.
//
// the issue is that long-running goroutines use memory and may chew
// up CPU time, perhaps causing later tests to fail and generating
// confusing debug output. any goroutine with a long-running loop
// should call killed() to check whether it should stop.
func (rf *Raft) Kill() {
	atomic.StoreInt32(&rf.dead, 1)
	// Your code here, if desired.
}

func (rf *Raft) killed() bool {
	z := atomic.LoadInt32(&rf.dead)
	return z == 1
}

// 写入器，将数据写入缓冲区
func (rf *Raft) putApplyChBuffer(msg []ApplyMsg) {
	rf.applyChMu.Lock()
	for len(rf.applyChBuffer) != 0 {
		rf.applyChCond.Wait()
	}
	rf.applyChBuffer = append(rf.applyChBuffer, msg...)
	rf.applyChCond.Signal()
	rf.applyChMu.Unlock()
}

// 读取器，将数据从缓冲区发送到applyCh
func (rf *Raft) getApplyChBuffer() {
	for !rf.killed() {
		rf.applyChMu.Lock()
		for len(rf.applyChBuffer) == 0 {
			rf.applyChCond.Wait()
		}
		for i := 0; i < len(rf.applyChBuffer); i++ {
			rf.PrintLog(fmt.Sprintf("Send newly commited log, [CommandIndex: %d]", rf.applyChBuffer[i].CommandIndex-1), "skyblue")
			rf.applyCh <- rf.applyChBuffer[i]
		}
		rf.applyChBuffer = make([]ApplyMsg, 0)
		rf.applyChCond.Signal()
		rf.applyChMu.Unlock()
	}
}

// the service or tester wants to create a Raft server. the ports
// of all the Raft servers (including this one) are in peers[]. this
// server's port is peers[me]. all the servers' peers[] arrays
// have the same order. persister is a place for this server to
// save its persistent state, and also initially holds the most
// recent saved state, if any. applyCh is a channel on which the
// tester or service expects Raft to send ApplyMsg messages.
// Make() must return quickly, so it should start goroutines
// for any long-running work.
func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *Raft {
	rf := &Raft{}
	rf.peers = peers
	rf.persister = persister
	rf.me = me

	rf.applyCh = applyCh
	rf.applyChCond = *sync.NewCond(&rf.applyChMu)
	rf.applyChBuffer = make([]ApplyMsg, 0)

	// 初始化
	rf.mu.Lock()

	rf.currentTerm = 0
	rf.votedFor = -1
	rf.log = make([]LogEntry, 0)
	rf.commitIndex = -1
	rf.lastApplied = -1
	rf.role = 0
	rf.lastHeartbeatTime = time.Now().UnixMilli()
	rf.electionTimeout = 200 + rand.Intn(100)
	// snapshot相关初始化
	rf.snapshot = nil
	rf.snapshotTerm = 0
	rf.snapshotIndex = -1

	// 读取持久化数据
	rf.readPersist(persister.ReadRaftState())
	rf.readPersistSnapshot(persister.ReadSnapshot())

	// 根据快照，更新自己的commitIndex
	rf.commitIndex = rf.snapshotIndex
	rf.lastApplied = rf.commitIndex

	// 重启之后重新commit snapshot
	if rf.snapshotIndex != -1 {
		rf.putApplyChBuffer([]ApplyMsg{ApplyMsg{CommandValid: false, Command: nil, CommandIndex: -1, SnapshotValid: true, SnapshotIndex: rf.snapshotIndex + 1, SnapshotTerm: rf.snapshotTerm, Snapshot: rf.snapshot}})
	}

	rf.PrintLog(fmt.Sprintf("RESTARTED"), "red")
	rf.PrintServerState("red")

	rf.mu.Unlock()

	// start ticker goroutine to start elections
	go rf.ticker()

	// 单独的用于发送日志的goroutine
	go rf.getApplyChBuffer()

	return rf
}
