package raft

type InstallSnapshotArgs struct {
}

type InstallSnapshotReply struct {
}

// SnapShot入口，client实际调用，用于leader或follower自己快照
func (rf *Raft) installSnapShot(index int, snapshot []byte) {

}
