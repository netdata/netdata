<!--
title: "Enable a collector & configure a module"
sidebar_label: "Enable a collector & configure a module"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/tasks/general-configuration/enable-a-collector-&-configure-a-module.md"
learn_status: "Published"
learn_topic_type: "Tasks"
learn_rel_path: "general-configuration"
learn_docs_purpose: "Instructions on how to enable a collector or its orchestrator, and how to configure a module"
-->

When Netdata starts up, each collector module searches for exposed metrics on the default endpoint established by that
service or application's standard installation procedure. For example, the [Nginx
collector module](https://github.com/netdata/go.d.plugin/blob/master/modules/nginx/README.md) searches at
`http://127.0.0.1/stub_status` for exposed metrics in the correct format. If an Nginx web server is running and exposes
metrics on that endpoint, the collector begins gathering them.

However, not every node or infrastructure uses standard ports, paths, files, or naming conventions. You may need to
enable or configure a collector to gather all available metrics from your systems, containers, or applications.

## Prerequisites

- Find the exact collector to enable, or the module to configure

## Enable a collector module or its orchestrator

You can enable/disable collector modules individually, or enable/disable entire orchestrators, using their configuration
files.
For example, you can change the behavior of the Go orchestrator, or any of its collector modules, by editing `go.d.conf`
.

Use our recommended method
of [editing configuration files](https://github.com/netdata/netdata/blob/master/docs/tasks/general-configuration/configure-the-agent.md)
followed by the name of the collector module or the orchestrator you want to enable:

```bash
cd /etc/netdata
sudo ./edit-config go.d.conf
```

Within this file, you can either disable the orchestrator entirely (`enabled: yes`), or find a specific collector module
and enable/disable it with `yes` and `no` settings. Uncomment any line you change to ensure the Netdata daemon reads it
on start.

After you make your
changes, [restart the Agent](https://github.com/netdata/netdata/blob/master/docs/tasks/general-configuration/start-stop-and-restart-agent.md)
for the changes to take effect.

## Configure a collector module

Open a configuration file by following the recommended way
of [editing configuration files](https://github.com/netdata/netdata/blob/master/docs/tasks/general-configuration/configure-the-agent.md)
. For example, edit the Nginx collector module with the following:

```bash
./edit-config go.d/nginx.conf
```

Each configuration file describes every available option and offers examples to help you tweak Netdata's settings
according to your needs. In addition, every collector's documentation shows the exact command you need to run to
configure that collector module. Uncomment any line you change to ensure the collector's orchestrator or the Netdata
daemon read it on start.

After you make your
changes, [restart the Agent](https://github.com/netdata/netdata/blob/master/docs/tasks/general-configuration/start-stop-and-restart-agent.md)
for the changes to take effect.