# 1m_received_traffic_overflow

## OS: Linux

Network interfaces are categorized primarily on the bandwidth they can operate (1 Gbps, 10 Gbps, etc). High network
utilization occurs when the volume of data on a network link approaches the capacity of the link. Netdata agent
calculates the average outbound utilization for a specific network interface over the last minute. High outbound
utilization increases latency and packet loss because packet bursts are buffered

This alarm may indicate either network congestion or malicious activity.

### Troubleshooting section

<details>
    <summary>Prioritize important traffic</summary>

Quality of service (QoS) is the use of routing prioritization to control traffic and ensure the performance of
critical applications. QoS works best when low-priority traffic exists that can be dropped when congestion occurs. The
higher-priority traffic must fit within the bandwidth limitations of the link or path. The following are two open source
solutions to apply QoS policies to your network interfaces.

- `FireQOS`:

  FireQOS is a traffic shaping helper. It has a very simple shell scripting language to express traffic shaping.

  [See more on FireQOS](https://firehol.org/tutorial/fireqos-new-user/)

- `tcconfig`:

  Tcconfig is a command wrapper that makes it easy to set up traffic control of network
  bandwidth/latency/packet-loss/packet-corruption/etc.

  [See more on tcconfig](https://tcconfig.readthedocs.io/en/latest/index.html)

</details>


<details>
   <summary>Add bandwidth</summary>

- For **Cloud infrastructures**, adding bandwidth might be easy. It depends on your cloud infrastracture and your cloud
  provider. Some of them either offer you the service to upgrade machines to a higher bandwidth rate or upgrade you
  machine to a more powerful one with higher bandwidth rate.

- For **Bare-metal** machines, you will need either a hardware upgrade or the addition of a network card using link
  aggregation to combine multiple network connections in parallel (e.g LACP).

</details>
