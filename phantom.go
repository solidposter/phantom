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
var tstart,tend time.Time

func main() {
	tstart = time.Now()
	rand.Seed(time.Now().UnixNano())

	modePtr := flag.Bool("s", false, "set server mode")
	keyPtr := flag.Int("k", 0, "server key")
	clntPtr := flag.Int("n", 1, "number of clients to run")
	pktsPtr := flag.Int("c", 1000, "number of packets to send per client")
	sizePtr := flag.Int("b", 512, "packet size")
	rampPtr := flag.Int("r", 0, "Ramp-up interval in seconds")
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
			fmt.Println("Invalid options. Server port should be the final option.", flag.Args()[1:])
			return
		}
	}

	// client mode
	if len(flag.Args()) == 0 {
		fmt.Println("Specify server:port")
		return
	}
	if *keyPtr == 0 {
		fmt.Println("Specify server key")
		return
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
		*clntPtr = 10000		// override number of clients to 10k
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
		fmt.Println("Invalid options. Server IP:port should be the final option.", flag.Args()[1:])
		return
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
	finalreport()
}

func dropexit () {
	for {
		if atomic.LoadUint64(&totDrops) != 0 {
			fmt.Println()
			finalreport()
			os.Exit(0)
		 } else {
			time.Sleep(1000 * time.Millisecond) // insert sleep to handle s
		}
	}
}

func finalreport() {
	tend = time.Now()
	fmt.Println("Runtime:", tend.Sub(tstart), "Packets received:", totPkts, "Packets dropped:", totDrops)
}

func statsprinter() {
	var c1,c2 uint64

	c1 = atomic.LoadUint64(&totPkts)
	for {
		time.Sleep(1000 * time.Millisecond)
		c2 = atomic.LoadUint64(&totPkts)
		fmt.Print("pps: ",c2-c1," total drops: ",atomic.LoadUint64(&totDrops))
		fmt.Printf(" avg rtt: %.3f",1/float64(c2-c1)*1000*float64(atomic.LoadUint64(&nclients)))
		fmt.Println("ms")
		c1 = c2
	}
}

func trapper() {
	cs := make(chan os.Signal)
	signal.Notify(cs, os.Interrupt, syscall.SIGTERM)
	<- cs
	fmt.Println()
	finalreport()
	os.Exit(0)
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
		os.Exit(1)
	}
	fmt.Println("listening on",pc.LocalAddr(),"with server key",serverkey)

	buffer := make([]byte, 65536)
	for {
		len,addr,err := pc.ReadFrom(buffer)
		if err != nil {
			fmt.Println(err)
			os.Exit(2)
		}
		atomic.AddUint64(&totPkts, 1)
		if int64(binary.LittleEndian.Uint64(buffer[0:8])) == serverkey {
			pc.WriteTo(buffer[0:len], addr)
		} else {
			atomic.AddUint64(&totDrops, 1)
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

