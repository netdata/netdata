<!--
title: "Install Netdata on Synology"
description: "The Netdata Agent can be installed on AMD64-compatible NAS systems using the 64-bit pre-compiled static binary."
custom_edit_url: https://github.com/netdata/netdata/edit/master/packaging/installer/methods/synology.md
-->

# Install Netdata on Synology

The documentation previously recommended installing the Debian Chroot package from the Synology community package
sources and then running Netdata from within the chroot. This does not work, as the chroot environment does not have
access to `/proc`, and therefore exposes very few metrics to Netdata. Additionally, [this
issue](https://github.com/SynoCommunity/spksrc/issues/2758), still open as of 2018/06/24, indicates that the Debian
Chroot package is not suitable for DSM versions greater than version 5 and may corrupt system libraries and render the
NAS unable to boot.

The good news is that our [one-line installation script](kickstart.md) works fine if your NAS is one that uses the amd64 architecture. It
will install the content into `/opt/netdata`, making future removal safe and simple.

## Run as netdata user

When Netdata is first installed, it will run as _root_. This may or may not be acceptable for you, and since other
installations run it as the `netdata` user, you might wish to do the same. This requires some extra work:

1.  Create a group `netdata` via the Synology group interface. Give it no access to anything.
2.  Create a user `netdata` via the Synology user interface. Give it no access to anything and a random password. Assign
    the user to the `netdata` group. Netdata will chuid to this user when running.
3.  Change ownership of the following directories, as defined in [Netdata
    Security](/docs/netdata-security.md#security-design):

```sh
chown -R root:netdata /opt/netdata/usr/share/netdata
chown -R netdata:netdata /opt/netdata/var/lib/netdata /opt/netdata/var/cache/netdata
chown -R netdata:root /opt/netdata/var/log/netdata
```

4. Uncomment and set `web files owner` to `root`, and `web files group` to `netdata` in
   the `/opt/netdata/etc/netdata/netdata.conf`.
5. Restart Netdata

```sh
/etc/rc.netdata restart
```

## Create startup script

Additionally, as of 2018/06/24, the Netdata installer doesn't recognize DSM as an operating system, so no init script is
installed. You'll have to do this manually:

1.  Add [this file](https://gist.github.com/oskapt/055d474d7bfef32c49469c1b53e8225f) as `/etc/rc.netdata`. Make it
    executable with `chmod 0755 /etc/rc.netdata`.
2.  Add or edit `/etc/rc.local` and add a line calling `/etc/rc.netdata` to have it start on boot:

```conf
# Netdata startup
[ -x /etc/rc.netdata ] && /etc/rc.netdata start
```

3. Make sure `/etc/rc.netdata` is executable: `chmod 0755 /etc/rc.netdata`.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fpackaging%2Finstaller%2Fmethods%2Fsynology&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
