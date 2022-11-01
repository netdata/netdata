<!--
title: "fping.plugin"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/fping.plugin/README.md
-->

# fping.plugin

The fping plugin supports monitoring latency, packet loss and uptime of any number of network end points,
by pinging them with `fping`.

This plugin requires version 5.1 or newer of `fping` (earlier versions may or may not work). Our static builds and
Docker images come bundled with a known working version of `fping`. Native packages and local builds will need to
have a working version installed before the plugin is usable.

## Installing fping locally

If your distribution’s repositories do not include a working version of `fping`, the supplied plugin can install
it, by running:

```sh
/usr/libexec/netdata/plugins.d/fping.plugin install
```

The above will download, build and install the right version as `/usr/local/bin/fping`. This requires a working C
compiler, GNU autotools (at least autoconf and automake), and GNU make. On Debian or Ubuntu, you can pull in most
of the required tools by installing the `build-essential` package (this should include everything except automake
and autoconf).

## Configuration

Then you need to edit `/etc/netdata/fping.conf` (to edit it on your system run
`/etc/netdata/edit-config fping.conf`) like this:

```sh
# set here all the hosts you need to ping
# I suggest to use hostnames and put their IPs in /etc/hosts
hosts="host1 host2 host3"

# override the chart update frequency - the default is inherited from Netdata
update_every=1

# time in milliseconds (1 sec = 1000 ms) to ping the hosts
# 200 = 5 pings per second
ping_every=200

# other fping options - these are the defaults
fping_opts="-R -b 56 -i 1 -r 0 -t 5000"
```

## alarms

Netdata will automatically attach a few alarms for each host.
Check the [latest versions of the fping alarms](https://raw.githubusercontent.com/netdata/netdata/master/health/health.d/fping.conf)

## Additional Tips

### Customizing Amount of Pings Per Second

For example, to update the chart every 10 seconds and use 2 pings every 10 seconds, use this:

```sh
# Chart Update Frequency (Time in Seconds)
update_every=10

# Time in Milliseconds (1 sec = 1000 ms) to Ping the Hosts
# The Following Example Sends 1 Ping Every 5000 ms
# Calculation Formula: ping_every = (update_every * 1000 ) / 2
ping_every=5000
```

### Multiple fping Plugins With Different Settings

You may need to run multiple fping plugins with different settings for different end points.
For example, you may need to ping a few hosts 10 times per second, and others once per second.

Netdata allows you to add as many `fping` plugins as you like.

Follow this procedure:

**1. Create New fping Configuration File**

```sh
# Step Into Configuration Directory
cd /etc/netdata

# Copy Original fping Configuration File To New Configuration File
cp fping.conf fping2.conf
```

Edit `fping2.conf` and set the settings and the hosts you need for the seconds instance.

**2. Soft Link Original fping Plugin to New Plugin File**

```sh
# Become root (If The Step Step Is Performed As Non-Root User)
sudo su

# Step Into The Plugins Directory
cd /usr/libexec/netdata/plugins.d

# Link fping.plugin to fping2.plugin
ln -s fping.plugin fping2.plugin
```

That's it. Netdata will detect the new plugin and start it.

You can name the new plugin any name you like.
Just make sure the plugin and the configuration file have the same name.


