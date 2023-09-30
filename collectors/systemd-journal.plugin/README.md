<!--
title: "SystemD-Journal"
description: "View and analyze logs available in systemd journal"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/systemd-journal.plugin/README.md"
sidebar_label: "SystemD-Journal"
learn_status: "Published"
learn_rel_path: "Integrations/Logs"
-->

# SystemD Journal

Netdata's SystemD Journal plugin allows viewing and analyzing `systemd` journal logs.

![image](https://github.com/netdata/netdata/assets/2662304/691b7470-ec56-430c-8b81-0c9e49012679)

## Key features:

- Works on both **individual servers** and **journal centralization servers**.
- Supports `persistent` and `volatile` journals.
- Supports `system`, `user`, `namespaces` and `remote` journals.
- Allows filtering on **any journal field** or **field value**, for any time-frame.
- Allows **full text search** (`grep`) on all journal fields, for any time-frame.
- Provides a **histogram** for log entries over time, with a break down per field-value, for any field and any time-frame.
- Works directly on journal files, without any other third party components.
- Supports coloring log entries, the same way `journalctl` does.
- PLAY mode to provide the same experience as `journalctl -f`, showing new logs entries immediately after they are received.

### Prerequisites

`systemd-journal.plugin` is a Netdata Function Plugin. To protect your privacy, as with all Netdata Functions, a free Netdata Cloud user account is required to access it.

### Limitations:

- This plugin is not available when Netdata is installed in a container. The problem is that `libsystemd` is not available in Alpine Linux (there is a `libsystemd`, but it is a dummy that returns failure on all calls). We plan to change this, by shipping Netdata containers based on Debian.
- For the same reason (lack of `systemd` support for Alpine Linux), the plugin is not available on `static` builds of Netdata (which are based on `muslc`, not `glibc`).

## Journal Sources

The plugin automatically generates the available journal sources, based on the journal files it detects in `/var/log/journal`.

![journal-sources](https://github.com/netdata/netdata/assets/2662304/28e63a3e-6809-4586-b3b0-80755f340e31)

The plugin, by default merges all journal sources together, to provide a unified view of all log messages available.

> To improve query performance, we recommend selecting the relevant journal source, before doing more analysis on the logs.

### `system` journals

These are the defaults journals available on all systems managed by `systemd`.
`system` journals contain:

- kernel log messages (via `kmsg`),
- audit records, originating from the kernel audit subsystem,
- messages received via `syslog`,
- messages received via the standard output and error of service units,
- structured messages received via the native journal API.

### `user` journals
By default, each user, with a UID outside the range of system users (0 - 999), dynamic service users, and the nobody user (65534), will get their own set of `user` journal files. For more information about this policy check [Users, Groups, UIDs and GIDs on systemd Systems](https://systemd.io/UIDS-GIDS/).

Netdata's `systemd-journal.plugin`  allows viewing and querying the journal files of all users.

### `namespaces` journals

Journal 'namespaces' are both a mechanism for logically isolating the log stream of projects consisting of one or more services from the rest of the system and a mechanism for improving performance. Systemd service units may be assigned to a specific journal namespace through the `LogNamespace=` unit file setting.

Netdata's `systemd-journal.plugin` auto-detects the namespaces available and provides a list of all namespaces at the sources list of the UI.

### `remote` journals

Remote journals are created by `systemd-journal-remote`. This feature allows creating logs centralization points within your infrastructure.

The Netdata plugin automatically extracts the remote IPs and based on their reverse DNS records it presents remote journals using the hostnames of the servers than pushed their metrics to the logs centralization server.

#### Configuring a journals centralization server

On the centralization server install `systemd-journal-remote`, and enable it with `systemctl`, like this:

```sh
# change this according to your distro
sudo apt-get install systemd-journal-remote

# enable receiving
sudo systemctl enable --now systemd-journal-remote.socket
sudo systemctl enable systemd-journal-remote.service
```

`systemd-journal-remote` is now listening for incoming journals from remote hosts, on port `19532`. Please note that `systemd-journal-remote` supports using secure connections. To learn more run `man systemd-journal-remote`.

#### Configuring clients to push their logs to the server

On the clients you want to centralize their logs, install `systemd-journal-remote`, configure `systemd-journal-upload`, enable it and start it with `systemctl`.

To install it run:

```sh
# change this according to your distro
sudo apt-get install systemd-journal-remote
```

Then, edit `/etc/systemd/journal-upload.conf` and set the IP address and the port of the server, like this:

```
[Upload]
URL=http://centralization.server.ip:19532
```

Finally, enable and start `systemd-journal-upload`, like this:

```sh
sudo systemctl enable systemd-journal-upload
sudo systemctl start systemd-journal-upload
```

Keep in mind that immediately after starting `systemd-journal-upload` on a server, a replication process starts pushing logs in the order they have been received. This means that depending on the size of the available logs, some time may be needed for Netdata to show the most recent logs of that server.

#### Limitations when using a logs centralization server

As of this writing `namespaces` support by systemd is limited:

- Docker containers cannot log to namespaces. Check [this issue](https://github.com/moby/moby/issues/41879).
- `systemd-journal-upload` automatically uploads `system` and `user` journals, but not `namespaces` journals. For this you need to spawn a `systemd-journal-upload` per namespace.

## Journal Fields

Fields found in the journal files are automatically added to the UI in multiple places to help you explore and filter the data.

### Journal fields as columns in the table

All journal fields available in the journal files, are offered as columns on the UI, using the gear button above the table:

![image](https://github.com/netdata/netdata/assets/2662304/cd75fb55-6821-43d4-a2aa-033792c7f7ac)

### Journal fields as additional info to each log entry

When you click a log line, the side bar on the right changes to provide the full list of fields related this log line. You can close this info sidebar, by selecting the filter icon at its top.

![image](https://github.com/netdata/netdata/assets/2662304/3207794c-a61b-444c-8ffe-6c07cbc90ae2)

### Journal fields as filters

The plugin presents a select list of fields as filters to the query, with counters for each of the possible values for the field.
This list can used to quickly check which fields and values are available for the entire time-frame of the query.

Internally the plugin has:

1. A white list of fields, that if they are encountered in the data set, they should be presented as filters.
2. A black list of fields, that even if they are encountered in the data set, they should never be presented as filters. This list includes fields with very high cardinatility, like timestamps, unique message ids, etc. This is mainly for protecting server performance, to avoid building in memory indexes for the fields that almost each of their values is unique.

Keep in mind that the values presented in the filters, and their sorting is affected by the "full data queries" setting, as shown in the image below.

![image](https://github.com/netdata/netdata/assets/2662304/ac710d46-07c2-487b-8ce3-e7f767b9ae0f)

When "full data queries" is off, empty values are hidden and cannot be selected. This is due to a limitation of `libsystemd` that does not allow negative or empty matches. Also, values with zero counters may appear in the list.

When "full data queries" is on, Netdata is applying all filtering to the data (not `libsystemd`), but this means that all the data of the entire time-frame, without any filtering applied, have to be read by the plugin to prepare the response.

### Journal fields as histogram sources

The histogram presented above the table of log entries, can use any of the fields that is available as filter as a data source. For each of the values this field has, across the entire time-frame of the query, the histogram will get a dimension, showing the number of messages over time.

The granularity of the histogram is adjusted automatically to have about 150 columns over time.
The histogram presented by the plugin is interactive:

- **Zoom**, either with the global date-time picker, or the zoom tool in the histogram's toolbox.
- **Pan**, either with global date-time picker, or by dragging with the mouse the chart to the left or the right.
- **Click**, to quickly jump to the highlighted point in time in the log entries.

![image](https://github.com/netdata/netdata/assets/2662304/d3dcb1d1-daf4-49cf-9663-91b5b3099c2d)

## PLAY mode

The plugin supports PLAY mode, to continuously update the screen with new log entries found in the journal files.

On centralized log servers, this provides a unified view of all the logs encountered across the entire infrastructure (depending on the selected sources and filters).

## Full text search

The plugin supports searching for any text on all fields of the log entries. Full text search is combined with the selected filters.

## Configuration and maintenance

This Netdata plugin does not require any configuration or maintenance.

## Access this plugin

This Netdata plugin is available to be used via Netdata Parents and Netdata Cloud.
