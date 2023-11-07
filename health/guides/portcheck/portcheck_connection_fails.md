# portcheck_connection_fails

**Other | TCP endpoint**

The Netdata Agent calculates the average ratio of failed connections over the last 5 minutes. This
alert indicates that too many connections failed. Receiving this alert means that your endpoint is
unreachable due to: 

1. The service is no longer running or not working properly.
2. Access to this port is denied by a firewall.
3. Port forwarding rile is incorrectly configured
4. The IP of the node you want to access is set to a private IP address

This alert is triggered in warning state when the ratio of failed connections is between 10-40% and
in critical state when it is greater than 40%.

### Troubleshooting section

<details>
<summary>Check the firewall rules in the remote</summary>

Check the INPUT chain rules, verify that you have allowed access from the host (Agent configured in it)
to the remote node.

**IPtables**

    ```
    root@netdata # iptables -L INPUT
    ```

</details>