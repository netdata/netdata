<!--
---
title: "Agent-cloud link (ACLK)"
custom_edit_url: https://github.com/netdata/netdata/edit/master/aclk/README.md
---
-->

# Agent-cloud link (ACLK)


## Configuration Options

In `netdata.conf`:

```ini
[cloud]
    proxy = none
```

Parameter proxy can take one of the following values:

- `env` - the default (try to read environment variables `http_proxy` and `socks_proxy`)
- `none` - do not use any proxy (even if system configured otherwise)
- `socks5[h]://[user:pass@]host:ip` - will use specified socks proxy
- `http://[user:pass@]host:ip` - will use specified http proxy
