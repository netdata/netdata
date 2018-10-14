# unbound

Monitoring uses the remote control interface to fetch statistics.

Provides the following charts:

1. **Queries Processed**
 * Ratelimited
 * Cache Misses
 * Cache Hits
 * Expired
 * Prefetched
 * Recursive

2. **Request List**
 * Average Size
 * Max Size
 * Overwritten Requests
 * Overruns
 * Current Size
 * User Requests

3. **Recursion Timings**
 * Average recursion processing time
 * Median recursion processing time

If extended stats are enabled, also provides:

4. **Cache Sizes**
 * Message Cache
 * RRset Cache
 * Infra Cache
 * DNSSEC Key Cache
 * DNSCrypt Shared Secret Cache
 * DNSCrypt Nonce Cache

### configuration

Unbound must be manually configured to enable the remote-control protocol.
Check the Unbound documentation for info on how to do this.  Additionally,
if you want to take advantage of the autodetection this plugin offers,
you will need to make sure your `unbound.conf` file only uses spaces for
indentation (the default config shipped by most distributions uses tabs
instead of spaces).

Once you have the Unbound control protocol enabled, you need to make sure
that either the certificate and key are readable by Netdata (if you're
using the regular control interface), or that the socket is accessible
to Netdata (if you're using a UNIX socket for the contorl interface).

By default, for the local system, everything can be auto-detected
assuming Unbound is configured correctly and has been told to listen
on the loopback interface or a UNIX socket.  This is done by looking
up info in the Unbound config file specified by the `ubconf` key.

To enable extended stats for a given job, add `extended: yes` to the
definition.

You can also enable per-thread charts for a given job by adding
`per_thread: yes` to the definition.  Note that the numbe rof threads
is only checked on startup.

A basic local configuration with extended statistics and per-thread
charts looks like this:

```yaml
local:
    ubconf: /etc/unbound/unbound.conf
    extended: yes
    per_thread: yes
```

While it's a bit more complicated to set up correctly, it is recommended
that you use a UNIX socket as it provides far better performance.

---
