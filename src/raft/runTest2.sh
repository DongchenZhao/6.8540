#!/bin/bash

# 清空目标目录
#rm -rf /home/zdc/test2B/*
#mkdir -p /home/zdc/test2B

testName="test2A"
testcase="2A"
folderName=${testName}

for run in {11..21}; do
    echo "start test No.${run}"
    timestamp=$(date +%s)
    log_file="/home/zdc/${folderName}/${run}-${timestamp}-${testName}.txt"
    go test -race -run ${testcase} > "$log_file" 2>&1

    # 检查结果是否包含FAIL
    tail -n 10 "$log_file" | grep -q "FAIL"
    if [ $? -eq 0 ]; then
        mv "$log_file" "/home/zdc/${folderName}/${run}-${timestamp}-${testName}-fail.txt"
    fi
done
