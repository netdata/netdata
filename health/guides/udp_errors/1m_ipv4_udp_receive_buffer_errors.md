# 1m_ipv4_udp_receive_buffer_errors

*In computer networking, the User Datagram Protocol (UDP) is one of the core members of the Internet
protocol suite.*

In both Linux and FreeBSD variants, the kernel allocates buffers to serve the UDP protocol operations. 
Packets after reception from a network interface are forwarded to these buffers to be processed by 
the UDP protocol stack in a system's socket.

The Netdata Agent monitors the average number of UDP receive buffer errors over the last minute.
Receiving this alert means that your system is dropping incoming UDP packets. This may indicate that
the UDP receive buffer queue is full.

This alert is triggered in warning state when the number of UDP receive buffer errors over the last 
minute is more than 10.

<details>
<summary>See more on UDP protocol</summary> 

> UDP uses a simple connectionless communication model with a minimum of protocol mechanisms. UDP
provides checksums for data integrity, and port numbers for addressing different functions at the
source and destination of the datagram. It has no handshaking dialogues, and thus exposes the user's
program to any unreliability of the underlying network. There is no guarantee of delivery, ordering,
or duplicate protection.<sup>[1](https://en.wikipedia.org/wiki/User_Datagram_Protocol) </sup> If no
firewall exists any host can send udp packets to any port, which your server doesn't listen.

</details>

<details>
<summary>References and sources</summary>

1. [UDP definition on wikipedia](https://en.wikipedia.org/wiki/User_Datagram_Protocol)
2. [Man page of UDP protocol](https://man7.org/linux/man-pages/man7/udp.7.html)
3. [Redhat networking tuning guide](https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/5/html/tuning_and_optimizing_red_hat_enterprise_linux_for_oracle_9i_and_10g_databases/sect-oracle_9i_and_10g_tuning_guide-adjusting_network_settings-changing_network_kernel_settings)
4. [UDP on freebsd (blog)](https://awasihba.wordpress.com/2008/10/13/udp-on-freebsd/)


</details>

## OS: Linux

Receive buffer sizes for UDP are controlled by 4 
variables.<sup> [2](https://man7.org/linux/man-pages/man7/udp.7.html) </sup>

- `net.core.rmem_default`, the default setting of the socket receive buffer in bytes.

- `net.core.rmem_max`, the maximum receive socket buffer size in bytes.. Each socket gets
  `rmem_default` receive buffer size by default, and can request up to `rmem_max` with `setsockopt`
  option `SO_RCVBUF`.

- `net.ipv4.udp_mem`, this is a vector of three integers (min, pressure, max) governing the number
  of pages allowed for queueing by all UDP sockets.
    - min:    Below this number of pages, UDP is not bothered about its memory appetite. When the
      amount of memory allocated by UDP exceeds this number, UDP starts to moderate memory usage.
    - pressure: This value was introduced to follow the format of tcp_mem (see tcp(7)).
    - max: Defaults values for these three items are calculated at boot time from the amount of
      available memory.

- `net.ipv4.udp_rmem_min`, the minimal size (in bytes) of receive buffer used by UDP sockets in
  moderation. Each UDP socket is able to use the size for receiving data, even if total pages of UDP
  sockets exceed udp_mem pressure.

In general, issues with buffers that allocated dynamically are correlated with the kernel
memory, you must always be aware of memory pressure events. This can cause buffer errors.

### Troubleshooting section:

 <details>
    <summary>Increase the net.core.rmem_default and net.core.rmem_max values</summary>

1. Try to increase them, RedHat suggests the value of 262144
   bytes <sup> [3](https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/5/html/tuning_and_optimizing_red_hat_enterprise_linux_for_oracle_9i_and_10g_databases/sect-oracle_9i_and_10g_tuning_guide-adjusting_network_settings-changing_network_kernel_settings) </sup>

   ```
   sysctl -w net.core.rmem_default=262144
   sysctl -w net.core.rmem_max=262144
   ```

1. Verify the change and test with the same workload that triggered the alarm originally.

   ```
   root@netdata~ # sysctl net.core.rmem_default net.core.rmem_max
   net.core.rmem_default=262144
   net.core.rmem_max=262144
   ```

1. If this change works for your system, you could make it permanently.

   Bump these `net.core.rmem_default=262144` & `net.core.rmem_max=262144` entries under 
   `/etc/sysctl.conf`.

1. Reload the sysctl settings.

   ```
   root@netdata~ # sysctl -p
   ```

</details>

## OS: FreeBSD

Buffer space for any UDP connection on freebsd is affected by following parameters, as mentioned by
Awasihba in his personal blog. [4](https://awasihba.wordpress.com/2008/10/13/udp-on-freebsd/)

- `net.inet.udp.recvspace`, when you open any UDP socket this parameter decides default receiving
  buffer space for userland data for that socket. You can override that size with help of
  `setsockopt` in your code.

- `kern.ipc.maxsockbuf`, the buffer space for socket, is determined by this parameter. So if
  you try to open socket with large send and receive buffer, and you get error like "no buffer space
  available" then you should consider tweaking `kern.ipc.maxsockbuf`. Sometimes you see frequent UDP
  drops while dealing with large number of tiny UDP packets. Even if your `recvspace` buffer is not
  filled up completely, still you will drop the packets. After digging around for a while we
  figured out that it was happening because we were hitting another hard limit of`sockbuf->sb_mbmax`,
  it specifies maximum number of `mbufs` allocated for each socket. You can increase that limit by 
  increasing `kern.ipc.maxsockbuf`. You need to restart related services to apply this parameter.

- `kern.ipc.nmbcluster`, this parameter governs the total amount memory you have to allocate for all
  the open sockets on your system. This value defines how many numbers of mbuf cluster should be
  allocated. Usually each cluster is of 2k size. For example , if you are planning to open 1000
  sockets with each having 8k sending and 8k size receiving buffer each socket will need 16k of 
  memory and in total you will need 16M (16k x 1000 ) of memory to handle all 1000 connections.

In general, issues with buffers that allocated dynamically are correlated with the kernel
memory, you must always be aware of memory pressure events. This can cause buffer errors.

### Troubleshooting section:

 <details>
    <summary>Increase the kern.ipc.maxsockbuf value</summary>

1. Try to set this value to at least 16MB for 10GE overall 

   ```
   root@netdata~ # sysctl -w kern.ipc.maxsockbuf=16777216
   ```

1. Verify the change and test with the same workload that triggered the alarm originally.

   ```
   root@netdata~ # sysctl kern.ipc.maxsockbuf
   kern.ipc.maxsockbuf=16777216
    ```

1. If this change works for your system, you could make it permanently.

   Bump this `kern.ipc.maxsockbuf=16777216` entry under `/etc/sysctl.conf`.

1. Reload the sysctl settings.

   ```
   root@netdata~ # /etc/rc.d/sysctl reload
   ```

</details>
