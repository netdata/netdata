# tcp_orphans

## OS: Linux

This alert presents the percentage of used orphan IPV4 TCP sockets. If it is raised, it indicates
that your system is experiencing high IPv4 TCP sockets utilization.
When the system exceeds the limit, orphaned connections (connections not attached to any user filehandle) are
reset immediately.

This alert is triggered in warning state when the percentage of used orphan IPv4 TCP sockets is
above 25% and in critical state when that value exceeds 50%.


<Details>
<summary>What is a network socket</summary>

> A network socket is a software structure within a network node of a computer network that
> serves as an endpoint for sending and receiving data across the network. The structure and
> properties of a socket are defined by an application programming interface (API) for the
> networking architecture. Sockets are created only during the lifetime of a process of an
> application running in the node.

> Because of the standardization of the TCP/IP protocols in the development of the Internet, the
> term network socket is most commonly used in the context of the Internet protocol suite, and
> is therefore often also referred to as Internet socket.

> In this context, a socket is externally identified to other hosts by its socket address,
> which is the triad of transport protocol, IP address, and port number.
> <sup>[1](https://en.wikipedia.org/wiki/Network_socket) </sup>

</Details>

<br>

<details>
<summary>What is a "filehandle" or "socket descriptor"</summary>

> The application programming interface (API) for the network protocol stack creates a handle
> for each socket created by an application, commonly referred to as a socket descriptor. In
> Unix-like operating systems, this descriptor is a type of file descriptor. It is stored by
> the application process for use with every read and write operation on the communication channel.
> <sup>[1](https://en.wikipedia.org/wiki/Network_socket) </sup>

</details>

<br>

> An orphan socket is a socket that isn't associated with a file descriptor, usually after the
> close() call and there is no longer a file descriptor that reference it, but the socket still
> exists in memory, until TCP is done with it.<sup> [2](
> http://www.linux-admins.net/2013/01/troubleshooting-out-of-socket-memory.html) </sup>

<br>

<details>
<summary> References and Sources </summary>

1. [Network_sockets](https://en.wikipedia.org/wiki/Network_socket)
2. [Linux-admins.com](http://www.linux-admins.net/2013/01/troubleshooting-out-of-socket-memory.html) 
</sup>

</details>

### Troubleshooting Section

<details>
<summary>Increase the orphan socket limit</summary>

To counteract this behavior, you can increase the limit in the
file: `/proc/sys/net/ipv4/tcp_max_orphans`. Simply run:

```
root@netdata~ # echo {DESIRED_AMOUNT} > /proc/sys/net/ipv4/tcp_max_orphans
```

The kernel may penalize orphans by 2x or even 4x (hence the small warning and critical thresholds).
You may need to watch for orphaned sockets during peak hours and consider multiplying that number by
3 or 4. That should give you a good starting point.

> Note: Netdata strongly suggests knowing exactly what you are configuring before making system
> changes.
</details>
