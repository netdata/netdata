<!--
title: "OpenSIPS monitoring with Netdata"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/charts.d.plugin/opensips/README.md"
sidebar_label: "OpenSIPS"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "References/Collectors references/Networking"
-->

# OpenSIPS monitoring with Netdata

## Configuration

Edit the `charts.d/opensips.conf` configuration file using `edit-config` from the Netdata [config
directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config charts.d/opensips.conf
```


