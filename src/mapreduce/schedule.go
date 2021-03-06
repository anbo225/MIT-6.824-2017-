package mapreduce

import (
	"fmt"
)

//
// schedule() starts and waits for all tasks in the given phase (Map
// or Reduce). the mapFiles argument holds the names of the files that
// are the inputs to the map phase, one per map task. nReduce is the
// number of reduce tasks. the registerChan argument yields a stream
// of registered workers; each item is the worker's RPC address,
// suitable for passing to call(). registerChan will yield all
// existing registered workers (if any) and new ones as they register.
//
func schedule(jobName string, mapFiles []string, nReduce int, phase jobPhase, registerChan chan string) {
	var ntasks int
	var n_other int // number of inputs (for reduce) or outputs (for map)
	switch phase {
	case mapPhase:
		ntasks = len(mapFiles)
		n_other = nReduce
	case reducePhase:
		ntasks = nReduce
		n_other = len(mapFiles)
	}

	fmt.Printf("Schedule: %v %v tasks (%d I/Os)\n", ntasks, phase, n_other)

	// All ntasks tasks have to be scheduled on workers, and only once all of
	// them have been completed successfully should the function return.
	// Remember that workers may fail, and that any given worker may finish
	// multiple tasks.
	//
	// TODO TODO TODO TODO TODO TODO TODO TODO TODO TODO TODO TODO TODO
	//

	work := make(chan int)
	done := make(chan bool)
	exit := make(chan bool)
	runTask := func(srv string) {
		for task := range work {
			taskargs := DoTaskArgs{
				JobName:       jobName,
				File:          mapFiles[task],
				Phase:         phase,
				TaskNumber:    task,
				NumOtherPhase: n_other,
			}
			if call(srv, "Worker.DoTask", &taskargs, new(struct{})) {
				done <- true
			} else {
				work <- task
			}
		}
	}

	go func() {
		for {
			select {
			case srv := <-registerChan:
				go runTask(srv)
			case <-exit:
				return
			}

		}
		for srv := range registerChan {
			go runTask(srv)
		}
	}()

	go func() {
		for task := 0; task < ntasks; task++ {
			fmt.Println(task)
			work <- task
		}

	}()

	for i := 0; i < ntasks; i++ {
		<-done
	}
	close(work)
	exit <- true

	// var wg sync.WaitGroup
	//从registerChande'dao依次分配各个任务
	// for i := 0; i < ntasks; i++ {
	// 	wg.Add(1)
	// 	taskargs := DoTaskArgs{
	// 		JobName:       jobName,
	// 		File:          mapFiles[i],
	// 		Phase:         phase,
	// 		TaskNumber:    i,
	// 		NumOtherPhase: n_other,
	// 	}
	// 	//使用goroutine并发执行一个任务
	// 	go func() {
	// 		//使用for循环来防止rpc调用失败
	// 		defer wg.Done()
	// 		for {
	// 			worker := <-registerChan
	// 			ok := call(worker, "Worker.DoTask", &taskargs, new(struct{}))
	// 			if ok == true {
	// 				go func() { registerChan <- worker }()
	// 				break
	// 			}
	// 		}
	//
	// 	}()
	//
	// }
	//
	// wg.Wait()

	fmt.Printf("Schedule: %v phase done\n", phase)
}
