# 10s_received_packets_storm

## OS: Linux, FreeBSD

A packet storm is defined as an unusually high amount of traffic on a specific interface. In a sliding window of one minute
Netdata agent monitors for significant increases of packets (ratio of an average number of received packets) in the last
10 seconds. If this system is expected to have spikes, you can cautiously ignore this alarm, but should continue to monitor as this 
alarm may also indicate a broadcast/multicast storm or DoS attack.

<details>
<summary>See more on broadcast storms.</summary>

A broadcast storm is the accumulation of broadcast and multicast traffic on a computer network. Extreme amounts of
broadcast traffic constitute a "broadcast storm". It can consume sufficient network resources so as to render the
network unable to transport normal traffic. Most commonly the cause is a switching loop in the Ethernet wiring topology.
As broadcasts and multicasts are forwarded by switches out of every port, the switch or switches will repeatedly
rebroadcast broadcast messages and flood the network. Since the Layer 2 header does not support a time to live (TTL)
value, if a frame is sent into a looped topology, it can loop forever.

</details>


<details>
<summary>See more on DoS attacks.</summary>

A Denial-of-Service (DoS) attack is an attack meant to shut down a machine or network, making it inaccessible to its
intended users. DoS attacks accomplish this by flooding the target with traffic, or sending it information that triggers
a crash. We can categorize the attacks into two types.

- Infrastructure Layer Attacks:

  Attacks at Layer 3 and 4 of the OSI model are typically categorized as Infrastructure layer attacks. The most common
  type of DDoS attack include vectors, like synchronized (SYN) floods, and other reflection attacks, like User Datagram
  Packet (UDP) floods. These attacks are usually large in volume and aim to overload the capacity of the network or the
  application servers. Fortunately, these are also the type of attacks that have clear signatures and are easier to
  detect.

- Application Layer Attacks:

  Attacks at Layer 6 and 7 of the OSI model, are often categorized as Application layer attacks. While these attacks are
  less common, they also tend to be more complex. These attacks are typically small in volume compared to the
  Infrastructure layer attacks, but tend to focus on particular expensive parts of the application, thereby making it
  unavailable for real users. Common examples of this type of attack include a flood of HTTP requests to a login page, or an expensive search API, or
  even Wordpress XML-RPC floods.
  
</details>


### Troubleshooting section:

</details>

<details>
<summary>Counter measures on DoS and DDoS attacks</summary>

- Use a service like Cloudflare. Cloudflare DDoS protection secures websites, applications, and entire networks while
  ensuring the performance of legitimate traffic is not compromised.

- Limit broadcasting. Often attacks will send requests to every device on the network, amplifying the attack. Limiting
  or turning off broadcast forwarding where possible can disrupt attacks. Users can also disable echo and chargen
  services where possible.

</details>


</details>

<details>

<summary>Counter measures on broadcast storms</summary>

- Switching loops are largely addressed through link aggregation, shortest path bridging, or spanning tree protocol. In
  Metro Ethernet rings, it is prevented using the Ethernet Ring Protection Switching (ERPS) or Ethernet Automatic
  Protection System (EAPS) protocols.

- You can filter broadcasts by Layer 3 equipment, most typically routers or even switches that employ advanced filtering.

- Routers and firewalls can be configured to detect and prevent maliciously inducted broadcast storms

</details>