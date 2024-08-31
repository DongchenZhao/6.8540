#time go test -race -run TestManyElections2A
#time go test -race -run 2A
#time go test -race -run 2B
#time go test -run 2B
#time go test -race -run 2C

#time go test -race -run TestBasicAgree2B
#time go test -race -run 2D
# 生成当前时间戳


#!/bin/bash

# Generate the current timestamp
timestamp=$(date "+%Y-%m-%d-%H-%M-%S")

# Define the log file name with the timestamp
logfile="/Users/zhaodongchen/Code/6.8540Logs/test_$timestamp.log"

# Run the go test command with race detection and redirect both stdout and stderr to the log file
time go test -race > "$logfile" 2>&1