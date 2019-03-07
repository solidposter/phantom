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
	"encoding/binary"
	"flag"
	"fmt"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"
)

var nclients uint64
var totPkts,totDrops uint64
var tstart time.Time

func main() {
	tstart = time.Now()
	rand.Seed(time.Now().UnixNano())

	modePtr := flag.Bool("s", false, "set server mode")
	keyPtr := flag.Int("k", 0, "server key")
	clntPtr := flag.Int("n", 1, "number of clients to run")
	pktsPtr := flag.Int("c", 1000, "number of packets to send per client")
	sizePtr := flag.Int("b", 512, "packet size")
	rampPtr := flag.Int("r", 0, "Ramp-up interval in seconds")
	flag.Usage = usage
	flag.Parse()

	// catch CTRL+C
	go trapper()

	// start in server mode, flag.Args()[0] is port to listen on.
	if *modePtr {
		if len(flag.Args()) == 0 {
			udpbouncer("0",*keyPtr)
		} else if len(flag.Args()) == 1 {
			udpbouncer(flag.Args()[0],*keyPtr)
		} else {
			fmt.Println("Error, only the server port should follow the options.", flag.Args())
			return
		}
	}

	// client mode
	if len(flag.Args()) == 0 {
		fmt.Println("Specify server:port")
		os.Exit(1)
	}
	if *keyPtr == 0 {
		fmt.Println("Specify server key")
		os.Exit(1)
	}
	if *sizePtr < 36 {	// IP+UDP+int64 (the int64 key is in the first 8 bytes of data)
		*sizePtr = 36
	}
	fmt.Println("packet size:", *sizePtr)

	// client in ramp-up mode, increase speed until something fails
	// add a new client every *rampPtr seconds
	// when dropexit() detects packet loss it will print the final report and exit
	if *rampPtr > 0 {
		go dropexit()
		*clntPtr = int(^uint(0) >> 1)	// override number of clients to a lot
		*pktsPtr = int(^uint(0) >> 1)	// override packets per client to a lot
		fmt.Println("ramp-up interval:", *rampPtr, "seconds")
		*rampPtr = *rampPtr * 1000	// change to ms

	} else {
		*rampPtr = 10	// normal mode, default delay between clients is 10 ms
	}

	if *pktsPtr < 1 {
		*pktsPtr = int(^uint(0) >> 1)
	}
	fmt.Println("packets per client:", *pktsPtr)
	fmt.Println("number of clients:", *clntPtr)
	if len(flag.Args()) == 1 {
		fmt.Println("server address:", flag.Args()[0])
	} else {
		fmt.Println("Error, only server IP:port follow the options.", flag.Args())
		os.Exit(1)
	}

	// start the statistics printer
	go statsprinter()

	// start the clients
	for i := 0; i < *clntPtr; i++ {
		go udpclient(flag.Args()[0],*pktsPtr, *sizePtr, *keyPtr)
		atomic.AddUint64(&nclients, 1)	// bump the threads counter
		time.Sleep(time.Duration(*rampPtr) * time.Millisecond)	// default 10 ms sleep between go routines, unless in ramp-up mode
	}

	// wait for all clients to exit
	for {
		if atomic.LoadUint64(&nclients) == 0 {
			break
		}
		time.Sleep(1000 * time.Millisecond)
	}
	reportexit()
}

func dropexit () {
	for {
		if atomic.LoadUint64(&totDrops) != 0 {
			fmt.Println()
			reportexit()
		 }
		time.Sleep(1000 * time.Millisecond)
	}
}

func reportexit() {
	fmt.Println("Runtime:", time.Now().Sub(tstart), "Packets sent:", totPkts, "Packets dropped:", totDrops)
	os.Exit(0)
}

func statsprinter() {
	var c1,c2 uint64
	var avgrtt float64

	tstamp := time.Now()
	c1 = atomic.LoadUint64(&totPkts)
	time.Sleep(900 * time.Millisecond)	// estetics, don't race with the ramp-up clients
	for {
		c2 = atomic.LoadUint64(&totPkts)
		avgrtt = time.Now().Sub(tstamp).Seconds() / float64(c2-c1) * float64(atomic.LoadUint64(&nclients)) * 1000
		fmt.Print("pps: ",c2-c1," total drops: ",atomic.LoadUint64(&totDrops))
		fmt.Printf(" avg rtt: %.3f ms", avgrtt)
		fmt.Print(" clients: ", atomic.LoadUint64(&nclients))
		fmt.Println(" runtime:", time.Now().Sub(tstart))
		c1 = c2
		tstamp = time.Now()
		time.Sleep(1000 * time.Millisecond)
	}
}

func trapper() {
	cs := make(chan os.Signal)
	signal.Notify(cs, os.Interrupt, syscall.SIGTERM)
	<- cs
	fmt.Println()
	reportexit()
}

func udpbouncer(port string, key int) {
	serverkey := int64(key)
	if serverkey == 0 {
		serverkey = rand.Int63()
	}

	fmt.Print("Starting server mode, ")
	pc, err := net.ListenPacket("udp","0.0.0.0:"+port)
	if err != nil {
		fmt.Println(err)
		os.Exit(2)
	}
	fmt.Println("listening on",pc.LocalAddr(),"with server key",serverkey)

	buffer := make([]byte, 65536)
	for {
		len,addr,err := pc.ReadFrom(buffer)
		if err != nil {
			fmt.Println(err)
			os.Exit(3)
		}
		if int64(binary.LittleEndian.Uint64(buffer[0:8])) != serverkey {
			atomic.AddUint64(&totDrops, 1)
			continue
		}
		_, err = pc.WriteTo(buffer[0:len], addr)
		if err != nil {
			fmt.Println(err)
			atomic.AddUint64(&totDrops, 1)
		} else {
			atomic.AddUint64(&totPkts, 1)
		}
	}
}

func udpclient(addr string, numpkts int, pktsize int, key int) {
	// decrement the client counter on exit
	defer atomic.AddUint64(&nclients, ^uint64(0))

	// allocate a buffer of random data according to requested packet size
	// stick the server key into the first 8 bytes
	buffer := make([]byte, pktsize-28)	// subtract 20+8, IP+UDP header
	rand.Read(buffer)	// put random data into the buffer
	binary.LittleEndian.PutUint64(buffer[0:8], uint64(key))

	conn, err := net.Dial("udp",addr)
	if err != nil {
		fmt.Println(err)
		os.Exit(2)
	}

	for i := 0; i < numpkts; i++ {
		_, err = conn.Write(buffer)
		if err != nil {
			fmt.Println("write failed:",err)
			time.Sleep(10 * time.Millisecond) // chill a little
			continue
		}
		atomic.AddUint64(&totPkts, 1)

		conn.SetReadDeadline(time.Now().Add(1000*time.Millisecond))
		_, err = conn.Read(buffer)
		if err != nil {
			fmt.Println("read failed:",err)
			atomic.AddUint64(&totDrops, 1)
		}
	}
}

func usage() {
	const text = `
Run as a server (server port is the final option):

server options:
 -s            enable server mode
 -k <number>   server key

phantom -s (random key and port are chosen)
phantom -k 1969 -s (server key 1969, random port)
phantom -k 1969 -s 2929 (server key 1969, port 2929)


Run as a client (server ip:port is the final option):

client options:
 -n <number>   number of threads (default 1)
 -c <number>   packets per thread (default 1k)
 -b <number>   packet size (default 512 bytes)
 -k <number>   server key
 -r <number>   ramp-up mode (see below)

phantom -k 1969 192.0.2.1:2929 (key 1969 server 192.0.2.1:2929)
phantom -k 1969 -n 10 -c 1000000 -b 288 192.0.2.1:2929 (10 threads, 100k packets per thread, 288 byte packets)

Set -c 0 to get a client that runs for a very long time, or til interrupted.

Client in ramp-up mode:

In ramp-up mode the client will add a thread at the interval specified until packet loss is detected.
The option -r overrides the -n and -c options.

phantom -k 1969 -r 5 192.0.2.1:2929 (key 1969, ramp-up 5s, server 192.0.2.1:2929)
`
	fmt.Print(text)
}

