# `systemd` journal plugin

[KEY FEATURES](#key-features) | [JOURNAL SOURCES](#journal-sources) | [JOURNAL FIELDS](#journal-fields) |
[PLAY MODE](#play-mode) | [FULL TEXT SEARCH](#full-text-search) | [PERFORMANCE](#query-performance) |
[CONFIGURATION](#configuration-and-maintenance) | [FAQ](#faq)

The `systemd` journal plugin by Netdata makes viewing, exploring and analyzing `systemd` journal logs simple and efficient.
It automatically discovers available journal sources, allows advanced filtering, offers interactive visual
representations and supports exploring the logs of both individual servers and the logs on infrastructure wide 
journal centralization servers.

![image](https://github.com/netdata/netdata/assets/2662304/691b7470-ec56-430c-8b81-0c9e49012679)

## Key features

- Works on both **individual servers** and **journal centralization servers**.
- Supports `persistent` and `volatile` journals.
- Supports `system`, `user`, `namespaces` and `remote` journals.
- Allows filtering on **any journal field** or **field value**, for any time-frame.
- Allows **full text search** (`grep`) on all journal fields, for any time-frame.
- Provides a **histogram** for log entries over time, with a break down per field-value, for any field and any time-frame.
- Works directly on journal files, without any other third party components.
- Supports coloring log entries, the same way `journalctl` does.
- In PLAY mode provides the same experience as `journalctl -f`, showing new logs entries immediately after they are received.

### Prerequisites

`systemd-journal.plugin` is a Netdata Function Plugin.

To protect your privacy, as with all Netdata Functions, a free Netdata Cloud user account is required to access it.
For more information check [this discussion](https://github.com/netdata/netdata/discussions/16136).

### Limitations

- This plugin is not available when Netdata is installed in a container. The problem is that `libsystemd` is not available in Alpine Linux (there is a `libsystemd`, but it is a dummy that returns failure on all calls). We plan to change this, by shipping Netdata containers based on Debian.
- For the same reason (lack of `systemd` support for Alpine Linux), the plugin is not available on `static` builds of Netdata (which are based on `muslc`, not `glibc`).

To use the plugin, install one of our native distribution packages, or install it from source.

## Journal Sources

The plugin automatically detects the available journal sources, based on the journal files available in
`/var/log/journal` (persistent logs) and `/run/log/journal` (volatile logs).

![journal-sources](https://github.com/netdata/netdata/assets/2662304/28e63a3e-6809-4586-b3b0-80755f340e31)

The plugin, by default, merges all journal sources together, to provide a unified view of all log messages available.

> To improve query performance, we recommend selecting the relevant journal source, before doing more analysis on the logs.

### `system` journals

`system` journals are the default journals available on all `systemd` based systems.

`system` journals contain:

- kernel log messages (via `kmsg`),
- audit records, originating from the kernel audit subsystem,
- messages received by `systemd-journald` via `syslog`,
- messages received via the standard output and error of service units,
- structured messages received via the native journal API.

### `user` journals

Unlike `journalctl`, the Netdata plugin allows viewing, exploring and querying the journal files of **all users**.

By default, each user, with a UID outside the range of system users (0 - 999), dynamic service users,
and the nobody user (65534), will get their own set of `user` journal files. For more information about
this policy check [Users, Groups, UIDs and GIDs on systemd Systems](https://systemd.io/UIDS-GIDS/).

Keep in mind that `user` journals are merged with the `system` journals when they are propagated to a journal
centralization server. So, at the centralization server, the `remote` journals contain both the `system` and `user`
journals of the sender.

### `namespaces` journals

The plugin auto-detects the namespaces available and provides a list of all namespaces at the "sources" list on the UI.

Journal namespaces are both a mechanism for logically isolating the log stream of projects consisting
of one or more services from the rest of the system and a mechanism for improving performance.

`systemd` service units may be assigned to a specific journal namespace through the `LogNamespace=` unit file setting.

Keep in mind that namespaces require special configuration to be propagated to a journal centralization server.
This makes them a little more difficult to handle, from the administration perspective.

### `remote` journals

Remote journals are created by `systemd-journal-remote`. This `systemd` feature allows creating logs centralization points within
your infrastructure, based exclusively on `systemd`.

Usually `remote` journals are named by the IP of the server sending these logs. The Netdata plugin automatically
extracts these IPs and performs a reverse DNS lookup to find their hostnames. When this is successful,
`remote` journals are named by the hostnames of the origin servers.

For information about configuring a journals' centralization server, check [this FAQ item](#how-do-i-configure-a-journals-centralization-server).

## Journal Fields

Fields found in the journal files are automatically added to the UI in multiple places to help you explore
and filter the data.

The plugin automatically enriches certain fields to make them more user-friendly:

- `_BOOT_ID`: the hex value is annotated with the timestamp of the first message encountered for this boot id.
- `PRIORITY`: the numeric value is replaced with the human-readable name of each priority.
- `SYSLOG_FACILITY`: the encoded value is replaced with the human-readable name of each facility.
- `ERRNO`: the numeric value is annotated with the short name of each value.
- `_UID` `_AUDIT_LOGINUID` and `_SYSTEMD_OWNER_UID`: the local user database is consulted to annotate them with usernames.
- `_GID`: the local group database is consulted to annotate them with group names.
- `_CAP_EFFECTIVE`: the encoded value is annotated with a human-readable list of the linux capabilities.
- `_SOURCE_REALTIME_TIMESTAMP`: the numeric value is annotated with human-readable datetime in UTC.

The values of all other fields are presented as found in the journals.

> IMPORTANT:
> `_UID` `_AUDIT_LOGINUID`, `_SYSTEMD_OWNER_UID` and `_GID` annotations are added during presentation and are taken
> from the server running the plugin. For `remote` sources, the names presented may not reflect the actual user and
> group names on the origin server. The numeric value will still be visible though, as-is on the origin server.

The annotations are not searchable with full text search. They are only added for the presentation of the fields. 

### Journal fields as columns in the table

All journal fields available in the journal files are offered as columns on the UI. Use the gear button above the table:

![image](https://github.com/netdata/netdata/assets/2662304/cd75fb55-6821-43d4-a2aa-033792c7f7ac)

### Journal fields as additional info to each log entry

When you click a log line, the `info` sidebar will open on the right of the screen, to provide the full list of fields related to this
log line. You can close this `info` sidebar, by selecting the filter icon at its top.

![image](https://github.com/netdata/netdata/assets/2662304/3207794c-a61b-444c-8ffe-6c07cbc90ae2)

### Journal fields as filters

The plugin presents a select list of fields as filters to the query, with counters for each of the possible values
for the field. This list can used to quickly check which fields and values are available for the entire time-frame
of the query.

Internally the plugin has:

1. A white-list of fields, to be presented as filters.
2. A black-list of fields, to prevent them from becoming filters. This list includes fields with a very high cardinality, like timestamps, unique message ids, etc. This is mainly for protecting the server's performance, to avoid building in memory indexes for the fields that almost each of their values is unique.

Keep in mind that the values presented in the filters, and their sorting is affected by the "full data queries"
setting:

![image](https://github.com/netdata/netdata/assets/2662304/ac710d46-07c2-487b-8ce3-e7f767b9ae0f)

When "full data queries" is off, empty values are hidden and cannot be selected. This is due to a limitation of
`libsystemd` that does not allow negative or empty matches. Also, values with zero counters may appear in the list.

When "full data queries" is on, Netdata is applying all filtering to the data (not `libsystemd`), but this means
that all the data of the entire time-frame, without any filtering applied, have to be read by the plugin to prepare
the response required. So, "full data queries" can be significantly slower over long time-frames.

### Journal fields as histogram sources

The plugin presents a histogram of the number of log entries across time.

The data source of this histogram can be any of the fields that are available as filters.
For each of the values this field has, across the entire time-frame of the query, the histogram will get corresponding
dimensions, showing the number of log entries, per value, over time.

The granularity of the histogram is adjusted automatically to have about 150 columns visible on screen.

The histogram presented by the plugin is interactive:

- **Zoom**, either with the global date-time picker, or the zoom tool in the histogram's toolbox.
- **Pan**, either with global date-time picker, or by dragging with the mouse the chart to the left or the right.
- **Click**, to quickly jump to the highlighted point in time in the log entries.

![image](https://github.com/netdata/netdata/assets/2662304/d3dcb1d1-daf4-49cf-9663-91b5b3099c2d)

## PLAY mode

The plugin supports PLAY mode, to continuously update the screen with new log entries found in the journal files.
Just hit the "play" button at the top of the Netdata dashboard screen.

On centralized log servers, PLAY mode provides a unified view of all the new logs encountered across the entire infrastructure,
from all hosts sending logs to the central logs server via `systemd-remote`.

## Full-text search

The plugin supports searching for any text on all fields of the log entries.

Full text search is combined with the selected filters.

The text box accepts asterisks `*` as wildcards. So, `a*b*c` means match anything that contains `a`, then `b` and then `c` with anything between them.

## Query performance

Journal files are designed to be accessed by multiple readers and one writer, concurrently.

Readers (like this Netdata plugin), open the journal files and `libsystemd`, behind the scenes, maps regions
of the files into memory, to satisfy each query.

On logs aggregation servers, the performance of the queries depend on the following factors:

1. The **number of files** involved in each query.

   This is why we suggest to select a source when possible.
   
2. The **speed of the disks** hosting the journal files.

   Journal files perform a lot of reading while querying, so the fastest the disks, the faster the query will finish.
   
3. The **memory available** for caching parts of the files.

   Increased memory will help the kernel cache the most frequently used parts of the journal files, avoiding disk I/O and speeding up queries.
   
4. The **number of filters** applied.

   Queries are significantly faster when just a few filters are selected.

In general, for a faster experience, **keep a low number of rows within the visible timeframe**.

Even on long timeframes, selecting a couple of filters that will result in a **few dozen thousand** log entries
will provide fast / rapid responses, usually less than a second. To the contrary, viewing timeframes with **millions
of entries** may result in longer delays.

The plugin aborts journal queries when your browser cancels inflight requests. This allows you to work on the UI
while there are background queries running.

At the time of this writing, this Netdata plugin is about 25-30 times faster than `journalctl` on queries that access
multiple journal files, over long time-frames.

During the development of this plugin, we submitted, to `systemd`, a number of patches to improve `journalctl`
performance by a factor of 14:

- https://github.com/systemd/systemd/pull/29365
- https://github.com/systemd/systemd/pull/29366
- https://github.com/systemd/systemd/pull/29261

However, even after these patches are merged, `journalctl` will still be 2x slower than this Netdata plugin,
on multi-journal queries.

The problem lies in the way `libsystemd` handles multi-journal file queries. To overcome this problem,
the Netdata plugin queries each file individually and it then it merges the results to be returned.
This is transparent, thanks to the `facets` library in `libnetdata` that handles on-the-fly indexing, filtering,
and searching of any dataset, independently of its source.

## Configuration and maintenance

This Netdata plugin does not require any configuration or maintenance.

## FAQ

### Can I use this plugin on journals' centralization servers?

Yes. You can centralize your logs using `systemd-journal-remote`, and then install Netdata
on this logs centralization server to explore the logs of all your infrastructure.

This plugin will automatically provide multi-node views of your logs and also give you the ability to combine the logs
of multiple servers, as you see fit.

Check [configuring a logs centralization server](#configuring-a-journals-centralization-server).

### Can I use this plugin from a parent Netdata?

Yes. When your nodes are connected to a Netdata parent, all their functions are available
via the parent's UI. So, from the parent UI, you can access the functions of all your nodes.

Keep in mind that to protect your privacy, in order to access Netdata functions, you need a
free Netdata Cloud account.

### Is any of my data exposed to Netdata Cloud from this plugin?

No. When you access the agent directly, none of your data passes through Netdata Cloud.
You need a free Netdata Cloud account only to verify your identity and enable the use of
Netdata Functions. Once this is done, all the data flow directly from your Netdata agent
to your web browser.

Also check [this discussion](https://github.com/netdata/netdata/discussions/16136).

When you access Netdata via `https://app.netdata.cloud`, your data travel via Netdata Cloud,
but they are not stored in Netdata Cloud. This is to allow you access your Netdata agents from
anywhere. All communication from/to Netdata Cloud is encrypted.

### What are `volatile` and `persistent` journals?

`systemd` `journald` allows creating both `volatile` journals in a `tmpfs` ram drive,
and `persistent` journals stored on disk.

`volatile` journals are particularly useful when the system monitored is sensitive to
disk I/O, or does not have any writable disks at all.

For more information check `man systemd-journald`.

### I centralize my logs with Loki. Why to use Netdata for my journals?

`systemd` journals have almost infinite cardinality at their labels and all of them are indexed,
even if every single message has unique fields and values.

When you send `systemd` journal logs to Loki, even if you use the `relabel_rules` argument to
`loki.source.journal` with a JSON format, you need to specify which of the fields from journald
you want inherited by Loki. This means you need to know the most important fields beforehand.
At the same time you loose all the flexibility `systemd` journal provides:
**indexing on all fields and all their values**.

Loki generally assumes that all logs are like a table. All entries in a stream share the same
fields. But journald does exactly the opposite. Each log entry is unique and may have its own unique fields.

So, Loki and `systemd-journal` are good for different use cases.

`systemd-journal` already runs in your systems. You use it today. It is there inside all your systems
collecting the system and applications logs. And for its use case, it has advantages over other
centralization solutions. So, why not use it?

### Is it worth to build a `systemd` logs centralization server?

Yes. It is simple, fast and the software to do it is already in your systems.

For application and system logs, `systemd` journal is ideal and the visibility you can get
by centralizing your system logs and the use of this Netdata plugin, is unparalleled.

### How do I configure a journals' centralization server?

A short summary to get journal server running can be found below.

For more options and reference to documentation, check `man systemd-journal-remote` and `man systemd-journal-upload`.

#### Configuring a journals' centralization server

On the centralization server install `systemd-journal-remote`, and enable it with `systemctl`, like this:

```sh
# change this according to your distro
sudo apt-get install systemd-journal-remote

# enable receiving
sudo systemctl enable --now systemd-journal-remote.socket
sudo systemctl enable systemd-journal-remote.service
```

`systemd-journal-remote` is now listening for incoming journals from remote hosts, on port `19532`.
Please note that `systemd-journal-remote` supports using secure connections.
To learn more run `man systemd-journal-remote`.

To change the protocol of the journal transfer (HTTP/HTTPS) and the save location, do:

```sh
# copy the service file
sudo cp /lib/systemd/system/systemd-journal-remote.service /etc/systemd/system/

# edit it
# --listen-http=-3 specifies the incoming journal for http.
# If you want to use https, change it to --listen-https=-3.
nano /etc/systemd/system/systemd-journal-remote.service

# reload systemd
sudo systemctl daemon-reload
```

To change the port, copy `/lib/systemd/system/systemd-journal-remote.socket` to `/etc/systemd/system/` and edit it.
Then do `sudo systemctrl daemon-reload`


#### Configuring journal clients to push their logs to the server

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

Remember to match the protocol (http/https) the server expects.

Edit `systemd-journal-upload`, and add `Restart=always` to make sure the client will keep trying to push logs, even if the server is temporarily not there, like this:

```sh
sudo systemctl edit systemd-journal-upload
```

At the top, add:

```
[Service]
Restart=always
```

Then, enable and start `systemd-journal-upload`, like this:

```sh
sudo systemctl enable systemd-journal-upload
sudo systemctl start systemd-journal-upload
```

Keep in mind that immediately after starting `systemd-journal-upload` on a server, a replication process starts pushing logs in the order they have been received. This means that depending on the size of the available logs, some time may be needed for Netdata to show the most recent logs of that server.

#### Limitations when using a logs centralization server

As of this writing `namespaces` support by `systemd` is limited:

- Docker containers cannot log to namespaces. Check [this issue](https://github.com/moby/moby/issues/41879).
- `systemd-journal-upload` automatically uploads `system` and `user` journals, but not `namespaces` journals. For this you need to spawn a `systemd-journal-upload` per namespace.

