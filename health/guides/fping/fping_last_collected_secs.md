# fping_last_collected_secs

**Other | Network**

`fping` is a command line tool to send ICMP (Internet Control Message Protocol) echo request to
network hosts, similar to ping, but performing much better when pinging multiple hosts. The Netdata
Agent utilizes `fping` to monitor latency, packet loss, uptime and reachability of any number of
network endpoints.

The Netdata Agent monitors the number of seconds since the last successful data collection.
The `fping_last_collected_secs` alert indicates that no data collection has taken place with
the `fping.plugin` for some time.

### Troubleshooting section

<details>
<summary>Check Netdata's configuration</summary>

Consult the [fping.plugin guide](https://learn.netdata.cloud/docs/agent/collectors/fping.plugin/),
and the [fping man pages](https://fping.org/fping.1.html) to make sure that the fping options are
set correctly.

</details>



