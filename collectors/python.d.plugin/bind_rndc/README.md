# ISC Bind collector

Collects Name server summary performance statistics using `rndc` tool.

## Metrics

See [metrics.csv](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/bind_rndc/metrics.csv) for a list of metrics.

## Requirements

-   Version of bind must be 9.6 +
-   Netdata must have permissions to run `rndc stats`

## Configuration

Edit the `python.d/bind_rndc.conf` configuration file using `edit-config` from the Netdata [config
directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/bind_rndc.conf
```

Sample:

```yaml
local:
  named_stats_path       : '/var/log/bind/named.stats'
```

If no configuration is given, module will attempt to read named.stats file  at `/var/log/bind/named.stats`




### Troubleshooting

To troubleshoot issues with the `bind_rndc` module, run the `python.d.plugin` with the debug option enabled. The 
output will give you the output of the data collection job or error messages on why the collector isn't working.

First, navigate to your plugins directory, usually they are located under `/usr/libexec/netdata/plugins.d/`. If that's 
not the case on your system, open `netdata.conf` and look for the setting `plugins directory`. Once you're in the 
plugin's directory, switch to the `netdata` user.

```bash
cd /usr/libexec/netdata/plugins.d/
sudo su -s /bin/bash netdata
```

Now you can manually run the `bind_rndc` module in debug mode:

```bash
./python.d.plugin bind_rndc debug trace
```

