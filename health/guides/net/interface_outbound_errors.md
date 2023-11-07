# interface_outbound_errors

## OS: FreeBSD

When we want to investigate the outbound traffic, the journey of a network packet starts at the application layer. Data
are written (commonly) to a socket by a user program. The programmer may (raw sockets) or may not (datagram and stream
sockets) have the possibility of absolute control over the data which is being sent through the network. The kernel will
take the data which is written in a socket queue and allocate the necessary socket buffers. The kernel will try to
forward the packets to their destination encapsulating the routing metadata (headers, checksums, fragmentation
information) for each packet through a network interface. The Netdata agent monitors the number of outbound errors for a
specific network interface in the last 10 minutes. Some of the errors that may occur in this process include:

- Errors due to aborted connections

- Carrier sense errors

- FIFO errors

- Heartbeat errors

- Window errors

<details>
   <summary>See more on Carrier Sense Errors</summary>

Carrier Sense Errors occur when an interface attempts to transmit a frame, but no carrier is detected. In that case if
the frame cannot be transmitted, it is discarded.

</details>


<details>
   <summary>See more about heartbeat </summary>

> A heartbeat protocol is generally used to negotiate and monitor the availability of a resource, such as a floating IP
> address, and the procedure involves sending network packets to all the nodes in the culture to verify its
> reachability. Typically when a heartbeat starts on a machine, it will perform an election process with other machines
> on the heartbeat network to determine which machine, if any, owns the resource. On heartbeat networks of more than two
> machines, it is important to take into account partitioning, where two halves of the network could be functioning but
> not able to communicate with each other. In a situation such as this, it is important that the resource is only owned
> by one machine, not one machine in each partition.
>
> As a heartbeat is intended to be used to indicate the health of a machine, it is important that the heartbeat protocol
> and the transport that it runs on are as reliable as possible. Causing a failover because of a false alarm may
> depending on the resource, be highly undesirable. It is also important to react quickly to an actual failure, further
> signifig the reliability of the heartbeat messages. For this reason, it is often desirable to have a heartbeat running
> over more than one transport; for instance, an Ethernet segment using UDP/IP, and a serial
> link. <sup> [1](https://en.wikipedia.org/wiki/Heartbeat_(computing)</sup>

</details>


<details>
<summary>References and sources:</summary>

1. [Heartbeat definition on Wikipedia](https://en.wikipedia.org/wiki/Heartbeat_(computing))

</details>

### Troubleshooting section:

<details>
<summary>General approach</summary>

In any case, a good starting point is to get more information about the nature of your errors.

- `netstat` (network statistics) is a command-line network utility that displays, network connections for Transmission
  Control Protocol, routing tables and network protocol statistics for any interface in your system.

    ```
    root@netdata~ # netstat -sI <your_interface>
    ```

</details>

