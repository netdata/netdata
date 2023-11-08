### Understand the alert

The `unbound_request_list_overwritten` alert is triggered when Unbound, a popular DNS resolver, overwrites old queued requests because its request queue is full. This alert can indicate a Denial of Service (DoS) attack or network saturation.

### What does request list overwritten mean?

When the request queue is full, Unbound starts overwriting the oldest requests in the queue with newer incoming requests. This is done to handle increasing load, but it may also lead to dropped or lost queries.

### Troubleshoot the alert

- Check the Unbound log file for any unusual events or error messages. The default log file location is `/var/log/unbound.log`. You may find more information about the cause of the request queue overload, such as a high number of incoming queries or sudden spikes in traffic.

- Monitor Unbound's real-time statistics using the `unbound-control` command, which allows you to view various metrics related to the performance of the Unbound server:

  ```
  sudo unbound-control stats_noreset
  ```

  Look for the `num.query.list` and `num.query.list.overwritten` values to determine how many queries are in the request queue and how many of them are being overwritten.

- Analyze the incoming DNS queries to check for suspicious patterns, such as high query rates from specific clients or repeated queries for the same domain. You can use tools like `tcpdump` to capture and inspect DNS traffic:

  ```
  sudo tcpdump -i any -nn -s0 -w dns_traffic.pcap 'port 53'
  ```

  You can then analyze the captured data using packet analyzers like Wireshark or tshark.

- Increase the request queue length by adjusting the `num-queries-per-thread` value in the Unbound configuration file (`/etc/unbound/unbound.conf`), which determines the maximum number of queries that can be queued per thread before overwriting begins. Increasing this value may help to accommodate higher incoming query loads:

  ```
  server:
      num-queries-per-thread: 4096
  ```

  Remember to restart the Unbound service for the changes to take effect (`sudo systemctl restart unbound`).

- Consider implementing rate limiting to prevent a single client from overloading the server. Unbound supports rate limiting using the `ratelimit` configuration option:

  ```
  server:
      ratelimit: 1000
  ```

  This example sets a limit of 1000 queries per second, but you should tune it according to your environment.

### Useful resources

1. [Unbound Configuration Guide](https://nlnetlabs.nl/documentation/unbound/unbound.conf/)
2. [Unbound Rate Limiting](https://calomel.org/unbound_dns.html#ratelimit)
