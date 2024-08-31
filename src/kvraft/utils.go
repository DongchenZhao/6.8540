package kvraft

import (
	"log"
	"strconv"
)

func PrintLog(content string, color string, me int, isServer bool) {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	prefix := ""
	if isServer {
		prefix = "[lab3][Server " + strconv.Itoa(me) + "]"
	} else {
		prefix = "[lab3][Client " + strconv.Itoa(me) + "]"
	}
	logStr := selectColor(color) + prefix + strconv.Itoa(me) + "]" + content + "\033[0m"
	log.Println(logStr)
}

func selectColor(inputColor string) string {
	color := "\033[0m"
	switch inputColor {
	case "red":
		color = "\033[31m"
	case "green":
		color = "\033[32m"
	case "yellow":
		color = "\033[33m" // 描述rpc异常
	case "blue":
		color = "\033[34m" // 描述client-server交互，请求或结果
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
