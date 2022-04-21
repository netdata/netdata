<!--
title: "ioping.plugin"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/ioping.plugin/README.md
description: "Netdata's ioping plugin allows you to measure your disk latency."
-->

# ioping.plugin

The ioping plugin supports monitoring latency for any number of directories/files/devices, by pinging them with `ioping`.
When your disks don't respond in the time you defined in `ioping.conf`, the plugin raises an alert (`ioping_disk_latency`).

The collector is enabled if Netdata detects that you have `ioping` installed. Otherwise, it is disabled. 

## Prerequisites

To use this plugin, you need to install `ioping`.

## Installing ioping

You can install `ioping` using the plugin.
The following command will download, build and install the correct `ioping` version as `/usr/libexec/netdata/plugins.d/ioping`.

To install `ioping`, run:

   ```sh
   /usr/libexec/netdata/plugins.d/ioping.plugin install
   ```
   Once `ioping` is installed, Netdata will automatically start collecting disk latency data.

> Note: If you set up Netdata using a different environment path than the default (`/etc/netdata/.environment`), use the option `-e` to indicate where the Netdata environment file is installed.

## Configuring ioping.plugin

Netdata ships each plugin and collector with a default configuration. To determine whether you need to make changes, check the [default configuration for `ioping.conf`](https://raw.githubusercontent.com/netdata/netdata/master/health/health.d/ioping.conf).

To make changes to the ioping.plugin configuration: 

1. Navigate to the [Netdata config directory](/docs/configure/nodes#the-netdata-config-directory):

   ```bash
   cd /etc/netdata
   ``` 

2. Use the [`edit-config`](/docs/configure/nodes#use-edit-config-to-edit-configuration-files) script to edit `ioping.conf`.

   ```bash
   sudo ./edit-config ioping.conf
   ```
 ### Example

```sh
# Uncomment the following line
ioping="/usr/libexec/netdata/plugins.d/ioping"

# Set the directory/file/device you need to ping
destination="destination"

# Override the chart update frequency - the default is inherited from Netdata
update_every="10s"

# The request size in bytes to ping the destination
request_size="4k"

# Other ioping options (- these are the defaults)
ioping_opts="-T 1000000 -R"
```

### Setting up multiple ioping plugins with different settings

In case you have multiple nodes that should be pinged in different intervals, you need to run multiple ioping plugins with different settings.
Netdata allows you to add as many `ioping` plugins as you like.
For example, you may need to ping one node once per 10 seconds, and another once per second.

To set up multiple ioping plugins:

1. Open a new terminal and change into the configuration directory.

   ```bash
   cd /etc/netdata
   ```

2. Copy the original ioping configuration file. You can name the file any name you like.

   ```bash
   cp ioping.conf ioping2.conf
   ```

3. Edit `ioping2.conf` and set the settings and the destination you need for the second instance.

4. To complete the next steps, you need change to the superuser.

   ```bash
   sudo su
   ```

5. Change into the plugins directory.

   ```bash
   cd /usr/libexec/netdata/plugins.d
   ```

6. Create a soft link to connect the original ioping plugin to new plugin file. Make sure the plugin and the configuration file have the same name (in our example "ioping2").

   ```bash
   ln -s ioping.plugin ioping2.plugin
   ```

   Netdata will detect the new plugin and start it.

## Metrics and Alerts produced by this collector

| Chart  | Metrics        | Alert                                                                             |
| ------ | -------------- | --------------------------------------------------------------------------------- |
| ioping | ioping.latency | [ioping_disk_latency](https://community.netdata.cloud/t/ioping-disk-latency/2120) |

## Related links

- [Default configuration for `ioping.conf`](https://raw.githubusercontent.com/netdata/netdata/master/health/health.d/ioping.conf)
- [Manpage for ioping options](https://www.systutorials.com/docs/linux/man/1-ioping/#lbAE)
- [Troubleshooting ioping alerts](https://community.netdata.cloud/t/ioping-disk-latency/2120)
