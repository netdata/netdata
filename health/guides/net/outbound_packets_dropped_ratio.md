# outbound_packets_dropped_ratio

## OS: Linux

When we want to investigate the outbound traffic, the journey of a network packet starts at the
application layer. Data are written (commonly) to a socket by a user program. The programmer may (
raw sockets) or may not (datagram and stream sockets) have the possibility of absolute control over
the data which is being sent through the network. The kernel will take the data which is written in
a socket queue and allocate the necessary socket buffers. The kernel will try to forward the packets
to their destination encapsulating the routing metadata (headers, checksums, fragmentation
information) for each packet through a network interface.

The Netdata Agent calculates the ratio of outbound dropped packets for a specific network interface
over the last 10 minutes. Receiving this alarm means that packets were dropped on their way to
transmission.

This alert is triggered in warning state when the ratio of outbound dropped packets for a specific
network interface over the last 10 minutes is more than 2%.

The main reasons of outbound packet drops are:

1. Link congestion
1. Overburdened devices
1. Defective hardware
1. Faulty network configuration
1. Restricted access from firewall rules

### Troubleshooting section:

The best way to resolve these kind of problems is to be extremely knowledgeable about your network
topologies.

<details>
<summary>Inspect the packets your network interface sends</summary>

Wireshark is a free and open-source packet analyzer. It is used for network troubleshooting,
analysis, software and communications protocol development.

[See more about Wireshark here](https://www.wireshark.org/)

</details>
