package raft

//
// this is an outline of the API that raft must expose to
// the service (or tester). see comments below for
// each of these functions for more details.
//
// rf = Make(...)
//   create a new Raft server.
// rf.Start(command interface{}) (index, term, isleader)
//   start agreement on a new log entry
// rf.GetState() (term, isLeader)
//   ask a Raft for its current term, and whether it thinks it is leader
// ApplyMsg
//   each time a new entry is committed to the log, each Raft peer
//   should send an ApplyMsg to the service (or tester)
//   in the same server.
//

import (
	"fmt"
	"labrpc"
	"math/rand"
	"sync"
	"time"
)

// import "bytes"
// import "encoding/gob"

const (
	FOLLOWER  = 0
	CANDIDATE = 1
	LEADER    = 2

	HEARTBEAT_CYCLE   = 200 // 200ms, 5 heartbeat per second
	ELECTION_MIN_TIME = 400
	ELECTION_MAX_TIME = 600
)

//
// as each Raft peer becomes aware that successive log entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make().
//
type ApplyMsg struct {
	Index       int
	Command     interface{}
	UseSnapshot bool   // ignore for lab2; only used in lab3
	Snapshot    []byte // ignore for lab2; only used in lab3
}

//
// A Go object implementing a single Raft peer.
//
type Raft struct {
	mu        sync.Mutex          // Lock to protect shared access to this peer's state
	peers     []*labrpc.ClientEnd // RPC end points of all peers
	persister *Persister          // Object to hold this peer's persisted state
	me        int                 // this peer's index into peers[]

	// Your data here (2A, 2B, 2C).
	// Look at the paper's Figure 2 for a description of what
	// state a Raft server must maintain.

	//Updated on stable storage before responding to RPCs
	currentTerm int
	votedFor    int
	logEntries  []LogEntry

	//Valatile state on Leader
	nextIndex  []int
	matchIndex []int

	//Valatile state on all servers
	commitIndex int
	lastApplied int

	status       int
	voteAcquired int

	heartBeatCh      chan struct{}
	giveVoteCh       chan struct{}
	becomeLeaderCh   chan struct{}
	becomeFollowerCh chan struct{}
	revCommand       chan struct{}
	stop             chan struct{}
	stateChangeCh  chan struct{}
	electionTimer    *time.Timer

	applyCh chan ApplyMsg
}

// return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	var term int
	var isleader bool
	// Your code here (2A).
	isleader = false
	if rf.status == 2 {
		isleader = true
	}
	term = rf.currentTerm
	return term, isleader
}

//
// save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
//
func (rf *Raft) persist() {
	// Your code here (2C).
	// Example:
	// w := new(bytes.Buffer)
	// e := gob.NewEncoder(w)
	// e.Encode(rf.xxx)
	// e.Encode(rf.yyy)
	// data := w.Bytes()
	// rf.persister.SaveRaftState(data)
}

//
// restore previously persisted state.
//
func (rf *Raft) readPersist(data []byte) {
	// Your code here (2C).
	// Example:
	// r := bytes.NewBuffer(data)
	// d := gob.NewDecoder(r)
	// d.Decode(&rf.xxx)
	// d.Decode(&rf.yyy)
	if data == nil || len(data) < 1 { // bootstrap without any state?
		return
	}
}

type LogEntry struct {
	Command interface{}
	Term    int
}

//
// example RequestVote RPC arguments structure.
// field names must start with capital letters!
//
type RequestVoteArgs struct {
	// Your data here (2A, 2B).
	Term         int
	CandidatedId int
	LastLogIndex int
	LastLogTerm  int
}

//
// example RequestVote RPC reply structure.
// field names must start with capital letters!
//
type RequestVoteReply struct {
	// Your data here (2A).
	Term        int
	VoteGranted bool
}

//
// example RequestVote RPC handler.
//
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here (2A, 2B).
	rf.mu.Lock()
	defer rf.mu.Unlock()
	if rf.currentTerm > args.Term {
		reply.VoteGranted = false
		reply.Term = rf.currentTerm
		return
	}

	if rf.currentTerm < args.Term {
		rf.currentTerm = args.Term

		rf.updateStateTo(FOLLOWER)
		//妈的咋突然少了段代码~~ 这里要变为follower状态
		//var wg sync.WaitGroup
		//wg.Add(1)
		go func(){
		//	defer wg.Done()
			rf.stateChangeCh<- struct{}{}
		}()
		//wg.Wait()

		//直接return，等待下一轮投票会导致活锁，比如node 1 ，2，3 。 node 1 加term为2，发请求给node2，3，term1。 node2，3更新term拒绝投票
		//return
	}

	//此处if 在 currentTerm < args.Term下必然成立，在currentTerm等于args.Term下不一定成立

	if rf.votedFor == -1 || rf.votedFor == args.CandidatedId  {
		//if candidate的log 至少 as up-to-date as reveiver's log
		lastLogIndex := len(rf.logEntries) - 1
		//fmt.Println(lastLogIndex,rf.me,rf.logEntries )
		lastLogTerm := rf.logEntries[len(rf.logEntries)-1].Term
		//fmt.Println(lastLogIndex,lastLogTerm , args.LastLogIndex,args.LastLogTerm)
		if lastLogTerm < args.LastLogTerm || (lastLogTerm == args.LastLogTerm && lastLogIndex <= args.LastLogIndex) {
			rf.votedFor = args.CandidatedId
			reply.Term = rf.currentTerm
			reply.VoteGranted = true
			fmt.Printf("[Term %d],Node %d Reply 值为%v. Term= %d , lastIndex = %d <= args.lastLogIndex %d\n",rf.currentTerm,rf.me,reply, args.LastLogTerm,lastLogIndex,args.LastLogIndex)
			if rf.status == FOLLOWER {
				go func() { rf.giveVoteCh <- struct{}{} }()
			}
			return
		}
		fmt.Println(lastLogIndex,lastLogTerm , args.LastLogIndex,args.LastLogTerm)
	}

	reply.Term = rf.currentTerm
	reply.VoteGranted = false
	fmt.Printf("[Term %d] Node %d Reply 值为%v,rf.votefor=%d,\n",rf.currentTerm,rf.me,reply,rf.votedFor)


}

// 异步来发送选举请求
func (rf *Raft) broadcastVoteReq() {

	for i  := range rf.peers {
		if i == rf.me {
			continue
		}

		go func(server int) {
			args := RequestVoteArgs{}
			args.Term = rf.currentTerm
			args.CandidatedId = rf.me
			args.LastLogIndex = len(rf.logEntries) - 1
			args.LastLogTerm = rf.logEntries[args.LastLogIndex].Term

			//rf.mu.Lock()
			//defer rf.mu.Unlock()
			// **** 在这里上锁将引入concurrency bug，发送一个请求由于网络延迟得不到响应，别的请求都无法发送，导致任何一次选举都无法成功 ****
			reply := RequestVoteReply{}
			DPrintf("[Term %d]: Node %d issues request vote to %d \n",
				rf.currentTerm, rf.me, server)
			if rf.sendRequestVote(server, &args, &reply) && rf.status == CANDIDATE {
				DPrintf("[Term %d]: Node %d issues 开始判断投票返回结果 to %d \n",
					rf.currentTerm, rf.me, server)
				//TODO : 应该判断收到的response的term，不搭理旧term的返回结果
				if reply.Term > rf.currentTerm {
					rf.currentTerm = reply.Term
					rf.becomeFollowerCh <- struct{}{}
					fmt.Println("请求投票的return term更大")
				} else if reply.Term < rf.currentTerm {
					fmt.Println(reply.Term)
					DPrintf("[Term %d]: 旧%d返回值，不理会\n", rf.currentTerm, reply.Term)
					return
				} else {
					if reply.VoteGranted == true {
						DPrintf("[Node %d] receive vote from %d\n", rf.me, server)
						rf.mu.Lock()
						rf.voteAcquired++
						rf.mu.Unlock()
						if rf.voteAcquired > len(rf.peers)/2 && rf.status != LEADER {
							DPrintf("In term %d: Node %d get %d\n",
								rf.currentTerm, rf.me, rf.voteAcquired)
							rf.becomeLeaderCh <- struct{}{}
						}
					} else {
						DPrintf("[Node %d] did not receive vote from %d\n", rf.me, server)

					}
				}
			} else {
				DPrintf("[Term %d], Node %d to Server %d send vote req failed.\n", rf.currentTerm, rf.me, server)
			}
		}(i)
	}
}

//
// example code to send a RequestVote RPC to a server.
// server is the index of the target server in rf.peers[].
// expects RPC arguments in args.
// fills in *reply with RPC reply, so caller should
// pass &reply.
// the types of the args and reply passed to Call() must be
// the same as the types of the arguments declared in the
// handler function (including whether they are pointers).
//
// The labrpc package simulates a lossy network, in which servers
// may be unreachable, and in which requests and replies may be lost.
// Call() sends a request and waits for a reply. If a reply arrives
// within a timeout interval, Call() returns true; otherwise
// Call() returns false. Thus Call() may not return for a while.
// A false return can be caused by a dead server, a live server that
// can't be reached, a lost request, or a lost reply.
//
// Call() is guaranteed to return (perhaps after a delay) *except* if the
// handler function on the server side does not return.  Thus there
// is no need to implement your own timeouts around Call().
//
// look at the comments in ../labrpc/labrpc.go for more details.
//
// if you're having trouble getting RPC to work, check that you've
// capitalized all field names in structs passed over RPC, and
// that the caller passes the address of the reply struct with &, not
// the struct itself.
//
func (rf *Raft) sendRequestVote(server int, args *RequestVoteArgs, reply *RequestVoteReply) bool {
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
	return ok
}

type AppendEntriesArgs struct {
	// Your data here (2A, 2B).
	Term         int
	LeaderId     int
	PrevLogIndex int
	PrevLogTerm  int
	Entries      []LogEntry
	LeaderCommit int
}

type AppendEntriesReply struct {
	// Your data here (2A).
	Term    int
	Success bool
}

func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	// Your code here (2A, 2B).

	//TODO : appendentrie还有问题，会删除不该删除的日志，旧的append比新的append请求后来
	rf.mu.Lock()
	defer rf.mu.Unlock()
	//这种情况下当做没收到心跳
	if rf.currentTerm > args.Term {
		reply.Success = false
		reply.Term = rf.currentTerm
		return
	}

	if rf.currentTerm < args.Term {
		rf.currentTerm = args.Term
		//此处不需要下面这段逻辑的原因是：leader或者candidate收到有效heartbeat都会变为follower
		//if rf.status != FOLLOWER {
		//	go func() { rf.becomeFollowerCh <-struct{}{}}()
		//}
		////TODO ：此处可优化， becomeFollowerCh带一个返回信息的channel，表示loop已经进去follow状态初始化完毕，可投票
		//time.Sleep(50 * time.Millisecond)
	}

	//PS:把这段代码放在最后会导致bug，因为如果当前节点log entries没有arg.PrevLogIndex数据，则直接返回false，会导致server不做出收到心跳后的正确反应
	//收到来自有效leader的心跳
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		rf.heartBeatCh <- struct{}{}
	}()
	wg.Wait()

	//如果当前节点log entries没有arg.PrevLogIndex数据，则直接返回false
	if args.PrevLogIndex > len(rf.logEntries)-1 {
		reply.Success = false
		reply.Term = rf.currentTerm
		return
	} else {
		//比较当前节点log entries中的term是否与 arg中一致
		preLogTerm := rf.logEntries[args.PrevLogIndex].Term
		if args.PrevLogTerm != preLogTerm {
			//term不一致
			reply.Success = false
			reply.Term = rf.currentTerm
			return
		} else {
			//term一致,删除之后不一样的内容，加入entries
			reply.Success = true
			reply.Term = rf.currentTerm

			//
			// 这样直接将log加入会引入bug，比如新的请求比旧的先来且二者都是合法的，旧的请求会覆盖新的请求，导致新请求部分数据丢失
			// rf.logEntries = append(rf.logEntries[:args.PrevLogIndex+1] , args.Entries...)
			// 正确的做法是：加入当前没有的内容

			//for i := args.PrevLogIndex + 1 ; i < len(rf.logEntries);i++{
			//	for j:= 0; j < len(args.Entries);j++{
			//
			//	}
			//}
			for i, _ := range args.Entries {
				//先判断rf.logEntries在 PrevLogIndx + i + 1 的位置是否有值，如有，是否相同
				pos := args.PrevLogIndex + i + 1 //当前args.entrise要写入的位置
				if len(rf.logEntries)-1 < pos {
					rf.logEntries = append(rf.logEntries, args.Entries[i:]...)
					break
				} else if rf.logEntries[pos].Term == args.Entries[i].Term {
					continue
				} else {
					rf.logEntries = append(rf.logEntries[:pos], args.Entries[i:]...)
					break
				}
			}

			//更新当前节点的commit index
			if args.LeaderCommit > rf.commitIndex {
				idx := rf.commitIndex + 1
				if args.LeaderCommit > (len(rf.logEntries) - 1) {
					rf.commitIndex = len(rf.logEntries) - 1
				} else {
					rf.commitIndex = args.LeaderCommit
				}
				//此处存在并发bug，2个Goroutine提交，后启动的可能比旧的提交快
				var wg sync.WaitGroup
				wg.Add(1)
				go func(id int, commitIndex int) {
					defer wg.Done()
					//rf.mu.Lock()
					//defer rf.mu.Unlock()
					DPrintf("[Term %d] Node %d 提交 :从 %d 到 %d  \n", rf.currentTerm, rf.me, id, commitIndex)
					for ; id <= commitIndex; id++ {
						//fmt.Println(rf.me, id , commitIndex)

						DPrintf("[Term %d] Node %d 提交  %v\n", rf.currentTerm, rf.me, ApplyMsg{Index: id, Command: rf.logEntries[id].Command})
						DPrintf("[Term %d] Node %d, log:%v \n", rf.currentTerm, rf.me, rf.logEntries)
						rf.applyCh <- ApplyMsg{Index: id, Command: rf.logEntries[id].Command}
					}

				}(idx, rf.commitIndex)
				wg.Wait()
			}
		}

	}

	return

	//	preLogIndex, preLogTerm := 0, 0
	//	if args.PrevLogIndex < len(rf.logEntries) {
	//		preLogIndex = args.PrevLogIndex
	//		preLogTerm = rf.logEntries[preLogIndex].Term
	//	}
	//
	//	if preLogIndex == args.PrevLogIndex && preLogTerm == args.PrevLogTerm {
	//		DPrintf("anbo  安博 [Term %d] preLogIndex %d, args.PrevLogIndex %d preLogTerm %d args.PrevLogTerm %d,,%v \n",rf.currentTerm,
	//			preLogIndex,args.PrevLogIndex,preLogTerm,args.PrevLogTerm,args.Entries)
	//
	//		if len(rf.logEntries)-1 < preLogIndex + len(args.Entries)  {
	//			rf.logEntries = append(rf.logEntries[:preLogIndex+1], args.Entries...)
	//		}
	//
	//		reply.Success = true
	//		reply.Term = rf.currentTerm
	//		if args.LeaderCommit > rf.commitIndex {
	//			idx := rf.commitIndex + 1
	//			if args.LeaderCommit > (len(rf.logEntries)-1) {
	//				rf.commitIndex = len(rf.logEntries) - 1
	//			} else {
	//				rf.commitIndex = args.LeaderCommit
	//			}
	//			go func(id int, commitIndex int) {
	//				//rf.mu.Lock()
	//				//defer rf.mu.Unlock()
	//				for ; id <= commitIndex; id++ {
	//					fmt.Println(rf.me, id , commitIndex)
	//					DPrintf("[Term %d] Node %d, log:%v index：%d\n", rf.currentTerm, rf.me, rf.logEntries,id)
	//					DPrintf("[Term %d] Node %d 提交  %v\n", rf.currentTerm, rf.me, ApplyMsg{Index: id, Command: rf.logEntries[id].Command})
	//
	//					rf.applyCh <- ApplyMsg{Index: id, Command: rf.logEntries[id].Command}
	//
	//
	//				}
	//
	//			}(idx,rf.commitIndex)
	//
	//		}
	//	} else {
	//		reply.Success = false
	//		reply.Term = rf.currentTerm
	//		if len(rf.logEntries)-1 >= args.PrevLogIndex {
	//		     DPrintf("[Term %d] Node %d, 清空原log:%v -> %v \n",rf.currentTerm,rf.me, rf.logEntries, rf.logEntries[:preLogIndex])
	//		//	if preLogIndex > 1 {
	//		   rf.logEntries = rf.logEntries[: args.PrevLogIndex]
	//			//}
	//
	//			DPrintf("[Term %d] preLogIndex %d, args.PrevLogIndex %d preLogTerm %d args.PrevLogTerm %d \n清空原log:%v -> %v \n",rf.currentTerm,
	//				preLogIndex,args.PrevLogIndex,preLogTerm,args.PrevLogTerm)
	//
	////			DPrintf("[Term %d] Node %d, 清空原log:%v -> %v \n",rf.currentTerm,rf.me ,rf.logEntries, rf.logEntries[:preLogIndex])
	//		}
	//
	//	}

}

// 异步来发送心跳或者log
func (rf *Raft) broadcastAppendEntries() {
	//TODO：这里需要判断是否需要发送带有信息的请求

	//Bug： 妈的调了一晚上，如果从前往后找，会导致之前的log永远无法提交,如果每次提交都会阻塞呢？
	for n := len(rf.logEntries) - 1; n > rf.commitIndex; n-- {
		count := 0
		for peer := range rf.peers {
			if peer == rf.me {
				continue
			}
			if rf.matchIndex[peer] >= n {
				count++
			}
		}

		if count >= len(rf.peers)/2 && rf.logEntries[n].Term == rf.currentTerm {
			wait := make(chan int)
			go func(commitIndex int, n int) {
				DPrintf("[Term %d] Node %d 提交 :从 %d 到 %d  \n", rf.currentTerm, rf.me, commitIndex+1, n)
				for index := commitIndex + 1; index <= n; index++ {
					rf.applyCh <- ApplyMsg{Index: index, Command: rf.logEntries[index].Command}
					DPrintf("[Term %d] Node %d 提交  %v\n", rf.currentTerm, rf.me, ApplyMsg{Index: index, Command: rf.logEntries[index].Command})
					DPrintf("[Term %d] Node %d, log:%v \n", rf.currentTerm, rf.me, rf.logEntries)
				}
				wait <- 1
			}(rf.commitIndex, n)
			<-wait
			rf.commitIndex = n
			break
		}

	}

	for i := range rf.peers {
		if i == rf.me {
			continue
		}
		go func(server int) {
			//fmt.Printf("Node %d, server %d, index %d \n",rf.me,server,rf.nextIndex[server] - 1)
			args := AppendEntriesArgs{}
			args.Term = rf.currentTerm
			args.LeaderId = rf.me

			args.PrevLogIndex = rf.nextIndex[server] - 1

			args.PrevLogTerm = rf.logEntries[args.PrevLogIndex].Term

			args.LeaderCommit = rf.commitIndex

			if len(rf.logEntries) > rf.nextIndex[server] {
				args.Entries = rf.logEntries[rf.nextIndex[server]:]
			}

			DPrintf("[Term %d] Node %d broadcast heartBeat to %d, args : %v\n", rf.currentTerm, rf.me, server, args)

			var reply AppendEntriesReply
			if rf.sendAppendEntries(server, &args, &reply) && rf.status == LEADER {
				//TODO : 应该判断收到的response的term，不搭理旧term的返回结果
				if reply.Term < rf.currentTerm {
					return
				} else if reply.Term > rf.currentTerm {
					rf.currentTerm = reply.Term
					rf.becomeFollowerCh <- struct{}{}
					return
				} else if reply.Success == true {
					//BUG : 此处用 f.nextIndex[server] += len(arg.entries) 会引入bug？比如2次发送同样的请求会导致加2次，要保证RPC的幂等性
					rf.nextIndex[server] = args.PrevLogIndex + len(args.Entries) + 1
					rf.matchIndex[server] = rf.nextIndex[server] - 1
					//	fmt.Printf("master更新 %d 节点 nextIndex 和 matchIndex为 %d,%d\n",server,rf.nextIndex[server],rf.matchIndex[server])

				} else {
					// 重设相关变量
					//if rf.nextIndex[server] == 1{
					//	fmt.Println(args.PrevLogIndex,args.PrevLogTerm)
					//}
					rf.nextIndex[server] = args.PrevLogIndex
				}

				//在此处加入设置commitIndex的逻辑代码

			}

		}(i)
	}
}

func (rf *Raft) sendAppendEntries(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)
	return ok
}

//
// the service using Raft (e.g. a k/v server) wants to start
// agreement on the next command to be appended to Raft's log. if this
// server isn't the leader, returns false. otherwise start the
// agreement and return immediately. there is no guarantee that this
// command will ever be committed to the Raft log, since the leader
// may fail or lose an election.
//
// the first return value is the index that the command will appear at
// if it's ever committed. the second return value is the current
// term. the third return value is true if this server believes it is
// the leader.
//
func (rf *Raft) Start(command interface{}) (int, int, bool) {
	index := -1
	term := -1
	isLeader := false

	// Your code here (2B).
	if rf.status == LEADER {
		rf.mu.Lock()
		index = len(rf.logEntries)
		logEntry := LogEntry{Command: command, Term: rf.currentTerm}
		rf.logEntries = append(rf.logEntries, logEntry)
		rf.mu.Unlock()
		term = rf.currentTerm
		isLeader = true
		go func() {
			rf.revCommand <- struct{}{}
		}()
		DPrintf("Node %d,执行命令%d\n", rf.me, command)
	}
	return index, term, isLeader

}

//
// the tester calls Kill() when a Raft instance won't
// be needed again. you are not required to do anything
// in Kill(), but it might be convenient to (for example)
// turn off debug output from this instance.
//
func (rf *Raft) Kill() {
	// Your code here, if desired.

	go func() {
		fmt.Println("kill调用")
		rf.stop <- struct{}{}
	}()
}

func (rf *Raft) beginElection() {

	rf.currentTerm += 1
	rf.votedFor = rf.me
	rf.voteAcquired = 1

	DPrintf("[Term %d] Node %d begin election\n", rf.currentTerm, rf.me)

	//重置election timeout，candidate开始选举，发送选举必须异步，以免选举时间过长，阻塞其他事件发生
	rf.resetTimer()
	rf.broadcastVoteReq()
}

func (rf *Raft) updateStateTo(state int) {

	stateDesc := []string{"FOLLOWER", "CANDIDATE", "LEADER"}
	preState := rf.status
	switch state {
	case FOLLOWER:
		rf.status = FOLLOWER
		rf.votedFor = -1    // prepare for next election
		rf.voteAcquired = 0 // prepare for next election
		DPrintf("[Term %d]: Node %d:  transfer from %s to %s,votefor = %d\n",
			rf.currentTerm, rf.me, stateDesc[preState], stateDesc[rf.status],rf.votedFor)
	case CANDIDATE:
		rf.status = CANDIDATE
		DPrintf("[Term %d]: Node %d:  transfer from %s to %s\n",
			rf.currentTerm, rf.me, stateDesc[preState], stateDesc[rf.status])
		rf.beginElection()
	case LEADER:
		rf.status = LEADER
		//初始化成为Leader中的数据
		for i := range rf.peers {
			rf.nextIndex[i] = len(rf.logEntries)
			rf.matchIndex[i] = 0
		}

		DPrintf("[Term %d]: Node %d:  transfer from %s to %s\n",
			rf.currentTerm, rf.me, stateDesc[preState], stateDesc[rf.status])
	default:
		//DPrintf("Warning: invalid state %d, do nothing.\n", state)
	}

}

func (rf *Raft) resetTimer() {
	//rf.mu.Lock()
	//defer rf.mu.Unlock()

	duration := rf.getElectionTimeout()

	DPrintf("[Term %d]:Node %d reset election timeout %d ms\n", rf.currentTerm, rf.me, duration/1000000)
	rf.electionTimer.Reset(time.Duration(duration))
}

func (rf *Raft) getElectionTimeout() time.Duration {
	duration := time.Duration(ELECTION_MIN_TIME+rand.Int63n(ELECTION_MAX_TIME-ELECTION_MIN_TIME)) * time.Millisecond
	return duration
}

func (rf *Raft) loop() {
	stateDesc := []string{"FOLLOWER", "CANDIDATE", "LEADER"}
	stop := false
	for {
		if stop {
			fmt.Println("loop已停止")
			break
		}
		switch rf.status {
		case FOLLOWER:
			//
			// Follower只有做了下面任一事件时，
			//   1.  *投票给别人*
			//   2.  *收到来自当前leader的心跳*
			// 才会重置选举时间
			//
			rf.resetTimer()
			select {
			case <-rf.giveVoteCh: //Follower投票给Candidate才会收到 giveVoteCh（从选举处理函数发出信号）
			case <-rf.heartBeatCh: //Follower收到有效leader心跳才会收到 heartBeatCh（从心跳处理函数发出信号）
			case <-rf.stop:
				stop = true
			case <-rf.electionTimer.C:
				rf.updateStateTo(CANDIDATE)

			}
		case CANDIDATE:
			select {
			//
			// 成为Candidate后第一件事是开始选举，使用Goroutine并发发送选举请求，尽快返回，进入到select处理中
			//
			case <-rf.heartBeatCh: //这个chanel ： candidate收到有效Leader发送的心跳时，及从在心跳处理函数发生
				DPrintf("[Term %d]: Node %d status [%s] reveive heartBeat，reset election timer\n", rf.currentTerm, rf.me, stateDesc[rf.status])
				rf.updateStateTo(FOLLOWER)
			case <-rf.electionTimer.C:
				rf.beginElection()
			case <-rf.becomeFollowerCh: // 这个chanel：candidate异步发出选举请求后，有节点返回term比它大，及在选举结果处理函数发生
				rf.updateStateTo(FOLLOWER)
			case <-rf.becomeLeaderCh:
				rf.updateStateTo(LEADER)
				case<-rf.stateChangeCh:
			case <-rf.stop:
				stop = true
			}

		case LEADER:
			broadtimeout := time.NewTimer(HEARTBEAT_CYCLE * time.Millisecond)
			rf.broadcastAppendEntries()
			select {
			case <-rf.heartBeatCh:
				//DPrintf("[Node %d]: status [%s] reveive heartBeat，reset election timer\n", rf.me, stateDesc[rf.status])
				rf.updateStateTo(FOLLOWER)
			case <-rf.becomeFollowerCh: // 这个chanel：candidate异步发出选举请求后，有节点返回term比它大，及在选举结果处理函数发生
				rf.updateStateTo(FOLLOWER)
			//case <-rf.giveVoteCh:
			//
			case <-rf.stateChangeCh:
			case <-rf.revCommand:
			//     Leader收到command后做啥呢？
			//  发送appendEntries请求
			//
			//
			//
			case <-broadtimeout.C:
			case <-rf.stop:
				stop = true
			}

		}

	}
}

//
// the service or tester wants to create a Raft server. the ports
// of all the Raft servers (including this one) are in peers[]. this
// server's port is peers[me]. all the servers' peers[] arrays
// have the same order. persister is a place for this server to
// save its persistent state, and also initially holds the most
// recent saved state, if any. applyCh is a channel on which the
// tester or service expects Raft to send ApplyMsg messages.
// Make() must return quickly, so it should start goroutines
// for any long-running work.
//
func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *Raft {

	rf := &Raft{}
	rf.peers = peers
	rf.persister = persister
	rf.me = me

	//为了高可用需要存储在稳定介质中的数据
	rf.votedFor = -1
	rf.status = FOLLOWER
	fmt.Printf("新建一个raft状态为 , %d", rf.status)
	rf.currentTerm = 0
	rf.logEntries = make([]LogEntry, 0, 10)
	rf.logEntries = append(rf.logEntries, LogEntry{Term: 0})

	// failure发生以后重新计算的数据
	rf.commitIndex = 0
	rf.lastApplied = 0 //状态机丢失，从头开始计算得到状态机

	//成为leader后重新计算的数据
	rf.nextIndex = make([]int, len(peers))
	for i := range rf.nextIndex {
		rf.nextIndex[i] = len(rf.logEntries)
	}
	rf.matchIndex = make([]int, len(peers))

	//TODO: 学习Go time包中定时器的使用
	duration := rf.getElectionTimeout()
	rf.electionTimer = time.NewTimer(duration)

	rf.giveVoteCh = make(chan struct{}, 1)
	rf.heartBeatCh = make(chan struct{}, 1)
	rf.becomeLeaderCh = make(chan struct{}, 1)
	rf.becomeFollowerCh = make(chan struct{}, 1)
	rf.revCommand = make(chan struct{}, 1) //TODO:此处是否需要使用有缓存的channel
	rf.stop = make(chan struct{}, 1)
	rf.stateChangeCh = make(chan struct{}, 1)
	rf.applyCh = applyCh
	// Your initialization code here (2A, 2B, 2C).

	go func() {
		rf.loop()
	}()

	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())

	return rf
}
