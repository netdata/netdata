### Understand the alert

The linux kernel contains queues where packets are stored after reception from a network interface controller before being processed by the next protocol stack. There is one netdev backlog queue per CPU core. netdev_max_backlog defines the maximum number of packets that can enter the queue. Queues fill up when an interface receives packets faster than kernel can process them. The default netdev_max_backlog value should be 1000. However this may not be enough in cases such as:

- Multiple interfaces operating at 1Gbps, or even a single interface at 10Gbps.

- Lower powered systems process very large amounts of network traffic.

Netdata monitors the average number of dropped packets in the last minute due to exceeding the netdev backlog queue.

### Troubleshoot the alert

- Increase the netdev_max_backlog value

1. Check your current value:

   ```
   root@netdata~ # sysctl net.core.netdev_max_backlog
   net.core.netdev_max_backlog = 1000
   ```

2. Try to increase it by a factor of 2.

   ```
   root@netdata~ # sysctl -w net.core.netdev_max_backlog=2000
   ```

3. Verify the change and test with the same workload that triggered the alarm originally.

   ```
   root@netdata~ # sysctl net.core.netdev_max_backlog
   net.core.netdev_max_backlog = 2000
    ```

4. If this change works for your system, you could make it permanently.

   Bump this `net.core.netdev_max_backlog=2000` entry under `/etc/sysctl.conf`.

5. Reload the sysctl settings.

   ```
   root@netdata~ # sysctl -p
   ```

