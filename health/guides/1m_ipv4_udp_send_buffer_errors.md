### Understand the alert

The linux kernel allocates buffers to serve the UDP protocol operations. Data is written into sockets that utilize UDP to send data to an another system/subsystem.

The Netdata Agent monitors the average number of UDP send buffer errors over the last minute. This alert indicates that the UDP send buffer is full or no kernel memory available. Receiving this alert
means that your system is dropping outgoing UDP packets. This alert is triggered in warning state when the number of UDP send buffer errors over the last minute is more than 10.

In general, issues with buffers that allocated dynamically are correlated with the kernel memory, you must always be aware of memory pressure events. This can cause buffer errors.

### Troubleshooting section:

- Increase the net.core.wmem_default and net.core.wmem_max values

1. Try to increase them, RedHat suggests the value of 262144 bytes 

   ```
   sysctl -w net.core.wmem_default=262144
   sysctl -w net.core.wmem_max=262144
   ```

2. Verify the change and test with the same workload that triggered the alarm originally.

   ```
   sysctl net.core.wmem_default net.core.wmem_max 
   net.core.wmem_default=262144
   net.core.wmem_max=262144
   ```

3. If this change works for your system, you could make it permanently.

   Bump these `net.core.wmem_default=262144` & `net.core.wmem_max=262144` entries under  `/etc/sysctl.conf`.

4. Reload the sysctl settings.

   ```
   sysctl -p
   ```

### Useful resources

1. [UDP definition on wikipedia](https://en.wikipedia.org/wiki/User_Datagram_Protocol)
2. [Man page of UDP protocol](https://man7.org/linux/man-pages/man7/udp.7.html)
3. [Redhat networking tuning guide](https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/5/html/tuning_and_optimizing_red_hat_enterprise_linux_for_oracle_9i_and_10g_databases/sect-oracle_9i_and_10g_tuning_guide-adjusting_network_settings-changing_network_kernel_settings)
