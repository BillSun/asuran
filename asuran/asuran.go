package main

import (
	"github.com/benbearchen/asuran/dnsproxy"
	"github.com/benbearchen/asuran/net"
	"github.com/benbearchen/asuran/profile"
	"github.com/benbearchen/asuran/web/proxy"
	"github.com/benbearchen/asuran/util/cmd"

	"fmt"
	"time"
)

func usage() {
	fmt.Println(`web transparent proxy

proxy type:
  /
  /about
        visit http://localhost/about for proxy information
  /test/target.domain:port/path
        return content of 'http://target.domain:port/path' & runtime info
  /<any other path>
        purely proxy <any other path>

cmd:
  bench [target.domain:port/path]
        benchmark url http://target.domain:port/path in some goroutines,
        finally calc used/avg time.  default: http://localhost
`)
}

func bench(target string) {
	fmt.Println("begin bench to: " + target + " ...")

	succ := 0
	fail := 0
	var t time.Duration = 0
	for i := 0; i < 10 || t.Seconds() < 30; i++ {
		start := time.Now()
		resp, err := net.NewHttpGet(target)
		if err == nil {
			defer resp.Close()
			_, err := resp.ReadAll()
			if err == nil {
				succ++
				end := time.Now()
				t += end.Sub(start)
				continue
			}
		}

		fail++
	}

	fmt.Printf("succ: %d\n", succ)
	if succ > 0 {
		fmt.Println("used: " + t.String())
		fmt.Println("avg:  " + time.Duration(int64(t)/int64(succ)).String())
	}

	fmt.Printf("fail: %d\n", fail)
}

func benchN(target string) {
	c := make(chan int)
	n := 4
	for i := 0; i < n; i++ {
		go func() { bench(target); c <- 1 }()
	}

	for i := 0; i < n; i++ {
		<-c
	}
}

func main() {
	p := proxy.NewProxy()
	ipProfiles := profile.NewIpProfiles()
	p.BindUrlOperator(ipProfiles.OperatorUrl())
	p.BindProfileOperator(ipProfiles.OperatorProfile())
	p.BindDomainOperator(ipProfiles.OperatorDomain())

	go dnsproxy.DnsProxy(dnsproxy.NewPolicy(p.NewDomainOperator()))

	var c cmd.Command
	c.OpenConsole()
	for {
		fmt.Print("\n$ ")
		command := c.Read()
		if command == "exit" {
			return
		} else if command == "help" {
			usage()
		} else if rest, ok := cmd.CheckCommand(command, "bench"); ok {
			benchN("http://" + rest)
		} else if command != "" {
			usage()
			fmt.Println("UNKNOWN command: " + command)
		}
	}
}