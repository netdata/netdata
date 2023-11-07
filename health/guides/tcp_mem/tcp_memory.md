# tcp_memory

## OS: Linux

The Netdata Agent calculates the percentage of used TCP memory. Receiving this alert indicates that
the TCP memory utilization uses more memory than the limit.

By default, the Linux network stack is not configured for high speed large file transfer across WAN
links. This is done to save memory resources. System performs out of memory checks, if the memory
used by the TCP protocol is higher than the third value(max) of the `net.ipv4.tcp_mem`, it throws
OOM error. This will make some applications to become unresponsive.



<details>
<summary>See more about TCP buffers </summary>

There are 3 main parameters to configure regarding the TCP buffers.

> - `tcp_mem` is a vector of 3 integers: `"low, pressure, high"`. These bounds, measured in units of
  the system page size, are used by TCP to track its memory usage. The defaults are calculated at
  boot time from the amount of available memory.  (TCP can only use low memory for this, which is
  limited to around 900 megabytes on 32-bit systems. 64-bit systems do not suffer this limitation.)

>    - low TCP doesn't regulate its memory allocation when the number of pages it has allocated
      globally is below this number.
>    - pressure When the amount of memory allocated by TCP exceeds this number of pages, TCP
      moderates its memory consumption. This memory pressure state is exited once the number of
      pages allocated falls below the low mark.
>    - high The maximum number of pages, globally, that TCP will allocate. This value overrides any
      other limits imposed by the kernel. <sup>[1](https://man7.org/linux/man-pages/man7/tcp.7.html) </sup>

The min/pressure/max TCP buffer space are automatically set in `/proc/sys/net/ipv4/tcp_mem` during
the boot time based on available RAM size.

> - `net.ipv4.tcp_rmem` contains three values that represent `"minimum default maximum_size` of the
  TCP socket receive buffer.

>    - The minimum represents the smallest receive buffer size guaranteed, even under memory
      pressure. The minimum value defaults to 1 page or 4096 bytes.

>    - The default value represents the initial size of a TCP sockets receive buffer. This value
      supersedes net.core.rmem_default used by other protocols. The default value for this setting
      is 87380 bytes. It also sets the tcp_adv_win_scale and initializes the TCP window size to
      65535 bytes.

>    - The maximum represents the largest receive buffer size automatically selected for TCP sockets.
      This value does not override net.core.rmem_max. The default value for this setting is
      somewhere between 87380 bytes and 6M bytes based on the amount of memory in the system.
>
> 
>  The recommendation is to use the maximum value of 16M bytes or higher (kernel level dependent)
  especially for 10 Gigabit adapters.
>
>
> - `net.ipv4.tcp_wmem` parameter also consists of 3 values `"minimum default maximum"`.
>    - The minimum represents the smallest receive buffer size a newly created socket is entitled to
      as part of its creation. The minimum value defaults to 1 page or 4096 bytes.
>    - The default value represents the initial size of a TCP sockets receive buffer. This value
      supersedes net.core.rmem_default used by other protocols. It is typically set lower than
      net.core.wmem_default. The default value for this setting is 16K bytes.
>    - The maximum represents the largest receive buffer size for auto-tuned send buffers for TCP
      sockets. This value does not override net.core.rmem_max. The default value for this setting is
      somewhere between 64K bytes and 4M bytes based on the amount of memory available in the
      system.
>
> 
>The recommendation is to use the maximum value of 16M bytes or higher (kernel level dependent)
especially for 10 Gigabit adapters. <sup>[2](https://www.ibm.com/docs/en/linux-on-systems?topic=tuning-tcpip-ipv4-setting) </sup>

</details>

<details>
<summary>References and sources</summary>

1. [man pages of tcp](https://man7.org/linux/man-pages/man7/tcp.7.html)
1. [Adjustments for IPv4 settings from IBM](https://www.ibm.com/docs/en/linux-on-systems?topic=tuning-tcpip-ipv4-settings)
</details>

### Troubleshooting section:

<details>
<summary>Increase the TCP memory </summary>

Increasing the TCP memory available in the Linux network stack may resolve this issue.

1. Try to increase the `tcp_mem` bounds

    ```
    root@netdata # sysctl -w net.ipv4.tcp_mem="819200 1091174 1638400"
    ```

1. Verify the change and test it with the same workload that triggered the alarm originally.
   If the problem still exists, you can always consider increase it more. 

    ```
    root@netdata~ # sysctl net.ipv4.tcp_mem
    net.ipv4.tcp_mem=819200 1091174 1638400
    ```
 
1. If this change works for your system, you could make it permanently. Bump these entries
   under `/etc/sysctl.conf`


1. Reload the sysctl settings.

    ```
    root@netdata~ # sysctl -p
    ```


