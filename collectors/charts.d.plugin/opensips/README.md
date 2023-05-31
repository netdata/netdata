<!--
title: "OpenSIPS monitoring with Netdata"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/charts.d.plugin/opensips/README.md"
sidebar_label: "OpenSIPS"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/Networking"
-->

# OpenSIPS collector

## Configuration

If using [our official native DEB/RPM packages](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/packages.md), make sure `netdata-plugin-chartsd` is installed.

Edit the `charts.d/opensips.conf` configuration file using `edit-config` from the Netdata [config
directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config charts.d/opensips.conf
```


