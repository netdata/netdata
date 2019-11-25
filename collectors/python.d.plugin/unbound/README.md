# unbound

## Deprecation Notes

This module is deprecated. Please use [new version](https://github.com/netdata/go.d.plugin/tree/master/modules/unbound) instead.

___

Monitoring uses the remote control interface to fetch statistics.

Provides the following charts:

1.  **Queries Processed**

    -   Ratelimited
    -   Cache Misses
    -   Cache Hits
    -   Expired
    -   Prefetched
    -   Recursive

2.  **Request List**

    -   Average Size
    -   Max Size
    -   Overwritten Requests
    -   Overruns
    -   Current Size
    -   User Requests

3.  **Recursion Timings**

-   Average recursion processing time
-   Median recursion processing time

If extended stats are enabled, also provides:

4.  **Cache Sizes**

    -   Message Cache
    -   RRset Cache
    -   Infra Cache
    -   DNSSEC Key Cache
    -   DNSCrypt Shared Secret Cache
    -   DNSCrypt Nonce Cache

## Configuration

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

### Troubleshooting

If you've configured the module and can't get it to work, make sure and
check all of the following:

-   If you're using autodetection, double check that your `unbound.conf`
    file is actually using spaces instead of tabs, and that appropriate
    indentation is present.  Most Linux distributions ship a default config
    for Unbound that uses tabs, and the plugin can't read such a config file
    correctly.  Also, make sure this file is actually readable by Netdata.
-   Ensure that the control protocol is actually configured correctly.
    You can check this quickly by running `unbound-control stats_noreset`
    as root, which should print out a bunch of info about the internal
    statistics of the server.  If this returns an error, you don't have
    the control protocol set up correctly.
-   If using the regular control interface, make sure that the certificate
    and key file you have configured in `unbound.conf` are readable by
    Netdata.  In general, it's preferred to use ACL's on the files to
    provide the required permissions.
-   If using a UNIX socket, make sure that the socket is both readable
    _and_ writable by Netdata.  Just like with the regular control
    interface, it's preferred to use ACL's to provide these permissions.
-   Make sure that SELinux, Apparmor, or any other mandatory access control
    system isn't interfering with the access requirements mentioned above.
    In some cases, you may have to add a local rule to allow this access.

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Funbound%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
