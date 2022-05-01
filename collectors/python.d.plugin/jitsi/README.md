<!--
title: "Jitsi Meet monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/jitsi/README.md
sidebar_label: "Jitsi Meet"
-->

# Jitsi Meet monitoring with Netdata

Collects metrics from the Jitsi Meet server components (currently only jvb is supported).

## Requirements

Private REST API needs to be configured first for the Jitsi videobridge component.

    1. Set `JVB_OPTS="--apis=rest"` in `/etc/jitsi/videobridge/config`

    2. Set `org.jitsi.videobridge.rest.private.jetty.host=127.0.0.1` in `/etc/jitsi/videobridge/sip-communicator.properties`

## Configuration

Edit the `python.d/jitsi.conf` configuration file using `edit-config` from the Netdata [config
directory](/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/jitsi.conf
```

```yaml
localhost:
  name : 'local'
  url  : 'http://127.0.0.1:8080'
```

When no configuration file is found, module tries to connect to `127.0.0.1:8080`.

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fjitsi%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
