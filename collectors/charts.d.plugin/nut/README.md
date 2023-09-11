<!--
title: "UPS/PDU monitoring with Netdata"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/charts.d.plugin/nut/README.md"
sidebar_label: "UPS/PDU"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/Remotes/Devices"
-->

# UPS/PDU collector

Collects UPS data for all power devices configured in the system.

The following charts will be created:

1.  **UPS Charge**

-   percentage changed

2.  **UPS Battery Voltage**

-   current voltage
-   high voltage
-   low voltage
-   nominal voltage

3.  **UPS Input Voltage**

-   current voltage
-   fault voltage
-   nominal voltage

4.  **UPS Input Current**

-   nominal current

5.  **UPS Input Frequency**

-   current frequency
-   nominal frequency

6.  **UPS Output Voltage**

-   current voltage

7.  **UPS Load**

-   current load

8.  **UPS Temperature**

-   current temperature

## Configuration

If using [our official native DEB/RPM packages](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/packages.md), make sure `netdata-plugin-chartsd` is installed.

Edit the `charts.d/nut.conf` configuration file using `edit-config` from the Netdata [config
directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config charts.d/nut.conf
```

This is the internal default for `charts.d/nut.conf`

```sh
# a space separated list of UPS names
# if empty, the list returned by 'upsc -l' will be used
nut_ups=

# how frequently to collect UPS data
nut_update_every=2
```

---


