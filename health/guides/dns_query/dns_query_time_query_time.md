# dns_query_time_query_time

## DNS

This alert presents the average DNS query round trip time (RTT) over the last 10 seconds.

If the DNS query exceeds a time limit to complete its operation (aka if it times out), then the
alert is raised into warning.

<details><summary>What is Round Trip Time?</summary>

> In networking, round-trip time (RTT), also known as round-trip delay time (RTD) is defined as
> a metric that measures in milliseconds the amount of time it takes for a data packet to be
> sent plus the amount of time it takes for acknowledgement of that signal to be received. This
> time delay includes propagation times for the paths between the two communication endpoints.
> <sup>[1](https://www.stormit.cloud/post/what-is-round-trip-time-rtt-meaning-calculation)

</details>

<details><summary>What is the main cause of DNS Latency?</summary>

- Cache misses  
  Even if a resolver can provide very good cache hit latency, cache misses are unavoidable and are
  very costly in terms of latency.

  "Cache hits" is the terminology used for when a system asks a resolver about some data and the
  resolver can provide it because he has it cached locally.

  "Cache misses" occur when a system asks a resolver about some data, and he doesn't have it cached
  locally. Then the resolver has to talk to other name servers, to see if they have the data
  requested, which takes time and greatly increases latency.<sup>[2](
  https://developers.google.com/speed/public-dns/docs/performance#introduction_causes_and_mitigations_of_dns_latency) </sup>


- DNS Server Location
  > The location of the DNS server you're accessing plays a huge role in your latency. The
  > farther the server is to your place, the higher the latency gets. But this is not always
  > the case as centralized DNS servers' latency isn't affected by the distance from the user.
  > Transit links also vary from one server to another. Latency will be lower if the transit
  > links are equipped with up-to-date technology.<sup>[3](
  https://www.smartdnsproxy.com/news/smart-dns-proxy/what-is-dns-latency-and-why-should-you-care-158.aspx) </sup>


- Wireless networks
  > Wireless networks have higher latency compared to wired networks. This happens because the
  > transfer of data doesn't go through fixed lines. Instead, it goes through Wi-Fi routers or
  > satellite dishes. These devices' efficiency also depends on the location where they're placed.<sup>[3](
  https://www.smartdnsproxy.com/news/smart-dns-proxy/what-is-dns-latency-and-why-should-you-care-158.aspx) </sup>


- Malicious DNS Traffic
  > Malicious DNS traffic can also cause high latency. It's because the DNS server will work
  > double time in processing it. PRSD attacks are the most common type of malicious traffic.
  > When this happens, it causes a lot of malware and botnet queries which cause high recursion
  > rates. These consume and waste a lot of CPU cycles on the server.<sup>[3](
  https://www.smartdnsproxy.com/news/smart-dns-proxy/what-is-dns-latency-and-why-should-you-care-158.aspx) </sup>


- Under-scaling of DNS Server
  > Proper scaling of the DNS infrastructure is important. Because if it’s not scaled correctly,
  > chances are is that it will use too much CPU power. When this happens, it will impact the
  > latency and cause it to increase. The more the CPU is utilized, the higher your latency gets.<sup>[3](
  https://www.smartdnsproxy.com/news/smart-dns-proxy/what-is-dns-latency-and-why-should-you-care-158.aspx) </sup>

</details>

For further information, please refer to our *References and Sources* section.

<details><summary>References and Sources</summary>

1. [What is Round-Trip Time (RTT)?](
   https://www.stormit.cloud/post/what-is-round-trip-time-rtt-meaning-calculation)
2. [Causes and mitigations of DNS latency](
   https://developers.google.com/speed/public-dns/docs/performance#introduction_causes_and_mitigations_of_dns_latency)
3. [What is DNS Latency and Why Should You Care?](
   https://www.smartdnsproxy.com/news/smart-dns-proxy/what-is-dns-latency-and-why-should-you-care-158.aspx)
4. [Configure your network settings to use Google Public DNS](https://developers.google.com/speed/public-dns/docs/using)

</details>

### Troubleshooting Section

This alert can have multiple causes.

As a first step, you can try changing your DNS server. Your current configuration might be using a
slow server, or what your ISP provides might not be the best. You can find more information on
[Google Developers](https://developers.google.com/speed/public-dns/docs/using) on how to configure
your settings for your specific OS to use Google's Public DNS.

