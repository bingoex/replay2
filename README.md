# replay2
a send pkg tool for stress testing version 2 <br>
发包压测工具版本2

- 支持并发（使用多协程）
- 流量控制：利用票据机制，控制生产者的速度。只有当woker消费完任务，才释放票据，生产者获得票据后才生产任务。
- 压测数据支持按间隔时间打印（打印信息：当前时间＋当前完成任务总数＋当前并发woker数＋当前interval的qps(任务数/s))
- 支持任务回调、最终打印回调(用户可定制)
- 最终打印（错误信息：连接、发包、收包错误数及其具体任务id、错误信息）
- 最终打印（打印信息：总任务数＋连接失败数＋发包失败数＋收包失败数＋成功数＋总qps（任务数/s））

## how to use 
 ```shell
 ./bench -h
 Usage of ./replay2:
   -c uint
        cocurrency request to lanch (default 20)
    -d int
        socket read/write timeout in ms (default 200)
    -e    dump error or not
    -f string
        udp or tcp (default "udp")
    -p string
        pkg file to send
    -q uint
        cocurrency print cycle, mearsured in second
    -s string
        server addr, ie, 192.168.0.1:9981
    -t uint
        total request to issue (default 40)
    -v    dump response
```



