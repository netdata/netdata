# Monitor Unbound DNS servers with Netdata

In v1.19 of Netdata, we release a completely refactored collector for servers running the
[Unbound](https://nlnetlabs.nl/projects/unbound/about/) DNS resolver.

Unbound is a "validating, recursive, caching DNS resolver"[1][] from NLNet Labs. It runs on FreeBSD, OpenBSD, NetBSD,
MacOS, Linux, and Windows, and supports DNS-over-TLS, which ensures that DNS queries and answers are all encrypted with
TLS. In theory, that should reduce the risk of eavesdropping or man-in-the-middle attacks when communicating to DNS
servers.

This tutorial will show you how to collect dozens of essential metrics from your Unbound servers with minimal
configuration.

## Set up your Unbound installations

As with all data sources, Netdata can auto-detect Unbound servers if you installed them using the standard installation
procedure.

Regardless of whether you're connecting to a local or remote Unbound server, you need to be able to access the server's
`remote-control` interface via an IP address, FQDN, or Unix socket.

To set up the `remote-control` interface, you can use `unbound-control`. First, run `unbound-control-setup` to generate
the TLS key files that will encrypt connections to the remote interface.

Finally, add the following to the end of your `unbound.conf` configuration file.

```conf
# enable remote-control
remote-control:
    control-enable: yes
```

See the [Unbound documentation](https://nlnetlabs.nl/documentation/unbound/howto-setup/#setup-remote-control) for more
details on using `unbound-control`, such as how to handle situations when Unbound is run under a unique user. Be sure to
read the documentation for your [`unbound.conf` file](https://nlnetlabs.nl/documentation/unbound/unbound.conf/) as well.

## Configure the Unbound collector module

To configure Netdata's Unbound collector module, navigate to your Netdata configuration directory (typically at
`/etc/netdata/`) and use `edit-config` to initialize and edit your Unbound configuration file.

```bash
cd /etc/netdata/
sudo ./edit-config go.d/unbound.conf
```

The file contains all the global and job-related parameters. The `name` setting is required, and two Unbound servers
can't have the same name.

There are a few methods of connecting to an Unbound server, depending on whether the server is local or remote. To see
all the available options, see the [unbound.conf
file](https://github.com/netdata/go.d.plugin/blob/master/config/go.d/unbound.conf).

### Auto-detect a local Unbound server

To attempt to auto-detect all your Unbound server's settings, set the `conf_path` parameter in addition to `name`:

```yaml
jobs:
  - name: local
    conf_path: /path/to/unbound.conf
```

Netdata will attempt to read `unbound.conf` to get the appropriate `address`, `cumulative`, `use_tls`, `tls_cert`, and
`tls_key` parameters. If that doesn't work, continue to the next section.

### Manual setup for a local Unbound server

If Netdata can't read your `unbound.conf` file, or it doesn't contain all the necessary configuration options, you will
need to set up 

For collecting metrics from a local Unbound server, you can try the following:

```yaml
jobs:
  - name: local
    address: 127.0.0.1:8953
    use_tls: yes
    tls_skip_verify: yes
    tls_cert: /etc/unbound/unbound_control.pem
    tls_key: /etc/unbound/unbound_control.key
```

Netdata will need read access to both `.pem` and `.key` files.

### Manual setup for a remote Unbound server

Collecting metrics from remote Unbound servers requires manual 

If your Unbound server has `statistics-cumulative` set to `yes` in your `unbound.conf` file, you can set the parameter
`cumulative` to `yes` to change the statistics collection mode.

Here are a few examples:

```yaml
jobs:
  - name: remote
    address: 203.0.113.10:8953
    use_tls: no

  - name: remote_cumulative
    address: 203.0.113.11:8953
    use_tls: no
    cumulative: yes

  - name: remote
    address: 203.0.113.10:8953
    use_tls: yes
    tls_cert: /etc/unbound/unbound_control.pem
    tls_key: /etc/unbound/unbound_control.key
```

To see all the available options, see the default [unbound.conf
file](https://github.com/netdata/go.d.plugin/blob/master/config/go.d/unbound.conf).

## Tweak Unbound collector alarms

t/k

## What's next?

Now that you're collecting metrics from your Unbound servers, let us know how it's working for you! There's always room
for improvement or refinement based on real-world use cases. Feel free to [file an
issue](https://github.com/netdata/netdata/issues/new?labels=bug%2C+needs+triage&template=bug_report.md) with your
thoughts.

[1]: https://nlnetlabs.nl/projects/unbound/about/
