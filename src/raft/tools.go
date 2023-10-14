package raft

import (
	"log"
	"strconv"
)

func (rf *Raft) PrintLog(content string, color string) {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	logStr := selectColor(color) + "[Server " + strconv.Itoa(rf.me) + "]" + content + "\033[0m"
	log.Println(logStr)
}

func (rf *Raft) PrintState(content string) {

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
