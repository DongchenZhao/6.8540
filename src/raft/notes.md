## lab2A

- 测试人员要求Leader每秒发送心跳RPC不超过10次。
- 测试人员要求你的 Raft 在旧领导者失败后五秒内选举出新的领导者（如果大多数对等点仍然可以通信）。但请记住，如果出现分裂投票（如果数据包丢失或候选人不幸选择相同的随机退避时间，则可能会发生这种情况），领导者选举可能需要多轮。您必须选择足够短的选举超时（以及心跳间隔），以便即使需要多轮选举也很可能在不到五秒的时间内完成。

## lab2B

- 如果日志冲突，删除冲突日志和其后续所有日志
- 如果日志匹配，append AE RPC的entries里面自己没有的日志条目

- 加速日志回溯优化
    - If a follower does not have prevLogIndex in its log, it should return with conflictIndex = len(log) and conflictTerm = None.
    - If a follower does have prevLogIndex in its log, but the term does not match, it should return conflictTerm = log[prevLogIndex].Term, and then search its log for the first index whose entry has term equal to conflictTerm.
      - 例如
        - 0 1 2 3 4 5 6 7 8 9 10
        - 1 1 1 4 4 5 5 6 6 6 
        - 1 1 1 2 2 2 3 3 3 3 3
        - 假设leader从最后1个6开始
        - follower日志冲突，XTerm=3 XIndex=6,并且截断 9 10(最后两个日志)
        - leader找不到XTerm，所以设置nextIndex = conflictIndex，下次比较的是5-2（index=5处，prevLogIndex=5）
        - follower日志冲突，XTerm=2 XIndex=3，并截断 index=5及其之后的所有日志
        - leader找不到XTerm，所以设置nextIndex = conflictIndex，下次比较的是1-1（index=2处，prevLogIndex=2）
        - 日志匹配
    - Upon receiving a conflict response, the leader should first search its log for conflictTerm.
      - If it finds an entry in its log with that term, it should set nextIndex to be the one beyond the index of the last entry in that term in its log.
      - 例如
        - 4 4 4
        - 4 5 5 5 5 <- leader
      - 或者
        - 1 1 2 2 3 3 3 4 4 4 4 <- leader
        - 1 1 2 2 3 3 3 3 3 3 3 3 3
      - 设置为leader冲突term串的下一个term串的第一个，leader会发送冲突term串的最后一个
        - leader假设从第2个4开始（假设leader很短时间内被写入了4个4，收到第2个4才开始AE RPC，因为AE是周期性的）
        - 冲突，follower截断日志 1 1 2 2 3 3 3 3【最后1个3仍然是无效的】
        - leader收到冲突，设置prevLogIndex=最后一个3的index，entries=[4 4]
        - follower收到，日志追加，追加过程中覆盖掉了之前没有完全截断的最后一个3
        - 注意在这种情况下，follower返回的conflictIndex是没有用的（即leader具有conflict点所在term的term串，如果leader没有，则会跳过follower的这个term串继续）
      
      - If it does not find an entry with that term, it should set nextIndex = conflictIndex.


You'll want to have a separate long-running goroutine that sends
committed log entries in order on the applyCh. It must be separate,
since sending on the applyCh can block; and it must be a single
goroutine, since otherwise it may be hard to ensure that you send log
entries in log order. The code that advances commitIndex will need to
kick the apply goroutine; it's probably easiest to use a condition
variable (Go's sync.Cond) for this.

## lab2C