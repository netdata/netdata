### Understand the alert

The `10min_netisr_backlog_exceeded` alert occurs when the `netisr_maxqlen` queue within FreeBSD's network kernel dispatch service reaches its maximum capacity. This queue stores packets received by interfaces and waiting to be processed by the destined subsystems or userland applications. When the queue is full, the system drops new packets. This alert indicates that the average number of dropped packets in the last minute has exceeded the netisr queue length.

### Troubleshoot the alert

1. **Increase the netisr_maxqlen value**

  a. Check the current value:

  ```
  root@netdata~ # sysctl net.route.netisr_maxqlen
  net.route.netisr_maxqlen: 256
  ```

  b. Increase the value by a factor of 4:

  ```
  root@netdata~ # sysctl -w net.route.netisr_maxqlen=1024
  ```

  c. Verify the change and test with the same workload that triggered the alarm originally:

  ```
  root@netdata~ # sysctl net.route.netisr_maxqlen
  net.route.netisr_maxqlen: 1024
  ```

  d. If the change works for your system, make it permanent by adding this entry, `net.route.netisr_maxqlen=1024`, to `/etc/sysctl.conf`.

  e. Reload the sysctl settings:

  ```
  root@netdata~ # /etc/rc.d/sysctl reload
  ```

2. **Monitor the system**
   
   After increasing the `netisr_maxqlen` value, continue to monitor your system's dropped packet statistics using tools like `netstat` to determine if the queue backlog situation has improved. If you are still experiencing high packet drop rates, you may need to further increase the `netisr_maxqlen` value, or explore other optimizations for your networking stack.

3. **Check hardware and system resources**

   In some cases, overloaded or underpowered hardware may cause issues with packet processing. Ensure that your hardware (network cards, switches, routers, etc.) is performing optimally, and that your system has enough CPU and RAM resources to handle the traffic load.

4. **Network traffic analysis**

   Analyze your network traffic using tools like `tcpdump`, `iftop`, or `iptraf` to identify specific traffic patterns or types causing the backlog issue. This analysis can help you optimize your network infrastructure or take actions to reduce unnecessary traffic.

5. **Update FreeBSD version**

   Ensure that your FreeBSD system is up to date, as newer kernel versions may include performance improvements and optimizations for packet processing. Updating to a newer version might help resolve netisr backlog issues.

### Useful resources

1. [FreeBSD Performance Tuning](https://calomel.org/freebsd_network_tuning.html)
2. [FreeBSD Handbook: Tuning Kernel Limits](https://www.freebsd.org/doc/en_US.ISO8859-1/books/handbook/configtuning-kernel-limits.html)
