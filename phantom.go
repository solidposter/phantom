package main

//
// Copyright (c) 2019 Tony Sarendal <tony@polarcap.org>
//
// Permission to use, copy, modify, and distribute this software for any
// purpose with or without fee is hereby granted, provided that the above
// copyright notice and this permission notice appear in all copies.
//
// THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
// WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
// MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
// ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
// WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
// ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
// OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
//

import (
	"flag"
	"fmt"
	"math/rand"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

var wg sync.WaitGroup
var totPkts, totDrops uint64

func main() {
	modePtr := flag.Bool("s", false, "set server mode")
	clntPtr := flag.Int("n", 1, "number of clients to run")
	pktsPtr := flag.Int("c", 1000, "number of packets to send per client")
	sizePtr := flag.Int("b", 512, "packet size")
	flag.Parse()

	if *modePtr {
		fmt.Print("Starting server mode, ")
		if len(flag.Args()) == 0 {
			udpbouncer("2222")
		} else {
			udpbouncer(flag.Args()[0])
		}
	}

	if len(flag.Args()) == 0 {
		fmt.Println("Specify server:port")
		return
	}
	fmt.Println("number of clients:", *clntPtr)
	if *pktsPtr < 1 {
		*pktsPtr = int(^uint(0) >> 1)
	}
	fmt.Println("packets per client:", *pktsPtr)
	fmt.Println("packet size:", *sizePtr)
	fmt.Println("server address:", flag.Args()[0])
	rand.Seed(time.Now().UnixNano())
	wg.Add(int(*clntPtr))

	ch := make(chan int)
	go statsprinter(ch,*clntPtr)	// not counted by wg, channel ch used to close

	tstart := time.Now()
	for i := 0; i < *clntPtr; i++ {
		go udpclient(flag.Args()[0],*pktsPtr, *sizePtr)
		time.Sleep(10 * time.Millisecond) // insert sleep to handle startup of many go routines
	}
	wg.Wait()

	t := time.Now()
	close(ch)
	fmt.Println("Runtime:", t.Sub(tstart), "Packets received:", totPkts, "Packets dropped:", totDrops)
}

func udpbouncer(port string) {
	fmt.Println("listening on port", port)
	pc, err := net.ListenPacket("udp","0.0.0.0:"+port)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	buffer := make([]byte, 4096)
	for {
		len,addr,err := pc.ReadFrom(buffer)
		if err != nil {
			fmt.Println(err)
			os.Exit(2)
		}
		pc.WriteTo(buffer[0:len], addr)
	}
}

func udpclient(addr string, numpkts int, pktsize int) {
	defer wg.Done()

	buffer := make([]byte, pktsize-28)	// subtract 20+8, IP+UDP header
	rand.Read(buffer)	// put random data into the buffer
	conn, err := net.Dial("udp",addr)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	for i := 0; i < numpkts; i++ {
		_, err = conn.Write(buffer)
		if err != nil {
			fmt.Println("write failed:",err)
			time.Sleep(10 * time.Millisecond) // chill a little
			continue
		}

		conn.SetReadDeadline(time.Now().Add(1000*time.Millisecond))
		_, err = conn.Read(buffer)
		if err != nil {
			fmt.Println("read failed:",err)
			atomic.AddUint64(&totDrops, 1)
		} else {
			atomic.AddUint64(&totPkts, 1)
		}
	}
}

func statsprinter(ch chan int, nclients int) {
	var c1,c2 uint64

	c1 = atomic.LoadUint64(&totPkts)
	for {
		select {
			case <-ch:
				return
			case <-time.After(1 * time.Second):
				c2 = atomic.LoadUint64(&totPkts)
				fmt.Print("pps: ",c2-c1," total drops: ",atomic.LoadUint64(&totDrops))
				fmt.Printf(" avg rtt: %.3f",1/float64(c2-c1)*1000*float64(nclients))
				fmt.Println("ms")
		}
		c1 = c2
	}
}

