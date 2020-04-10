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

With the Netdata Agent running, visit `http://127.0.0.1/api/v1/info` in your browser. The returned JSON contains four
keys that will be helpful to diagnose any issues you might be having with the ACLK or claiming process.

```json
	"cloud-enabled"
	"cloud-available"
	"agent-claimed"
	"aclk-available"
```

Use these keys and the information below to troubleshoot the ACLK.

### cloud-enabled is false

If `cloud-enabled` is `false`, you probably ran the installer with `--disable-cloud` option.

Additionally, check that the `netdata cloud` setting in `netdata.conf` is set to `enable`:

```ini
[general]
    netadata cloud = enable
```

To fix this issue, reinstall Netdata using your [preferred method](../packaging/installer/README.md) and do not add the
`--disable-cloud` option.

### cloud-available is false

If `cloud-available` is `false` after you verified Cloud is enabled in the previous step, the most likely issue is that
Cloud features failed to build during installation.

If Cloud features fail to build, the installer continues and finishes the process without Cloud functionality as opposed
to failing the installation altogether. We do this to ensure the Agent will always finish installing.

If you can't see an explicit error in the installer's output, you can run the installer with the `--require-cloud`
option. This option causes the installation to fail if Cloud functionality can't be built and enabled, and the
installer's output should give you more error details.

One common cause of the installer failing to build Cloud features is not having one of the following dependencies on your system:

-   `cmake`
-   OpenSSL, including the `devel` package

If the installer's output does not help you enable Cloud features, contact us by [creating an issue on
GitHub](https://github.com/netdata/netdata/issues/new?labels=bug%2C+needs+triage%2C+ACLK&template=bug_report.md&title=The+installer+failed+to+prepare+the+required+dependencies+for+Netdata+Cloud+functionality) with details about your system and relevant output from `error.log`.

### agent-claimed is false

You must [claim your Agent](../claim/README.md).

### aclk-available is false

If `aclk-available` is `false` and all other keys are `true`, your Agent is having trouble connection to the Cloud
through the ACLK. Please check your firewall and Netdata proxy settings. 

If you are certain firewall and proxy settings are not the issue,
you should consult the Agent's `error.log` at `/var/log/netdata/error.log` and contact us by [creating an issue on
GitHub](https://github.com/netdata/netdata/issues/new?labels=bug%2C+needs+triage%2C+ACLK&template=bug_report.md&title=The+installer+failed+to+prepare+the+required+dependencies+for+Netdata+Cloud+functionality) with details about your system and relevant output from `error.log`.
