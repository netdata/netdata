# haproxy_backend_server_status

**Web Proxy | HAProxy**

HAProxy is a free, fast and reliable reverse-proxy offering high availability, load balancing,
and proxying for TCP and HTTP-based applications. It is particularly suited for very high traffic
web sites and powers a significant portion of the world's most visited ones. Over the years it has
become the de-facto standard opensource load balancer, is now shipped with most mainstream Linux
distributions, and is often deployed by default in cloud platforms.

The Netdata Agent monitors the average number of failed HAProxy backend servers over the last 10
seconds. Receiving this alert (in critical state) means that one or more HAProxy backend servers are
inaccessible or offline.

_There are four essential sections to an HAProxy configuration file. They are global, defaults,
frontend, and backend. These four sections define how the server as a whole performs, what your
default settings are, and how client requests are received and routed to your backend
servers._ <sup> [1](https://www.haproxy.com/blog/the-four-essential-sections-of-an-haproxy-configuration/) </sup>

<details>
<summary>HA Proxy Backend Servers</summary>

Backend servers are the cornerstone of the HA proxy architecture. HA proxy organizes multiple
servers to `Backends` (a pool of servers) and implements different (defined by you) Layer 4 or Layer
7 load balancing algorithms to assign the incoming requests to each individual server.

> You can define a new server with the `server` setting or use the `default-server` configuration
which is configured once. Its first argument is a name, followed by the IP address and port of the
backend server. You can specify a domain name instead of an IP address. In that case, it will be
resolved at startup or, if you add a `resolvers` argument, it will be updated during runtime. If the
DNS entry contains an SRV record, the port and weight will be filled in from it too. If the port
isn’t specified, then HAProxy will use the same port that the client connected on, which is useful
for randomly used ports such as for active-mode FTP.
>
> Every `server` line should have a `maxconn` setting that limits the maximum number of concurrent
requests that the server will be given. Even if it’s just a guess, having a value here puts you on
the right foot for avoiding saturating your servers with requests and gives a baseline that can be
adjusted later.  <sup> [1](https://www.haproxy.com/blog/the-four-essential-sections-of-an-haproxy-configuration/) </sup>

</details>

<details>
<summary>References and sources</summary>

1. [The Four Essential Sections of an HAProxy Configuration](https://www.haproxy.com/blog/the-four-essential-sections-of-an-haproxy-configuration/)

</details>

### Troubleshooting section

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
