### Understand the alert

This alert presents the average number of overflows in the TCP accept queue over the last minute.

- This alert gets raised in a warning state when the value is greater than 1 and less than 5.
- If the overflow average exceeds 5 in the last minute, then the alert gets raised in the critical state.

### What is the Accept queue?

The accept queue holds fully established TCP connections waiting to be handled by the listening application. It overflows when the server application fails to accept new connections at the rate they are coming in.

### This alert might also indicate a SYN flood.

A SYN flood is a form of denial-of-service attack in which an attacker rapidly initiates a connection to a server without finalizing the connection. The server has to spend resources waiting for half-opened connections, which can consume enough resources to make the system unresponsive to legitimate traffic. 

### Troubleshooting Section

Increase the queue length

1. Open the /etc/sysctl.conf file and look for the entry " net.ipv4.tcp_max_syn_backlog".
   The `tcp_max_syn_backlog` is the maximal number of remembered connection requests (SYN_RECV), which have not received an acknowledgment from connecting client.
2. If the entry does not exist, you can append the following default entry to the file; `net.ipv4. tcp_max_syn_backlog=1280`. Otherwise, adjust the limit to suit your needs.
3. Save your changes and run;
   ```
   sysctl -p 
   ```
   
Note: Netdata strongly suggests knowing exactly what values you need before making system changes.

### Useful resources

1. [SYN Floods](https://en.wikipedia.org/wiki/SYN_flood)
2. [ip-sysctl.txt](https://www.kernel.org/doc/Documentation/networking/ip-sysctl.txt)
3. [Transmission Control Protocol](https://en.wikipedia.org/wiki/Transmission_Control_Protocol)

