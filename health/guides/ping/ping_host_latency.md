# fping_host_latency

**Other | Network**

`fping` is a command line tool to send ICMP (Internet Control Message Protocol) echo requests to
network hosts, similar to ping, but performing much better when pinging multiple hosts. The Netdata
Agent utilizes `fping` to monitor latency, packet loss, uptime and reachability of any number of
network endpoints.

For the `fping_host_latency` alert, the Netdata Agent monitors the average latency to the network
host over the last 10 seconds. Receiving this alert indicates high latency to the network host. It is
likely you are experiencing networking issues or the host is overloaded.

### Troubleshooting section

<details>
    <summary>Customize the ICMP requests for each endpoint</summary>

Different endpoints could be in different networks. For example, a server in your intra network
would require less time to be accessed than your cloud infrastructures in terms of latency. You
should always consider not to use a global approach for checking every endpoint of yours. You can
find more information about how to configure every endpoint separately in
the [fping.plugin alarm guide](https://learn.netdata.cloud/docs/agent/collectors/fping.plugin/#additional-tips).

</details>

<details>
    <summary>Prioritize traffic on your endpoints</summary>

Quality of service (QoS) is the use of mechanisms or technologies to control traffic and ensure the
performance of critical applications. QoS works best when low-priority traffic exists that can be
dropped when congestion occurs. The higher-priority traffic must fit within the bandwidth
limitations of the link or path. The following are two open source solutions to apply QoS policies
to your network interfaces.

- `FireQOS`:

  FireQOS is a traffic shaping helper. It has a very simple shell scripting language to express
  traffic shaping.

  [See more on FireQOS](https://firehol.org/tutorial/fireqos-new-user/)

- `tcconfig`:

  Tcconfig is a command wrapper that makes it easy to set up traffic control of network bandwidth,
  latency, packet-loss, packet-corruption, etc.

  [See more on tcconfig](https://tcconfig.readthedocs.io/en/latest/index.html)

</details>
