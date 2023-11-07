# haproxy_backend_status

**Web Proxy | HAProxy**

HAProxy is a free, very fast and reliable reverse-proxy offering high availability, load balancing,
and proxying for TCP and HTTP-based applications. It is particularly suited for very high traffic
web sites and powers a significant portion of the world's most visited ones. Over the years it has
become the de-facto standard opensource load balancer, is now shipped with most mainstream Linux
distributions, and is often deployed by default in cloud platforms.

The Netdata Agent monitors the average number of failed HAProxy backends over the last 10 seconds.
Receiving this alert (in critical state) means that one or more HAProxy backend are inaccessible or
offline.<sup> [1](https://www.haproxy.com/blog/the-four-essential-sections-of-an-haproxy-configuration/) </sup>

<details>
<summary>HA Proxy Backends</summary>

> A HA proxy `backend` is a set of servers that receives forwarded requests. Backends are defined in 
the backend section of the HAProxy configuration. In its most basic form, a backend can be defined by:
>
> - which load balance algorithm to use
>
> - a list of servers and ports
>
> A backend can contain one or many servers in it–generally speaking, adding more servers to your
backend will increase your potential load capacity by spreading the load over multiple servers.
Increase reliability is also achieved through this manner, in case some of your backend servers
become unavailable. <sup>[2](https://www.digitalocean.com/community/tutorials/an-introduction-to-haproxy-and-load-balancing-concepts) </sup>

</details>

<details>
<summary>References and Sources</summary>

1. [The Four Essential Sections of an HAProxy Configuration](https://www.haproxy.com/blog/the-four-essential-sections-of-an-haproxy-configuration/)

2. [HA proxy explained in DigitalOcean](https://www.digitalocean.com/community/tutorials/an-introduction-to-haproxy-and-load-balancing-concepts)

</details>

### Troubleshooting Section

<details>
<summary>Check the HA proxy's configuration file for errors</summary>

Making changes in the configuration file may introduce errors. Make sure your always validate the
correctness of the configuration file.

1. In most Linux distros you can run the following check:

```
root@netadata # haproxy -c -f /etc/haproxy/haproxy.cfg
```

</details>


<details>
<summary>Check the HA proxy service for errors</summary>

1. Use `journalctl` and inspect the log:

```
root@netdata # journalctl -u haproxy.service  --reverse
```

</details>

<details>
<summary>Check the HA proxy's log</summary>

1. By default HA proxy logs under `/var/log/haproxy.log`:

```
root@netdata # cat /var/log/haproxy.log | grep 'emerg\|alert\|crit\|err\|warning\|notice'
```

You can also search for log messages with `info` and  `debug` tags.

</details>
