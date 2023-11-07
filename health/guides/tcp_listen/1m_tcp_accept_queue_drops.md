# 1m_tcp_accept_queue_drops

## OS: Linux

This alert presents the average number of dropped packets in the TCP accept queue over the last
sixty seconds. If it is raised, then the system is dropping incoming TCP connections. This could also be
an indication of accepted queue overflow, low memory, security issues, no route to a destination,
etc. 
- This alert gets raised to warning when the value is greater than 1 and less than 5.
- If the number of queue drops over the last minute exceeds 5, then the alert gets raised to critical.


<details>
<summary>TCP Accept Queue Drops</summary>

The accept queue holds fully established TCP connections waiting to be handled by the listening
application. It overflows when the server application fails to accept new connections at the rate
they are coming in.

</details>

<details>
  <summary>References and sources</summary>

1. [ip-sysctl.txt](https://www.kernel.org/doc/Documentation/networking/ip-sysctl.txt)
2. [Transmission Control Protocol](https://en.wikipedia.org/wiki/Transmission_Control_Protocol)

</details>

### Troubleshooting Section

<details>
<summary>Check for queue overflows</summary>

If you receive this alert, then you can cross-check its results with the 
`1m_tcp_accept_queue_overflows` alert. If that alert is also in a warning or critical state, 
then the system is experiencing accept queue overflowing. To fix that you can do the following:

1. Open the /etc/sysctl.conf file and look for the entry " net.ipv4.tcp_max_syn_backlog".
   > The `tcp_max_syn_backlog` is the maximal number of remembered connection requests
   > (SYN_RECV), which have not received an acknowledgment from connecting client. <sup> [1](
   > https://www.kernel.org/doc/Documentation/networking/ip-sysctl.txt) </sup>
2. If the entry does not exist, then append the following default entry to the
   file; `net.ipv4.tcp_max_syn_backlog=1280`. Otherwise, adjust the limit to suit your needs.
3. Save your changes and run;
   ```
   root@netdata~ #sysctl -p 
   ```
   to apply the changes.

> Note: Netdata strongly suggests knowing exactly what values you need before making system changes.
</details>
