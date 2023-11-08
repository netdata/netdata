### Understand the alert

Packet drops indicate that your system received some packets but could not process them. A sizeable amount of packet drops can consume significant amount of resources in your system. Some reasons that packets drops occurred in your system could be:

- Your system receives packets with bad VLAN tags.
- The packets you are receiving are using a protocol that is unknown to your system.
- You receive IPv6 packets, but your system is not configured for IPv6.

All these packets consume resources until being dropped (and for a short period after). For example, your NIC stores them in a ring-buffer until they are forwarded to the destined subsystem or userland application for further process.

Netdata calculates the ratio of inbound dropped packets for your wired network interface over the last 10 minutes.

### Identify VLANs in your interface

There are cases in which traffic is routed to your host due to the existence of multiple VLAN in your network.

1. Identify VLAN tagged packet in your interface.

```
tcpdump -i <your_interface> -nn -e  vlan
```

2. Monitor the output of the `tcpdump`, identify VLANs which may exist. If no output is displayed, your interface probably uses traditional ethernet frames.

3. Depending on your network topology, you may consider removing unnecessary VLANs from the switch trunk port toward your host.

### Update the ring buffer size on your interface

1. To view the maximum RX ring buffer size:

    ```
    ethtool -g enp1s0
    ```

2. If the values in the Pre-set maximums section are higher than in the current hardware settings section, increase RX
   ring buffer:

   ```
   enp1s0 rx 4080
   ```

3. Verify the change to make sure that you no longer receive the alarm when running the same workload. To make this
   permanently, you must consult your distribution guides.


### Inspect the packets your network interface receives

Wireshark is a free and open-source packet analyzer. It is used for network troubleshooting, analysis, software and communications protocol development.

### Useful resources

[Read more about Wireshark here](https://www.wireshark.org/)