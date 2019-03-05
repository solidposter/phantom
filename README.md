This is a basic UDP network tester I wrote to learn some go.
It ping-pongs UDP packets, where packet size, packet count
and number of threads can be specified on the command line.

When running as a server, port to listen on is the final option on the command line.
When running as a client, ip:port to connect to is the final option on the command line.

Run it as a server:

phantom -k 1969 -s 2929 (server listens on port 2929, key 1969)
phantom -s (random key and port are chosen)


Run as a client:

client flags:
 -n <number> number of threads (default 1)
 -c <number> packets per thread (default 1k)
 -b <number> packet size (default 512 bytes)
 -k <number> server key
 -r <number> ramp-up mode (see below)

phantom -k 1969 192.0.2.1:2929 (key 1969 server 192.0.2.1:2929)
phantom -k 1969 -n 10 -c 1000000 -b 288 192.0.2.1:2929 (10 threads, 100k packets per thread, 288 byte packets)

Set -c 0 to get a client that runs for a very long time, or til interrupted.

Client in ramp-up mode:

In ramp-up mode the client will add a thread at the interval specified until packet loss is detected.
The option -r overrides the -n and -c options.

phantom -k 1969 -r 5 192.0.2.1:2929 (key 1969, ramp-up 5s, server 192.0.2.1:2929)

