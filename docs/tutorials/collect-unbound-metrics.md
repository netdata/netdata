# Monitor Unbound DNS servers with Netdata

[Unbound](https://nlnetlabs.nl/projects/unbound/about/) is a "validating, recursive, caching DNS resolver" from NLNet
Labs. In v1.19 of Netdata, we release a completely refactored collector for collecting real-time metrics from Unbound
servers and displaying them in Netdata dashboards.

Unbound runs on FreeBSD, OpenBSD, NetBSD, MacOS, Linux, and Windows, and supports DNS-over-TLS, which ensures that DNS
queries and answers are all encrypted with TLS. In theory, that should reduce the risk of eavesdropping or
man-in-the-middle attacks when communicating to DNS servers.

This tutorial will show you how to collect dozens of essential metrics from your Unbound servers with minimal
configuration.

## Set up your Unbound installation

As with all data sources, Netdata can auto-detect Unbound servers if you installed them using the standard installation
procedure.

Regardless of whether you're connecting to a local or remote Unbound server, you need to be able to access the server's
`remote-control` interface via an IP address, FQDN, or Unix socket.

To set up the `remote-control` interface, you can use `unbound-control`. First, run `unbound-control-setup` to generate
the TLS key files that will encrypt connections to the remote interface. Then add the following to the end of your
`unbound.conf` configuration file. See the [Unbound
documentation](https://nlnetlabs.nl/documentation/unbound/howto-setup/#setup-remote-control) for more details on using
`unbound-control`, such as how to handle situations when Unbound is run under a unique user.

```conf
# enable remote-control
remote-control:
    control-enable: yes
```

Next, make your `unbound.conf`, `unbound_control.key`, and `unbound_control.pem` files readable by Netdata.

```bash
sudo setfacl -m user:netdata:r unbound.conf
sudo setfacl -m user:netdata:r unbound_control.key
sudo setfacl -m user:netdata:r unbound_control.pem
```

Finally, ensure that your `unbound.conf` file is indented using spaces, not tabs, as our collector can't parse a
configuration file with tabs. There are many ways to accomplish this, but here is one option:

```bash
sed -i $'s/\t/    /g' *.conf
```

Finally, take note whether you're using Unbound in _cumulative_ or _non-cumulative_ mode. This will become relevant when
configuring the collector.

## Configure the Unbound collector module

To use the Go version of the Unbound collector, you need to explicitly enable it. Open your `go.d.conf` configuration
file.

```bash
cd /etc/netdata/ # Replace with your Netdata configuration directory, if not /etc/netdata/
./edit-config python.d.conf
```

Find the `unbound` line, uncomment it, and set it to `unbound: yes`.

To configure Netdata's Unbound collector module, navigate to your Netdata configuration directory (typically at
`/etc/netdata/`) and use `edit-config` to initialize and edit your Unbound configuration file.

```bash
cd /etc/netdata/ # Replace with your Netdata configuration directory, if not /etc/netdata/
sudo ./edit-config go.d/unbound.conf
```

The file contains all the global and job-related parameters. The `name` setting is required, and two Unbound servers
can't have the same name.

The collector supports both cumulative and non-cumulative modes. Visit Unbound's [statistics configuration
documentation](https://www.nlnetlabs.nl/documentation/unbound/howto-statistics/) for details on enabling cumulative
mode, if you're interested.

> It is important you know whether your Unbound server is running in cumulative or non-cumulative mode, as a conflict
> between modes will create incorrect charts.

At this point, you're ready to edit the `unbound.conf` file according to your needs.

### Auto-detect a local Unbound server

To attempt to auto-detect all your Unbound server's settings, set the `conf_path` parameter in addition to `name`:

```yaml
jobs:
  - name: local
    conf_path: /path/to/unbound.conf
```

Netdata will attempt to read `unbound.conf` to get the appropriate `address`, `cumulative`, `use_tls`, `tls_cert`, and
`tls_key` parameters. 

Restart Netdata with `service netdata restart`, or the appropriate method for your system. You should see Unbound
metrics in your Netdata dashboard! But, if that failed, you will need to manually configure `unbound.conf`. See the
[default `unbound.conf` file](https://github.com/netdata/go.d.plugin/blob/master/config/go.d/unbound.conf) for details.

### Manual setup for a remote Unbound server

Collecting metrics from remote Unbound servers requires manual configuration. There are too many possibilities to cover
all remote connections here, but the [default `unbound.conf`
file](https://github.com/netdata/go.d.plugin/blob/master/config/go.d/unbound.conf) contains a few useful examples:

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
    cumulative: yes
    use_tls: yes
    tls_cert: /etc/unbound/unbound_control.pem
    tls_key: /etc/unbound/unbound_control.key
```

To see all the available options, see the default [unbound.conf
file](https://github.com/netdata/go.d.plugin/blob/master/config/go.d/unbound.conf).

## Tweak Unbound collector alarms

**COMING SOON: Ilya is adding/updating alarms and I will finish this section then.**

## What's next?

Now that you're collecting metrics from your Unbound servers, let us know how it's working for you! There's always room
for improvement or refinement based on real-world use cases. Feel free to [file an
issue](https://github.com/netdata/netdata/issues/new?labels=bug%2C+needs+triage&template=bug_report.md) with your
thoughts.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Ftutorials%2Funbound-metrics&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
