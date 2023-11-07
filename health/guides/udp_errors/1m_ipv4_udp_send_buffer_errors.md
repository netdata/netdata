# 1m_ipv4_udp_send_buffer_errors

## OS: Linux

*In computer networking, the User Datagram Protocol (UDP) is one of the core members of the Internet
protocol suite.*

The linux kernel allocates buffers to serve the UDP protocol operations. Data is written into
sockets that utilize UDP to send data to an another system/subsystem.

The Netdata Agent monitors the average number of UDP send buffer errors over the last minute. This
alert indicates that the UDP send buffer is full or no kernel memory available. Receiving this alert
means that your system is dropping outgoing UDP packets

This alert is triggered in warning state when the number of UDP send buffer errors over the last
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


Send buffer sizes for UDP are controlled by 4
variables.<sup> [2](https://man7.org/linux/man-pages/man7/udp.7.html) </sup>

- `net.core.wmem_default`, the default setting of the socket send buffer in bytes.

- `net.core.wmem_max`, default and max socket send buffer size in bytes. Each socket gets
  `wmem_default` send buffer size by default, and can request up to `wmem_max` with `setsockopt` 
  option `SO_SNDBUF`.

- `net.ipv4.udp_mem`, this is a vector of three integers (min, pressure, max) governing the number
  of pages allowed for queueing by all UDP sockets.
    - min:    Below this number of pages, UDP is not bothered about its memory appetite. When the
      amount of memory allocated by UDP exceeds this number, UDP starts to moderate memory usage.
    - pressure: This value was introduced to follow the format of tcp_mem (see tcp(7)).
    - max: Defaults values for these three items are calculated at boot time from the amount of
      available memory.

- `net.ipv4.udp_wmem`, the minimal size (in bytes) of send buffer used by UDP sockets in moderation.
   Each UDP socket is able to use the size for sending data, even if total pages of UDP sockets 
   exceed `udp_mem` pressure.

In general, issues with buffers that allocated dynamically are correlated with the kernel memory,
you must always be aware of memory pressure events. This can cause buffer errors.

<details>
<summary>References and sources</summary>

1. [UDP definition on wikipedia](https://en.wikipedia.org/wiki/User_Datagram_Protocol)
2. [Man page of UDP protocol](https://man7.org/linux/man-pages/man7/udp.7.html)
3. [Redhat networking tuning guide](https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/5/html/tuning_and_optimizing_red_hat_enterprise_linux_for_oracle_9i_and_10g_databases/sect-oracle_9i_and_10g_tuning_guide-adjusting_network_settings-changing_network_kernel_settings)

</details>

### Troubleshooting section:

 <details>
    <summary>Increase the net.core.wmem_default and net.core.wmem_max values</summary>

1. Try to increase them, RedHat suggests the value of 262144
   bytes <sup> [3](https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/5/html/tuning_and_optimizing_red_hat_enterprise_linux_for_oracle_9i_and_10g_databases/sect-oracle_9i_and_10g_tuning_guide-adjusting_network_settings-changing_network_kernel_settings) </sup>

   ```
   sysctl -w net.core.wmem_default=262144
   sysctl -w net.core.wmem_max=262144
   ```

1. Verify the change and test with the same workload that triggered the alarm originally.

   ```
   root@netdata~ # sysctl net.core.wmem_default net.core.wmem_max 
   net.core.wmem_default=262144
   net.core.wmem_max=262144
   ```

1. If this change works for your system, you could make it permanently.

   Bump these `net.core.wmem_default=262144` & `net.core.wmem_max=262144` entries under 
   `/etc/sysctl.conf`.

1. Reload the sysctl settings.

   ```
   root@netdata~ # sysctl -p
   ```

</details>
