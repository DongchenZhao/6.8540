package kvraft

import (
	"6.5840/labrpc"
	"fmt"
	"sync"
)
import "crypto/rand"
import "math/big"

type Clerk struct {
	servers []*labrpc.ClientEnd
	// You will have to modify this struct.
	mu       sync.Mutex
	id       int64
	leaderId int
	seq      int
}

func nrand() int64 {
	max := big.NewInt(int64(1) << 62)
	bigx, _ := rand.Int(rand.Reader, max)
	x := bigx.Int64()
	return x
}

// MakeClerk 新建一个客户端
func MakeClerk(servers []*labrpc.ClientEnd) *Clerk {
	ck := new(Clerk)
	ck.servers = servers
	// 初始化客户端
	ck.leaderId = 0
	ck.id = nrand()
	ck.seq = 0
	return ck
}

// fetch the current value for a key.
// returns "" if the key does not exist.
// keeps trying forever in the face of all other errors.
//
// you can send an RPC with code like this:
// ok := ck.servers[i].Call("KVServer.Get", &args, &reply)
//
// the types of args and reply (including whether they are pointers)
// must match the declared types of the RPC handler function's
// arguments. and reply must be passed as a pointer.
func (ck *Clerk) Get(key string) string {

	// You will have to modify this function.
	ck.seq++
	args := GetArgs{Key: key, ClientId: ck.id, Seq: ck.seq}
	serverId := ck.leaderId
	for {
		reply := GetReply{}
		PrintLog(fmt.Sprintf("Get RPC ------> [Server %d], [Args %v]", serverId, args), "default", 1, false)
		ok := ck.servers[serverId].Call("KVServer.Get", &args, &reply)
		if ok {
			if reply.Err == ErrNoKey {
				ck.leaderId = serverId
				return ""
			} else if reply.Err == OK {
				ck.leaderId = serverId
				return reply.Value
			} else if reply.Err == ErrWrongLeader {
				serverId = (serverId + 1) % len(ck.servers)
				continue
			}
		}
		// 重试，网络不可能无限超时，超时部分应该是由rpc部分来实现的
		PrintLog(fmt.Sprintf("Get RPC ------> [Server %d], [Args %v] failed, maybe server failed or packet lost", serverId, args), "yellow", 1, false)
		serverId = (serverId + 1) % len(ck.servers)
	}
}

// shared by Put and Append.
//
// you can send an RPC with code like this:
// ok := ck.servers[i].Call("KVServer.PutAppend", &args, &reply)
//
// the types of args and reply (including whether they are pointers)
// must match the declared types of the RPC handler function's
// arguments. and reply must be passed as a pointer.
func (ck *Clerk) PutAppend(key string, value string, op string) {
	// You will have to modify this function.
	ck.seq++
	args := PutAppendArgs{Key: key, Value: value, Op: op, ClientId: ck.id, Seq: ck.seq}
	serverId := ck.leaderId
	for {
		reply := PutAppendReply{}
		PrintLog(fmt.Sprintf("PutAppend RPC ------> [Server %d], [Args %v]", serverId, args), "blue", 1, false)
		ok := ck.servers[serverId].Call("KVServer.PutAppend", &args, &reply)
		if ok {
			if reply.Err == OK {
				ck.leaderId = serverId
				return
			} else if reply.Err == ErrWrongLeader {
				serverId = (serverId + 1) % len(ck.servers)
				continue
			}
		}
		// 重试，网络不可能无限超时，超时部分应该是由rpc部分来实现的
		PrintLog(fmt.Sprintf("PutAppend RPC ------> [Server %d], [Args %v] failed, maybe server failed or packet lost", serverId, args), "yellow", 1, false)
		serverId = (serverId + 1) % len(ck.servers)
	}
}

func (ck *Clerk) Put(key string, value string) {
	ck.PutAppend(key, value, "Put")
}
func (ck *Clerk) Append(key string, value string) {
	ck.PutAppend(key, value, "Append")
}
