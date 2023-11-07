# 1m_tcp_syn_queue_cookies

## OS: Linux

This alert presents the average number of sent SYN cookies due to the full TCP SYN queue over the
sixty seconds. Receiving this means that the incoming traffic is excessive. SYN queue cookies are
used to resist any potential SYN flood attacks.

This alert is raised to warning when the average exceeds 1 and will enter critical when the
value exceeds an average of 5 sent SYN cookies in sixty seconds.

<details>
<summary>SYN Queue Cookies</summary>

> The SYN Queue stores inbound SYN packets (specifically: struct inet_request_sock). It is
> responsible for sending out SYN+ACK packets and retrying them on timeout.  \
> After transmitting the SYN+ACK, the SYN Queue waits for an ACK packet from the client - the last
> packet in the three-way-handshake. All received ACK packets must first be matched against the
> fully established connection table, and only then against data in the relevant SYN Queue. On
> SYN Queue match, the kernel removes the item from the SYN Queue, successfully creates a full
> connection (specifically: struct inet_sock), and adds it to the Accept Queue.<sup> [1](
> https://blog.cloudflare.com/syn-packet-handling-in-the-wild/) </sup>
</details>

<br>

<details>
<summary>This alert likely indicates a SYN flood.</summary>

> A SYN flood is a form of denial-of-service attack in which an attacker rapidly initiates a
> connection to a server without finalizing the connection. The server has to spend resources
> waiting for half-opened connections, which can consume enough resources to make the system
> unresponsive to legitimate traffic. <sup> [2](https://en.wikipedia.org/wiki/SYN_flood) </sup>
</details>

<br>

<details>
<summary>SYN Cookies</summary>

> SYN cookies are a technique used to resist IP address spoofing attacks. The technique's primary
> inventor, Daniel J. Bernstein, defines SYN cookies as "particular choices of initial TCP sequence
> numbers by TCP servers." In particular, the use of SYN cookies allows a server to avoid dropping
> connections when the SYN queue fills up. Instead of storing additional connections, a SYN queue
> entry is encoded into the sequence number sent in the SYN+ACK response. If the server then
> receives a subsequent ACK response from the client with the incremented sequence number, then the
> server is able to reconstruct the SYN queue entry using information encoded in the TCP
> sequence number and proceed as usual with the connection. <sup> [3](
> https://en.wikipedia.org/wiki/SYN_cookies) </sup>
</details>

<br>

<details>
  <summary>References and sources</summary>

1. [SYN packet handling](https://blog.cloudflare.com/syn-packet-handling-in-the-wild/)
2. [SYN Floods](https://en.wikipedia.org/wiki/SYN_flood)
3. [SYN Cookies](https://en.wikipedia.org/wiki/SYN_cookies)
4. [ip-sysctl.txt](https://www.kernel.org/doc/Documentation/networking/ip-sysctl.txt)
5. [Transmission Control Protocol](https://en.wikipedia.org/wiki/Transmission_Control_Protocol)

</details>

### Troubleshooting Section

<details>
<summary>If the traffic is legitimate, then increase the limit of the SYN queue.</summary>

If you can determine that the traffic is legitimate, consider expanding the limit of the SYN
queue through configuration;  \
*(If the traffic is not legitimate, then this is not safe! You will 
expose more resources to an attacker if the traffic is not legitimate.)*

1. Open the /etc/sysctl.conf file and look for the entry "net.core.somaxconn". This value will
   affect both SYN and accept queue limits on newer Linux systems.
2. Set the value accordingly (By default it is set to 128) `net.core.somaxconn=128` (if the value
   doesn't exist, append it to the file)
3. Save your changes and run;
   ```
   root@netdata~ #sysctl -p 
   ```
   to apply the changes.

> Note: Netdata strongly suggests knowing exactly what you are configuring before making system
> changes.
</details>
