# fping_packet_loss

## OS: Any

`fping` is a command line tool to send ICMP (Internet Control Message Protocol) echo requests to
network hosts, similar to ping, but performing much better when pinging multiple hosts. The Netdata
Agent utilizes `fping` to monitor latency, packet loss, uptime and reachability of any number of
network endpoints.

For `fping_packet_loss`, the Netdata Agent calculates the packet loss ratio to a network host
over the last 10 minutes. Receiving this alert indicates high packet loss towards a network host.
This could be caused by link congestion, link node faults, high server load, or incorrect system
settings.

### Troubleshooting

<details>
    <summary>Prioritize important traffic on your endpoint (linux based endpoints)</summary>

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
