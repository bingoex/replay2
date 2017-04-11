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
	Cocurrency   uint64        // 并发数
	taskBucket   chan taskInfo // use make(chan int, Cocurrency) to simulate limited resources
	ticketBucket chan int      //只有从这里获取到ticket才能执行任务

	/* 打印的统计信息 */
	completedWork      uint64    //当前完成任务总数
	numOfWorkerRunning int64     //当前正在执行任务数
	lastReportTime     time.Time //最近一次打印报告时间
	lastCompleted      uint64    //上次完成的任务总数
	closed             uint64    //当前关闭的woker数
}

/* 简单工厂 */
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

	b.taskBucket = make(chan taskInfo, b.Cocurrency) //并发数个任务篮子(相当于任务队列)
	b.ticketBucket = make(chan int, b.Cocurrency)    //并发数个票据, 控制流入速度

	workder := func(task Task, id uint64) {
		for {
			tinfo, ok := <-b.taskBucket // 拿任务

			/* 关闭 */
			if false == ok {
				atomic.AddUint64(&b.closed, 1)
				break
			}

			atomic.AddInt64(&b.numOfWorkerRunning, 1)
			task.Do(id, tinfo.index) //执行回调，一般做发包操作

			atomic.AddUint64(&b.completedWork, 1)
			atomic.AddInt64(&b.numOfWorkerRunning, -1)

			b.ticketBucket <- Ticket //告诉管理层，我任务完成了，归还票据
		}
	}

	start := time.Now()
	for i := uint64(0); i < b.Cocurrency; i++ { //开启b.Cocurrency个协程并发处理任务
		go workder(task, i)
	}

	index := int64(0)
	nextIndex := func() int64 {
		index += 1
		return index
	}

	quitReport := make(chan bool)
	if updateInterval > 0 {
		go b.reportCocurrency(updateInterval, quitReport) // 每隔updateInterval报告一次数据
	}

	/* 初始化：给每个worker发一个任务 */
	go func() {
		for i := uint64(0); i < b.Cocurrency; i++ {
			b.taskBucket <- taskInfo{index: nextIndex()}
		}
	}()

	/* 塞任务 */
	for i := b.Cocurrency; i < total; i++ {
		<-b.ticketBucket //有人归还票据了，说明有协程闲下来了，塞任务给他吧
		b.taskBucket <- taskInfo{index: nextIndex()}
	}

	/* wait for all work to be completed */
	for atomic.LoadUint64(&b.completedWork) < total {
		runtime.Gosched() //让出CPU
	}

	close(b.taskBucket) //各协程会自动退出

	for atomic.LoadUint64(&b.closed) < b.Cocurrency {
		runtime.Gosched()
	}

	/* quit report */
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

			/* 时间、当前完成任务总数、当前并发woker数、当前interval的qps(任务数/s) */
			fmt.Printf("%s: completed tasks(%d), cocurrencye(%d), realtime qps(%0.2f)\n",
				time.Now(), curCompleted, b.numOfWorkerRunning,
				float64(delta)/(float64(elapsedTime)/float64(time.Second)))

			b.lastCompleted = curCompleted
			b.lastReportTime = now
		}
	}
}
