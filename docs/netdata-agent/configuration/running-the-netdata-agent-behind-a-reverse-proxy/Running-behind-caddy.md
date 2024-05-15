<!--
title: "Netdata via Caddy"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/Running-behind-caddy.md"
sidebar_label: "Netdata via Caddy"
learn_status: "Published"
learn_topic_type: "Tasks"
learn_rel_path: "Configuration/Secure your nodes"
-->

# Netdata via Caddy

To run Netdata via [Caddy v2 proxying,](https://caddyserver.com/docs/caddyfile/directives/reverse_proxy) set your Caddyfile up like this:

```caddyfile
netdata.domain.tld {
    reverse_proxy localhost:19999
}
```

Other directives can be added between the curly brackets as needed.

To run Netdata in a subfolder:

```caddyfile
netdata.domain.tld {
    handle_path /netdata/* {
        reverse_proxy localhost:19999
    }
}
```

## limit direct access to Netdata

You would also need to instruct Netdata to listen only to `127.0.0.1` or `::1`.

To limit access to Netdata only from localhost, set `bind socket to IP = 127.0.0.1` or `bind socket to IP = ::1` in `/etc/netdata/netdata.conf`.


