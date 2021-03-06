package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"replay2/tb"
	"sync"
	"sync/atomic"
	"time"
)

var cocurrency = flag.Uint64("c", 20, "cocurrency request to lanch")                         //并发发包的微线程数（可以理解为客户端数）
var totalRequest = flag.Uint64("t", 40, "total request to issue")                            //总请求数
var pkgfile = flag.String("p", "", "pkg file to send")                                       //二进制文件
var proto = flag.String("f", "udp", "udp or tcp")                                            //协议
var svrAddress = flag.String("s", "", "server addr, ie, 192.168.0.1:9981")                   //服务器地址和端口
var deadline = flag.Int64("d", 200, "socket read/write timeout in ms")                       //发包超时时间
var dumpRespone = flag.Bool("v", false, "dump response")                                     //是否打印收包buf
var cocurencyPrintCycle = flag.Uint64("q", 0, "cocurrency print cycle, mearsured in second") //多久打印一次结果
var dumpError = flag.Bool("e", false, "dump error or not")                                   //是否打印错误

var pkgToSend []byte //发包内容

func init() {
	flag.Parse()
	if *pkgfile == "" || *svrAddress == "" {
		flag.Usage()
		fmt.Printf("\nexmaple usage: %s -s 192.168.0.1:9981 -t 500 -c 20 -p pkg.bin\n\n", os.Args[0])
		os.Exit(-1)
	}

	var err error
	if pkgToSend, err = ioutil.ReadFile(*pkgfile); err != nil {
		panic(err)
	}

	if *proto != "udp" && *proto != "tcp" {
		fmt.Println("proto family can only be udp or tcp")
		flag.Usage()
		os.Exit(-1)
	}
}

type taskError struct {
	taskId uint64 //任务id
	err    error  //具体错误信息
}

func (te taskError) String() string {
	return fmt.Sprintf("task(%d), error(%s)", te.taskId, te.err.Error())
}

type bencher struct {
	lock        sync.Mutex
	dialErrCnt  int64 //连接错误数
	dialErrs    []taskError
	writeErrCnt int64 //发包错误数
	writeErrs   []taskError
	readErrCnt  int64 //收包错误数
	readErrs    []taskError
	successCnt  int64 //成功数
}

func (b *bencher) Setup(id int64) {

}

/* 发包回调 */
func (b *bencher) Do(id uint64, _ int64) {
	buf := make([]byte, 4096)
	resCnt := 0

	conn, err := net.Dial(*proto, *svrAddress)
	if err != nil {
		atomic.AddInt64(&b.dialErrCnt, 1)
		b.lock.Lock()
		b.dialErrs = append(b.dialErrs, taskError{id, err})
		b.lock.Unlock()
		goto quit
	}
	defer conn.Close()

	if err = conn.SetDeadline(time.Now().Add(time.Duration(*deadline) * time.Millisecond)); err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}

	if _, err = conn.Write(pkgToSend); err != nil {
		atomic.AddInt64(&b.writeErrCnt, 1)
		b.lock.Lock()
		b.writeErrs = append(b.writeErrs, taskError{id, err})
		b.lock.Unlock()
		goto quit
	}

	if resCnt, err = conn.Read(buf); err != nil {
		b.lock.Lock()
		b.readErrs = append(b.readErrs, taskError{id, err})
		b.lock.Unlock()
		atomic.AddInt64(&b.readErrCnt, 1)
		goto quit
	}

	if *dumpRespone {
		fmt.Println(resCnt, ":", buf[:resCnt])
	}

	b.lock.Lock()
	atomic.AddInt64(&b.successCnt, 1)
	b.lock.Unlock()

quit:
	return
}

/* 数据信息回调 */
func (b *bencher) Report(dumpError bool, duration time.Duration) {
	total := b.dialErrCnt + b.writeErrCnt + b.successCnt + b.readErrCnt
	fmt.Printf("============================\n")
	fmt.Printf("bench result:\n\n")

	if dumpError {
		if len(b.dialErrs) > 0 {
			fmt.Println("dial errors:")
			for _, e := range b.dialErrs {
				fmt.Println("\t", e)
			}
			fmt.Println()
		}

		if len(b.readErrs) > 0 {
			fmt.Println("read errors:")
			for _, e := range b.readErrs {
				fmt.Println("\t", e)
			}
			fmt.Println()
		}

		if len(b.writeErrs) > 0 {
			fmt.Println("write errors:")
			for _, e := range b.writeErrs {
				fmt.Println("\t", e)
			}
			fmt.Println()
		}
	}

	fmt.Printf("total test          : %d\n", total)
	fmt.Printf("  [dial]  error cnt : %d\n", b.dialErrCnt)
	fmt.Printf("  [read]  error cnt : %d\n", b.readErrCnt)
	fmt.Printf("  [write] error cnt : %d\n", b.writeErrCnt)
	fmt.Printf("  success   cnt     : %d\n", b.successCnt)
	fmt.Printf("\n\n\tsuccess rate: %.2f%%\n", 100*(float64(b.successCnt)/float64(total)))

	nano := float64(duration)
	second := nano / float64(time.Second)

	fmt.Printf("\n\n\tqps: %.2f/s\n", float64(b.successCnt)/second)
}

func main() {
	benchlb := tb.NewBencher(*cocurrency)                             //底层携程框架
	b := new(bencher)                                                 //回调接口
	duration := benchlb.Start(b, *totalRequest, *cocurencyPrintCycle) //启动

	fmt.Printf("\nbench take %s\n", duration)
	b.Report(*dumpError, duration)
}
