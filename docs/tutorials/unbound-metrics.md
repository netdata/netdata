# Monitor Unbound DNS resolvers with Netdata

In v1.19 of Netdata, we release a completely refactored collector for servers running the
[Unbound](https://nlnetlabs.nl/projects/unbound/about/) DNS resolver.

Unbound is a "validating, recursive, caching DNS resolver"[1] from NLNet Labs. It runs on FreeBSD, OpenBSD, NetBSD, MacOS, Linux, and Windows, and supports DNS-over-TLS, which ensures that DNS queries and answers are all encrypted with TLS. In theory, that should reduce the risk of eavesdropping or man-in-the-middle attacks on 

This tutorial will show you how to collect dozens of essential metrics from your Unbound servers with minimal
configuration.

## Set up your Unbound installations

As with all data sources, Netdata can auto-detect Unbound servers if you installed them using the standard installation
procedure.



If TLS is disabled, or you're connecting to the Unbound servers `remote-control` interface via a Unix socket, all you
need to know is the address.

If TLS is enabled, or the server is remote, you will also need 

## Configure the Unbound collector module

To configure Netdata's Unbound collector module, navigate to your Netdata configuration directory (typically at
`/etc/netdata/`) and use `edit-config` to initialize and edit your Unbound configuration file.

```bash
cd /etc/netdata/
sudo ./edit-config go.d/unbound.conf
```

The file contains all the global and job-related parameters. The `name` setting is required, and two Unbound servers
can't have the same name.

Here are a few examples:

```yaml
# [ JOBS ]
jobs:
  - name: local
    address: 127.0.0.1:8953
    use_tls: yes
    tls_skip_verify: yes
    tls_cert: /etc/unbound/unbound_control.pem
    tls_key: /etc/unbound/unbound_control.key

  - name: remote
    address: 203.0.113.10:8953
    use_tls: no

  - name: remote_cumulative
    address: 203.0.113.11:8953
    use_tls: no
    cumulative: yes
      
  - name: socket
    address: /var/run/unbound.sock
```



To see all the available options, see the [unbound.conf
file](https://github.com/netdata/go.d.plugin/blob/master/config/go.d/unbound.conf).

## Tweak Unbound collector alarms


## What's next?


[1]: https://nlnetlabs.nl/projects/unbound/about/