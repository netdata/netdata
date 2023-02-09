<!--
title: "Install Netdata on FreeNAS"
custom_edit_url: https://github.com/netdata/netdata/edit/master/packaging/installer/methods/freenas.md
sidebar_label: "Install Netdata on FreeNAS"
learn_status: "Published"
learn_topic_type: "Tasks"
learn_rel_path: "Installation"
-->

# Install Netdata on FreeNAS

On FreeNAS-Corral-RELEASE (>=10.0.3 and <11.3), Netdata is pre-installed.

To use Netdata, the service will need to be enabled and started from the FreeNAS [CLI](https://github.com/freenas/cli).

To enable the Netdata service:

```sh
service netdata config set enable=true
```

To start the Netdata service:

```sh
service netdata start
```


