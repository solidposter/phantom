This is a basic UDP network tester I wrote to learn some go.
It ping-pongs UDP packets, where packet size, packet count
and number of threads can be specified on the command line.


Run it as a server:

phantom -k 1969 -s 2929 (server listens on port 2929, key 1969)

If key or port isn't specified random values are chosen:
phantom -s 

Note that the server port is the last option when specifying server mode


Run as a client:

phantom -k 1969 192.0.2.1:2929 (key 1969 server 192.0.2.1:2929)

client flags:
 -n <number> number of threads (default 1)
 -c <number> packets per thread (default 1k)
 -b <number> packet size (default 512 bytes)
 -k <number> server key

Another client example:
phantom -k 1969 -n 10 -c 1000000 -b 288 192.0.2.1:2929

10 threads, 1M packets per thread, 288 bytes packet size, server 192.0.2.1:2929

Set -c 0 to get a client that runs for a very long time, or til interrupted.

