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


## Troubleshooting

With running Netdata agent visit this URL http://127.0.0.1/api/v1/info. JSON returned contains the following keys helpful to diagnose any issues you might be facing:

- In case `cloud-enabled` is `false` you either ran the installer with `--disable-cloud` flag and you need to rerun it. Additionally, check the following configuration key is set to `enabled`:
   ```ini
   [general]
       netadata cloud = XXX
   ```

- In case `cloud-available` is `false` after you verified Cloud is enabled in the previous step, something has caused the build of the Cloud features to fail during the installation. For user convenience, we install Netdata agent without Cloud functionality in such cases as opposed to failing installation. In case you are not able to spot the error causing Cloud functionality to not be built, try running the installer with the option `--require-cloud` which will cause the installation to fail. This should make the error more obvious.

A common cause of failure to build Cloud features is lack of one of the following dependencies in the system:
- `cmake`
- OpenSSL incl. devel package
