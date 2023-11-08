### Understand the alert

Between the IP stack and the Network Interface Controller (NIC) lies the driver queue. This queue is typically implemented as a FIFO ring buffer into the memory space allocated by the driver. The NIC receive frames and place them into memory as skb_buff data structures (SocKet Buffer). We can have queues (ingress queues) and transmitted (egress queues) but these queues do not contain any actual packet data. Each queue has a pointer to the devices associated with it, and to the skb_buff data structures that store the ingress/egress packets. The number of frames this queue can handle is limited. Queues fill up when an interface receives packets faster than kernel can process them.

Netdata monitors the number of FIFO errors (number of times an overflow occurs in the ring buffer) for a specific network interface in the last 10 minutes. This alarm is triggered when the NIC is not able to handle the peak load of incoming/outgoing packets with the current ring buffer size.

Not all NICs support FIFO queue operations.

### More about SKB

The SocKet Buffer (SKB), is the most fundamental data structure in the Linux networking code. Every packet sent or received is handled using this data structure. This is a large struct containing all the control information required for the packet (datagram, cell, etc).

The struct sk_buff has the following fields to point to the specific network layer headers:

- transport_header (previously called h) – This field points to layer 4, the transport layer (and can include tcp header or udp header or
  icmp header, and more)

- network_header (previously called nh) – This field points to layer 3, the network layer (and can include ip header or ipv6 header or arp
  header).

- mac_header (previously called mac) –  This field points to layer 2, the link layer.

- skb_network_header(skb), skb_transport_header(skb) and skb_mac_header(skb) - These return pointer to the header.

### Troubleshoot the alert

- Update the ring buffer size

1. To view the maximum RX ring buffer size:

   ```
   ethtool -g enp1s0
   ```

2. If the values in the Pre-set maximums section are higher than in the Current hardware settings section, increase RX (or TX) ring buffer:

   ```
   enp1s0 rx 4080
   ```

3. Verify the change to make sure that you no longer receive the alarm when running the same workload. To make this permanently, you must consult your distribution guides.
   
