### Understand the alert

`fping` is a command line tool to send ICMP (Internet Control Message Protocol) echo requests to network hosts, similar to ping, but performing much better when pinging multiple hosts. The Netdata
Agent utilizes `fping` to monitor latency, packet loss, uptime and reachability of any number of network endpoints.

The `fping_host_reachable` alert in the Netdata Agent checks the reachability of a network host (0: unreachable, 1: reachable). Receiving a critical alert indicates that your endpoints are unreachable. It is likely that the host is down or your system is experiencing networking issues.

### Troubleshoot the alert

- Check network connectivity

Verify that your system has access to the particular endpoint. Check for basic connectivity to known hosts from both your host and the endpoint.

- DNS settings

If you are using DNS resolution to check your endpoint, you should always consider check your DNS settings. To troubleshoot this issue, verify that your DNS can resolve your endpoints.

1. Check your current DNS (for example in linux you can use the host command):

      ```
      host -v <your_endpoint>
      ```

2. If the HTTP endpoint is supposed to be public facing endpoint, try an alternative DNS (for example Cloudflare's DNS):

      ```
      host -v <your_endpoint> 1.1.1.1
      ```
- Verify access restrictions in the remote host</summary>

If the remote host is a Linux-based machine and you have access to it, you can check the followings.
  
**Check the ICMP settings** 

In most linux distributions you can restrict the ICMP echo operations.
    
    1. Check your current setting. If this value is set to 1 your system ignore incoming ICMP echo requests.
          ```
          systemctl net.ipv4.icmp_echo_ignore_all
          ```
    2. To change this, bump this `net.ipv4.icmp_echo_ignore_all=0` entry under `/etc/sysctl.conf`.
    
    3. Reload the sysctl settings.
          ```
          sysctl -p
          ```

**Check your firewall rules** 
 
Depending on what firewall you use, the commands might differ from what's shown below. For example, if you are using IP tables you can check for restriction rules upon `icmp`.
      ```
      iptables -L | grep ICMP
      ```
    
For futher investigation or changes in your firewall settings we **strongly** advise you to consult your firewall's documentation and guidelines.