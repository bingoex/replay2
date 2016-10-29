package tb

import (
	"fmt"
	"runtime"
	"sync/atomic"
	"time"
)

const (
	Ticket = 0
)

type Task interface {
	Do(id uint64, taskIndex int64)
}

type taskInfo struct {
	index int64
}

type Bencher struct {
	Cocurrency   uint64        // cocurrency work schedual one time
	taskBucket   chan taskInfo // use make(chan int, Cocurrency) to simulate limited resources
	ticketBucket chan int      //只有从这里获取到ticket才能执行任务

	// task statistics
	completedWork      uint64
	numOfWorkerRunning int64
	lastReportTime     time.Time
	lastCompleted      uint64
	closed             uint64
}

func NewBencher(cocurrency uint64) *Bencher {
	return &Bencher{
		Cocurrency: cocurrency,
	}
}

func (b *Bencher) Start(task Task, total uint64, updateInterval uint64) time.Duration {
	if total < b.Cocurrency {
		b.Cocurrency = total
	}

	fmt.Println("cocurrency:", b.Cocurrency)

	b.taskBucket = make(chan taskInfo, b.Cocurrency)
	b.ticketBucket = make(chan int, b.Cocurrency)

	workder := func(task Task, id uint64) {
		for { // work restlessly
			tinfo, ok := <-b.taskBucket // 拿任务

			// if chann closed stop worker
			if false == ok {
				atomic.AddUint64(&b.closed, 1)
				break
			}

			atomic.AddInt64(&b.numOfWorkerRunning, 1)
			task.Do(id, tinfo.index)

			atomic.AddUint64(&b.completedWork, 1)
			atomic.AddInt64(&b.numOfWorkerRunning, -1)

			// task finshed, return a ticket
			b.ticketBucket <- Ticket //告诉管理层，我任务完成了，归还票据
		}
	}

	start := time.Now()
	for i := uint64(0); i < b.Cocurrency; i++ { //开启b.Cocurrency个微线程并发处理任务
		go workder(task, i)
	}

	index := int64(0)
	nextIndex := func() int64 {
		index += 1
		return index
	}

	quitReport := make(chan bool)
	if updateInterval > 0 {
		go b.reportCocurrency(updateInterval, quitReport) // 每隔updateInterval则报告一次数据
	}

	// feed workers with task
	go func() {
		for i := uint64(0); i < b.Cocurrency; i++ {
			b.taskBucket <- taskInfo{index: nextIndex()}
		}
	}()

	// wait to start other tasks
	for i := b.Cocurrency; i < total; i++ {
		<-b.ticketBucket //有人归还票据了，说明有微线程闲下来了，塞任务给他吧
		b.taskBucket <- taskInfo{index: nextIndex()}
	}

	// wait for all work to be completed
	for atomic.LoadUint64(&b.completedWork) < total {
		runtime.Gosched() //让出CPU
	}

	close(b.taskBucket) //各微线程会自动退出

	for atomic.LoadUint64(&b.closed) < b.Cocurrency {
		runtime.Gosched()
	}

	// quit report
	if updateInterval > 0 {
		quitReport <- true
	}

	return time.Since(start)
}

func (b *Bencher) reportCocurrency(interval uint64, quit <-chan bool) {
	firstReport := true
	for {
		select {
		case <-quit:
			return

		case <-time.After(time.Second * time.Duration(interval)):
			if firstReport {
				b.lastReportTime = time.Now()
				atomic.StoreUint64(&b.lastCompleted, atomic.LoadUint64(&b.completedWork))
				firstReport = false
				fmt.Printf("%s: completed tasks(%d), cocurrencye(%d)\n", time.Now(),
					b.lastCompleted, b.numOfWorkerRunning)
				continue
			}

			now := time.Now()
			curCompleted := atomic.LoadUint64(&b.completedWork)

			elapsedTime := time.Since(b.lastReportTime)
			delta := curCompleted - b.lastCompleted

			fmt.Printf("%s: completed tasks(%d), cocurrencye(%d), realtime qps(%0.2f)\n",
				time.Now(), curCompleted, b.numOfWorkerRunning,
				float64(delta)/(float64(elapsedTime)/float64(time.Second)))

			b.lastCompleted = curCompleted
			b.lastReportTime = now
		}
	}
}
