### Understand the alert

In both Linux and FreeBSD variants, the kernel allocates buffers to serve the UDP protocol operations. Packets after reception from a network interface are forwarded to these buffers to be processed by the UDP protocol stack in a system's socket.

The Netdata Agent monitors the average number of UDP receive buffer errors over the last minute. Receiving this alert means that your system is dropping incoming UDP packets. This may indicate that the UDP receive buffer queue is full. This alert is triggered in warning state when the number of UDP receive buffer errors over the last minute is more than 10.

In general, issues with buffers that allocated dynamically are correlated with the kernel memory, you must always be aware of memory pressure events. This can cause buffer errors.

### Troubleshoot the alert (Linux)

- Increase the net.core.rmem_default and net.core.rmem_max values

1. Try to increase them, RedHat suggests the value of 262144 bytes
   ```
   sysctl -w net.core.rmem_default=262144
   sysctl -w net.core.rmem_max=262144
   ```

2. Verify the change and test with the same workload that triggered the alarm originally.
   ```
   sysctl net.core.rmem_default net.core.rmem_max
   net.core.rmem_default=262144
   net.core.rmem_max=262144
   ```

3. If this change works for your system, you could make it permanently.

   Bump these `net.core.rmem_default=262144` & `net.core.rmem_max=262144` entries under `/etc/sysctl.conf`.

4. Reload the sysctl settings.

   ```
   sysctl -p
   ```

### Troubleshoot the alert (FreeBSD)

- Increase the kern.ipc.maxsockbuf value

1. Try to set this value to at least 16MB for 10GE overall 
   ```
   sysctl -w kern.ipc.maxsockbuf=16777216
   ```

2. Verify the change and test with the same workload that triggered the alarm originally.
   ```
   sysctl kern.ipc.maxsockbuf
   kern.ipc.maxsockbuf=16777216
    ```

3. If this change works for your system, you could make it permanently.

   Bump this `kern.ipc.maxsockbuf=16777216` entry under `/etc/sysctl.conf`.

4. Reload the sysctl settings.
   ```
   /etc/rc.d/sysctl reload
   ```

### Useful resources

1. [UDP definition on wikipedia](https://en.wikipedia.org/wiki/User_Datagram_Protocol)
2. [Man page of UDP protocol](https://man7.org/linux/man-pages/man7/udp.7.html)
3. [Redhat networking tuning guide](https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/5/html/tuning_and_optimizing_red_hat_enterprise_linux_for_oracle_9i_and_10g_databases/sect-oracle_9i_and_10g_tuning_guide-adjusting_network_settings-changing_network_kernel_settings)
4. [UDP on freebsd (blog)](https://awasihba.wordpress.com/2008/10/13/udp-on-freebsd/)
