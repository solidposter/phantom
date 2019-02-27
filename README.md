This is a basic UDP network tester.
It ping-pongs UDP packets, where packet size, packet count
and number of threads can be specified on the command line.

Run it as a server:
phantom -s (defaults to listen on UDP port 2222)
phantom -s 4567 (server listens on port 4567)

Run as a client:

phantom 192.0.2.1:2222 (default test to server 192.0.2.1 port 2222)
client flags:
 -n <number> number of threads (default 1)
 -c <number> packets per thread (default 10k)
 -b <number> packet size (default 512 bytes)

Another client example:
phantom -n 10 -c 1000000 -b 288 192.0.2.1:2222

10 threads, 1M packets per thread, 288 bytes packet size, server 192.0.2.1:2222


