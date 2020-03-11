<!--
---
title: "ioping.plugin"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/ioping.plugin/README.md
---
-->

# ioping.plugin

The ioping plugin supports monitoring latency for any number of directories/files/devices,
by pinging them with `ioping`.

A recent version of `ioping` is required (one that supports option `-N`).
The supplied plugin can install it, by running:

```sh
/usr/libexec/netdata/plugins.d/ioping.plugin install
```

The `-e` option can be supplied to indicate where the Netdata environment file is installed. The default path is `/etc/netdata/.environment`.

The above will download, build and install the right version as `/usr/libexec/netdata/plugins.d/ioping`.

Then you need to edit `/etc/netdata/ioping.conf` (to edit it on your system run
`/etc/netdata/edit-config ioping.conf`) like this:

```sh
# uncomment the following line - it should already be there
ioping="/usr/libexec/netdata/plugins.d/ioping"

# set here the directory/file/device, you need to ping
destination="destination"

# override the chart update frequency - the default is inherited from Netdata
update_every="1s"

# the request size in bytes to ping the destination
request_size="4k"

# other iping options - these are the defaults
ioping_opts="-T 1000000 -R"
```

## alarms

Netdata will automatically attach a few alarms for each host.
Check the [latest versions of the ioping alarms](../../health/health.d/ioping.conf)

## Multiple ioping Plugins With Different Settings

You may need to run multiple ioping plugins with different settings or different end points.
For example, you may need to ping one destination once per 10 seconds, and another once per second.

Netdata allows you to add as many `ioping` plugins as you like.

Follow this procedure:

**1. Create New ioping Configuration File**

```sh
# Step Into Configuration Directory
cd /etc/netdata

# Copy Original ioping Configuration File To New Configuration File
cp ioping.conf ioping2.conf
```

Edit `ioping2.conf` and set the settings and the destination you need for the seconds instance.

**2. Soft Link Original ioping Plugin to New Plugin File**

```sh
# Become root (If The Step Step Is Performed As Non-Root User)
sudo su

# Step Into The Plugins Directory
cd /usr/libexec/netdata/plugins.d

# Link ioping.plugin to ioping2.plugin
ln -s ioping.plugin ioping2.plugin
```

That's it. Netdata will detect the new plugin and start it.

You can name the new plugin any name you like.
Just make sure the plugin and the configuration file have the same name.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fioping.plugin%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
