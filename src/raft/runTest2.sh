#!/bin/bash

# 清空目标目录
rm -rf /home/zdc/test2B/*
mkdir -p /home/zdc/test2B

# 运行测试100次
for run in {1..20}; do
    echo "start test No.${run}"
    timestamp=$(date +%s)
    log_file="/home/zdc/test2B/${run}-${timestamp}.txt"
    go test -race -run 2B > "$log_file" 2>&1

    # 检查结果是否包含FAIL
    tail -n 10 "$log_file" | grep -q "FAIL"
    if [ $? -eq 0 ]; then
        mv "$log_file" "/home/zdc/test2B/${run}-${timestamp}-fail.log"
    fi
done
