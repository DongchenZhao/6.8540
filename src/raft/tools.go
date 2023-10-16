package raft

import (
	"fmt"
	"log"
	"strconv"
)

func printSplit(content string) {
	log.Println("")
	log.Printf("-------------------%s-------------------", content)
	log.Println("")
}

func (rf *Raft) PrintLog(content string, color string) {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	logStr := selectColor(color) + "[Server " + strconv.Itoa(rf.me) + "]" + content + "\033[0m"
	log.Println(logStr)
}

func (rf *Raft) PrintState(content string) {

}

// locked 只能在有锁的情况下调用
// 打印当前server的全部日志
func (rf *Raft) PrintRfLog() {
	rf.PrintLog("[LOG]"+getLogStr(rf.log), "red")
}

// 返回日志片段的字符串
func getLogStr(entries []LogEntry) string {
	logStr := "["
	for i := 0; i < len(entries); i++ {
		logStr += strconv.Itoa(entries[i].Term) + " "
	}
	logStr += "]"
	return logStr
}

func getAppendEntriesRPCStr(args *AppendEntriesArgs, reply *AppendEntriesReply) string {
	str1 := fmt.Sprintf("[Leader term %d], [Leader Id %d], [prevLogIndex %d], [prevLogTerm %d], [LeaderCommit %d], [Entries %s]", args.Term, args.LeaderId, args.PrevLogIndex, args.PrevLogTerm, args.LeaderCommit, getLogStr(args.Entries))
	str2 := fmt.Sprintf("[Follower term %d], [Follower Id %d], [Success %t], [XTerm %d], [XIndex %d], [XLen %d]", reply.Term, reply.ServerId, reply.Success, reply.XTerm, reply.XIndex, reply.XLen)
	return str1 + " || " + str2
}

func getRoleStr(role int) string {
	roleStr := ""
	switch role {
	case 0:
		roleStr = "Follower"
	case 1:
		roleStr = "Candidate"
	case 2:
		roleStr = "Leader"
	default:
		roleStr = "Unknown"
	}
	return roleStr
}

func selectColor(inputColor string) string {
	color := "\033[0m"
	switch inputColor {
	case "red":
		color = "\033[31m"
	case "green":
		color = "\033[32m"
	case "yellow":
		color = "\033[33m"
	case "blue":
		color = "\033[34m"
	case "purple":
		color = "\033[35m"
	case "skyblue":
		color = "\033[36m"
	case "default":
		color = "\033[0m"
	default:
		color = "\033[0m"
	}
	return color
}
