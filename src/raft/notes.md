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

- TODO: Start()中收到日志之后立即发送AE RPC [DONE], 性能问题

## lab2C

- persistent state: 在响应RPC之前persist
- votedFor 是否需要改

- 对于follower
  - 在handleAERPCRequest和handleRVRPCRequest释放锁之前persist()
- 对于leader
  - 当选的时候persist一下（有用吗）
  - Start收到日志的时候persist一下
- 对于candidate
  - candidate持有自己的votedFor然后重启怎么办


PASS
ok      6.5840/raft     130.978s
go test -race -run 2C  40.06s user 1.81s system 31% cpu 2:11.29 total



- TODO
  - leader稳定的时候如果接收大量日志会逐一发送给follower
  - leader 计算commit算法有误，commitIndex=0，matchIndex=[1 2 3]无法更新commitIndex
  - 防止选举压制，成为candidate之后重置自己的election timeout
    - 貌似不是选举压制问题，而是ticker中continue写成return了
    - 草

--- FAIL: TestFigure8Unreliable2C (47.44s)
config.go:601: one(9713) failed to reach agreement

## lab2D

- 修改日志的index储存方式，不再以下标作为索引，回归测试15遍

在 Lab 2D 中，测试人员定期调用 Snapshot()。在实验 3 中，您将编写一个调用 Snapshot() 的键/值服务器；快照将包含完整的键/值对表。服务层在每个对等点（不仅仅是领导者）上调用 Snapshot()。

索引参数指示快照中反映的最高日志条目。 Raft 应丢弃该点之前的日志条目。您需要修改 Raft 代码才能在仅存储日志尾部的情况下进行操作。

您需要实现本文中讨论的 InstallSnapshot RPC，它允许 Raft 领导者告诉落后的 Raft 对等方用快照替换其状态。您可能需要考虑 InstallSnapshot 应如何与图 2 中的状态和规则进行交互。

当follower的Raft代码收到InstallSnapshot RPC时，它可以使用applyCh将快照发送到ApplyMsg中的服务。 ApplyMsg 结构定义已包含您需要的字段（以及测试人员期望的字段）。请注意，这些快照只会推进服务的状态，而不会导致其向后移动（backwards）。

如果服务器崩溃，它必须从持久数据重新启动。您的 Raft 应该保留 Raft 状态和相应的快照。使用 persister.Save() 的第二个参数来保存快照。如果没有快照，则传递 nil 作为第二个参数。

- 使用 persister.Save() 的第二个参数来保存快照。如果没有快照，则传递 nil 作为第二个参数。
- 一个好的起点是修改您的代码，以便它能够仅存储从某个索引 X 开始的日志部分。最初，您可以将 X 设置为零并运行 2B/2C 测试。然后让Snapshot(index)丢弃index之前的日志，并设置X等于index。如果一切顺利，您现在应该通过第一个 2D 测试。
- 在单个 InstallSnapshot RPC 中发送整个快照。不要实现图 13 的偏移机制来分割快照。
- Raft 必须以允许 Go 垃圾收集器释放并重新使用内存的方式丢弃旧日志条目；这要求对被丢弃的日志条目没有可到达的引用（指针）。
- Even when the log is trimmed, your implemention still needs to properly send the term and index of the entry prior to new entries in AppendEntries RPCs; this may require saving and referencing the latest snapshot's lastIncludedTerm/lastIncludedIndex (consider whether this should be persisted).
- 即使日志被修剪，您的实现仍然需要在 AppendEntries RPC 中的新条目之前正确发送条目的term和索引；这可能需要保存并引用最新快照的lastIncludedTerm/lastIncludedIndex（考虑是否应该保留它）。

------
关于follower是否有可能比leader更快拍摄快照

> ---Term2
> F: 1 1 1 1 1 2
> L: 1 1 1 1 1 2
> --leader 将2设为commit，然后自己拍摄快照，但commit并未同步到F
> F 1 1 1 1 1 2
> L - - - - - -
> --F当选，旧leader成为拍照过快的follower
> L 1 1 1 1 1 2 3 3 3 3
> F - - - - - -
> --L(旧F)将matchIndex设置为[-1, -1, -1...], nextIndex设置为[10, 6, 6...]
> 日志匹配
> --假设代码有优化，leader连续接收日志后，nextIndex设置为[10, 10, 10]
> 日志不匹配，F返回冲突位置，即拍摄快照的位置
> follower无论拍摄多快，拍摄的都是commit的日志，即使有日志优化，leader也不会匹配commit之前的某一条日志
> 因此，**leader不会匹配snapshot中的日志**, 即leader不会匹配commit的日志（除了commit的最后一条）