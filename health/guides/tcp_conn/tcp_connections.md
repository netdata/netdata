# tcp_connections

## OS: Linux

This alert presents the percentage of used IPv4 TCP connections. If you receive it, it is an
indication of high IPv4 TCP connections utilization.

If this value is 100% then the system is no longer able to establish new TCP connections.

<details>
  
<summary>TCP Connections Alarm Settings</summary>

Inside the [tcp_conn.conf](
https://github.com/netdata/netdata/blob/master/health/health.d/tcp_conn.conf), on the `calc:` line,
there is this block of code:  
`(${tcp_max_connections} > 0) ? ( ${connections} * 100 / ${tcp_max_connections} ) : 0`

- That line of code will calculate the value of `$this` in the following lines.
- Essentially, if the max connections are not dynamic, and there is a limit, then we calculate the
  percentage of used IPv4 TCP connections. Otherwise, we have a dynamic threshold *(
  so `$ {tcp_max_connections}` may be nan or -1)*, which in this case the alert and `$this` will
  always be zero.
  
</details>

<br>

- This alert is raised to a state of warning when the percentage of used IPv4 TCP connections is
greater than 80% and less than 90%.
- If the percentage of used IPv4 TCP connections exceeds 90%, then the alert gets raised to critical.
