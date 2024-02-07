### Understand the alert

TCP reset is an abrupt closure of the session. It causes the resources allocated to the connection to be immediately released and all other information about the connection is erased.

The Netdata Agent monitors the average number of sent TCP RESETS over the last 10 seconds. This can indicate a port scan or that a service running on the system has crashed. Additionally, it's a result of a high number of sent TCP RESETS. Furthermore, it can also indicate a SYN reset attack.

### More about TCP Resets

TCP uses a three-way handshake to establish a reliable connection. The connection is full duplex, and both sides synchronize (SYN) and acknowledge (ACK) each other. The exchange of these four flags
is performed in three steps: SYN, SYN-ACK, and ACK. 

When an unexpected TCP packet arrives at a host, that host usually responds by sending a reset packet back on the same connection. A reset packet is one with no payload and with the RST bit set in the TCP header flags. There are a few circumstances in which a TCP packet might not be expected. The most common cases are:

1. A TCP packet received on a port that is not open.
2. An aborting connection
3. Half opened connections
4. Time wait assassination
5. Listening endpoint Queue is Full
6. A TCP Buffer Overflow

Basically, A TCP Reset usually occurs when a system receives data which doesn't agree with its view of the connection.

When your system cannot establish a connection it will retry by default `net.ipv4.tcp_syn_retries` times.

### Troubleshoot the alert

- Use tcpdump to capture the traffic and use Wireshark to inspect the network packets. You must stop the capture after a certain observation period (60s up to 5 minutes). This command will create a dump file which can be interpreted by Wireshark that contains all the TCP packets with RST flag set.
  ```
  tcpdump -i any 'tcp[tcpflags] & (tcp-rst) == (tcp-rst)' -s 65535 -w output.pcap
  ```

- Identify which application sends TCP resets

1. Check the instances of `RST` events of the TCP protocol. Wireshark also displays the ports on which the two systems tried to establish the TCP connection, (XXXXXX -> XXXXXX).
2. To check which application is using this port, run the following code:
    ```
    lsof -i:XXXXXX -P -n
    ```
### Useful resources

1. [TCP reset explanation](https://www.pico.net/kb/what-is-a-tcp-reset-rst/)
2. [TCP 3-way handshake on wikipedia](https://en.wikipedia.org/wiki/Handshaking)
3. [Read more about Wireshark here](https://www.wireshark.org/)