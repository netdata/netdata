# portcheck_connection_timeouts

**Other | TCP endpoint**

TCP provides a “virtual connection” between two nodes. When there is no activity, “keep alive”
packet are exchanged on a regular interval. Should the “keep alive” not arrive after specified
amount of time, the “connection times out” because there was not traffic during the timeout
interval.

The Netdata Agent calculates the average ratio of timeouts over the last 5 minutes. Receiving this
alert means that the monitored endpoint is either unreachable or most likely you are experiencing
networking issues or, the remote host/service is overloaded.

This alert is triggered in warning state when the ratio of timeouts is between 10-40% and in
critical state when it is greater than 40%.

### Troubleshooting section

<details>

  <summary>General approach</summary>

You should try to use Wireshark to inspect the network packets in the remote

Wireshark is a free and open-source packet analyzer. It is used for network troubleshooting,
analysis, software and communications protocol development.

[See more about Wireshark here](https://www.wireshark.org/)

Since you might won't be able to probe your traffic with wireshark in your host machine, You can
export it in a dump file and analyze it in a second iteration.

1. Try to export the traffic in your remote with `tcpdump`.

  ```
  root@netdata # tcpdump -i any 'port <PORT_YOU_MONITOR>' -s 65535 -w output.pcap
  ```

You must stop the capture after a certain observation period (60s up to 5 minutes). This command
will create a dump file which can be interpreted by Wireshark that contains all the traffic from any
interface for a specific port.

