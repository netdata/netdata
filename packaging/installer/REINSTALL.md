# Reinstall Netdata

In certain situations, such as needing to enable a feature or troubleshoot an issue, you may need to reinstall the
Netdata Agent on your node.

## One-line installer script (`kickstart.sh`)

### Reinstalling with the same install type

Run the one-line installer script with the `--reinstall` parameter to reinstall the Netdata Agent. This will preserve
any [user configuration](https://github.com/netdata/netdata/blob/master/docs/netdata-agent/configuration/README.md) in `netdata.conf` or other files, and will keep the same install
type that was used for the original install.

If you used any [optional
parameters](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/kickstart.md#optional-parameters-to-alter-your-installation) during initial
installation, you need to pass them to the script again during reinstallation. If you cannot remember which options you
used, read the contents of the `.environment` file and look for a `REINSTALL_OPTIONS` line. This line contains a list of
optional parameters.

```bash
wget -O /tmp/netdata-kickstart.sh https://get.netdata.cloud/kickstart.sh && sh /tmp/netdata-kickstart.sh --reinstall
```

### Performing a clean reinstall

Run the one-line installer script with the `--reinstall-clean` parameter to perform a clean reinstall of the
Netdata Agent. This will wipe all existing configuration and historical data, but can be useful sometimes for
getting a badly broken installation working again. Unlike the regular `--reinstall` parameter, this may use a
different install type than the original install used.

If you used any [optional
parameters](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/kickstart.md#optional-parameters-to-alter-your-installation) during initial
installation, you need to pass them to the script again during reinstallation. If you cannot remember which options you
used, read the contents of the `.environment` file and look for a `REINSTALL_OPTIONS` line. This line contains a list of
optional parameters.

```bash
wget -O /tmp/netdata-kickstart.sh https://get.netdata.cloud/kickstart.sh && sh /tmp/netdata-kickstart.sh --reinstall-clean
```

### Changing the install type of an existing installation

The clean reinstall procedure outlined above can also be used to manually change the install type for an existing
installation. Without any extra parameters, it will automatically pick the preferred installation type for your
system, even if that has changed since the original install. If you want to force use of a specific install type,
you can use the `--native-only`, `--static-only`, or `--build-only` parameter to control which install type gets
used, just like with a new install.

When using the `--reinstall-clean` option to change the install type, you will need to manually preserve any
configuration or historical data you want to keep. The following directories may need to be preserved:

- `/etc/netdata` (`/opt/netdata/etc/netdata` for static installs): For agent configuration.
- `/var/lib/netdata` (`/opt/netdata/var/lib/netdata` for static installs): For claiming configuration.
- `/var/cache/netdata` (`/opt/netdata/var/cache/netdata` for static installs): For historical data.

When copying these directories back after the reinstall, you may need to update file ownership by running `chown
-R netdata:netdata` on them.

## Troubleshooting

If you still experience problems with your Netdata Agent installation after following one of these processes, the next
best route is to [uninstall](https://github.com/netdata/netdata/blob/master/packaging/installer/UNINSTALL.md) and then try a fresh installation using the [one-line
installer](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/kickstart.md).

You can also post to our [community forums](https://community.netdata.cloud/c/support/13) or create a new [bug
report](https://github.com/netdata/netdata/issues/new?assignees=&labels=bug%2Cneeds+triage&template=BUG_REPORT.yml).
