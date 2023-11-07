# 10s_ipv4_tcp_resets_received

## OS: Linux, FreeBSD

TCP reset is an abrupt closure of the session. It causes the resources allocated to the connection
to be immediately released and all other information about the connection to be erased.

The Netdata Agent monitors the average number of received TCP RESETS over the last 10 seconds. This
can indicate that the system is trying to establish a connection to a server port on which no
process is listening. This can also indicate a SYN reset attack.

<details>
  <summary>See more about TCP Resets </summary>

TCP uses a three-way handshake to establish a reliable connection. The connection is full duplex,
and both sides synchronize (SYN) and acknowledge (ACK) each other. The exchange of these four flags
is performed in three steps: SYN, SYN-ACK, and ACK

When an unexpected TCP packet arrives at a host, that host usually responds by sending a reset
packet back on the same connection. A reset packet is one with no payload and with the RST bit set
in the TCP header flags. There are a few circumstances in which a TCP packet might not be expected.
The most common cases are:

 1. A TCP packet received in a non-existed TCP PORT

1. An aborting connection

1. Half opened connections

1. Time wait assassination

1. Listening endpoint Queue is Full

1. A TCP Buffer Overflow

Basically, A TCP Reset usually occurs when a system receives data which doesn't agree with its view
of the connection.

</details>


<details>

  <summary>References and source:</summary>

1. [TCP reset explanation](https://www.pico.net/kb/what-is-a-tcp-reset-rst/)
1. [TCP 3-way handshake on wikipedia](https://en.wikipedia.org/wiki/Handshaking)


</details>

### Troubleshooting section:

<details>

  <summary>General approach</summary>

Try using Wireshark to inspect the network packets.

Wireshark is a free and open-source packet analyzer. It is used for network troubleshooting,
analysis, software and communications protocol development.

[See more about Wireshark here](https://www.wireshark.org/)

Since you might won't be able to probe your traffic with wireshark in your host machine, You can
export it in a dump file and analyze it in a second iteration.

1. Try to export the traffic in your host with `tcpdump`.

  ```
  root@netdata # tcpdump -i any 'tcp[tcpflags] & (tcp-rst) == (tcp-rst)' -s 65535 -w output.pcap
  ```

You must stop the capture after a certain observation period (60s up to 5 minutes). This command
will create a dump file which can be interpreted by Wireshark that contains all the TCP packets with
RST flag set.

2. Copy this file in your workstation and examine it with Wireshark.

</details>


<details>

  <summary>Counter measure on malicious TCP resets</summary>

SYN cookie is a technique used to resist IP address spoofing attacks. In particular, the use of SYN
cookies allows a server to avoid dropping connections when the SYN queue fills up.

   <details>
   
   <summary>Enable SYN cookies in Linux</summary>
   
   1. Check if your system has the SYN cookies service enabled
   
      ```
      root@netdata # cat /proc/sys/net/ipv4/tcp_syncookies 
      ```
      If the value is 1, then the service is enabled, if not proceed to step 2.
   
   
   2. Bump this `net.ipv4.tcp_syncookies=1` value under `/etc/sysctl.conf`
   
   
   3. Apply the configuration
   
       ```
       root@netdata # sysctl -p to apply the configuration.
       ```
   
   </details>

   <details>
   
   <summary>Enable SYN cookies in FreeBSD</summary>
   
   1. Check if your system has the SYN cookies service enabled
   
      ```
      root@netdata # sysctl net.inet.tcp.syncookies_only 
      ```
      If the value is 1, then the service is enabled, if not proceed to step 2.
   
   
   2. Bump this `net.inet.tcp.syncookies_only=1` value under `/etc/sysctl.conf`
   
   
   3. Apply the configuration
   
       ```
       root@netdata~ # /etc/rc.d/sysctl reload
       ```
   
   
   </details>


The use of SYN cookies does not break any protocol specifications, and therefore should be
compatible with all TCP implementations. There are, however, a few caveats that take effect when SYN
cookies are in use.
</details>

