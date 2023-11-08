### Understand the alert

When we want to investigate the outbound traffic, the journey of a network packet starts at the application layer. 

Data are written (commonly) to a socket by a user program. The programmer may (raw sockets) or may not (datagram and stream sockets) have the possibility of absolute control over the data which is being sent through the network. The kernel will take the data which is written in a socket queue and allocate the necessary socket buffers. The kernel will try to forward the packets to their destination encapsulating the routing metadata (headers, checksums, fragmentation information) for each packet through a network interface.

The Netdata Agent calculates the ratio of outbound dropped packets for a specific network interface over the last 10 minutes. Receiving this alarm means that packets were dropped on their way to transmission.

This alert is triggered in warning state when the ratio of outbound dropped packets for a specific network interface over the last 10 minutes is more than 2%.

The main reasons of outbound packet drops are:

1. Link congestion
2. Overburdened devices
3. Defective hardware
4. Faulty network configuration
5. Restricted access from firewall rules

### Troubleshoot the alert:

Inspect the packets your network interface sends using Wireshark.

Wireshark is a free and open-source packet analyzer. It is used for network troubleshooting, analysis, software and communications protocol development.

### Useful resources

[Read more about Wireshark here](https://www.wireshark.org/)