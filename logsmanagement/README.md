# Logs Management

## Table of Contents

- [Summary](#summary)  
    - [Types of available collectors](#collector-types)  
- [Package Requirements](#package-requirements)
    - [systemd](#requirements-systemd)
- [General Configuration](#general-configuration)
- [Collector-specific Configuration](#collector-configuration)
    - [systemd](#collector-configuration-systemd)
	- [docker events](#collector-configuration-docker-events)
	- [web log](#collector-configuration-web-log)
	- [syslog](#collector-configuration-syslog)
	- [serial](#collector-configuration-serial)
- [Custom Charts](#custom-charts)
- [Troubleshooting](#troubleshooting)

<a name="summary"/>

## Summary

</a>

The logs management engine enables collection, processing, storage and querying of logs through the Netdata agent. The following pipeline depicts a high-level overview of the different stages that the logs have to pass through for this to be achieved:

![Logs management pipeline](https://user-images.githubusercontent.com/5953192/191845591-fea3392c-427a-4b56-95f4-e029775378b0.jpg "Logs management pipeline")

_TODO: (Add parsing -> visualization link)_

The [Fluent Bit](https://github.com/fluent/fluent-bit) project has been used at the logs collection engine, due to its stability and the variety of collection (input) plugins that it offers. Each collected log record passes through the Fluent Bit engine first, before it gets compressed and copied to the circular buffers of the logs management engine.

A bespoke circular buffering implementation was used to maximize performance and optimize memory utilization. More details about it can be found [here](https://github.com/netdata/netdata/pull/13291#buffering).

A few important things that users should be aware of:

* One collection cycle occurs per `update every` interval and any log records collected in a collection cycle are grouped and compressed together. So, a longer interval will reduce memory and disk space requirements.
* The timestamp of each collection cycle is the epoch datetime at the moment of the collection and __not the datetime of any log records__ (although it may the the same as them). As a result of that, some collection cycles may include log records with timestamps _preceding_ the cycle collection timestamp and this is normal.

<a name="collector-types"/>

### Types of available collectors

</a>

The following log collectors are supported at the moment. The table will be updated as more collectors are added:


|  Collector    | Log type      | Description  |
| ------------  | ------------  | ------------ |
| kmsg          | `flb_kmsg`    | Collection of new kernel ring buffer logs
| systemd       | `flb_systemd` | Collection of journald logs
| docker events | `flb_docker_events` | Collection of docker events logs, similar to executing the `docker events` command
| web log       | `flb_web_log` | Collection of Apache or Nginx access logs
| tail          | `flb_generic` | Collection of new logs from files by "tailing" them
| syslog        | `flb_syslog`  | Collection of RFC-3164 syslog logs by creating listening sockets
| serial        | `flb_serial`  | Collection of logs from a serial interface

<a name="package-requirements"/>

## Package Requirements

</a>

Netdata logs management introduces minimal additional package dependencies and those are actually [Fluent Bit dependencies](https://docs.fluentbit.io/manual/installation/requirements). The only extra build-time dependencies are:
 - `flex` 
 - `bison` 
 - `musl-fts-dev` (Alpine Linux only)

However, there may be some exceptions to this rule as more collectors are added to the logs management engine, so if a specific collector is disabled due to missing dependencies, please refer to this section.

<a name="requirements-systemd"/>

### systemd

</a>

If systemd development libraries are missing at build time, the systemd log collector will not be available. This can be fixed by installing the missing libraries prior to building the agent:

Debian / Ubuntu:

```
apt install libsystemd-dev
```
Centos / Fedora:
```
yum install systemd-devel
```

<a name="general-configuration"/>

## General Configuration

</a>

_TODO: Which sources support log path parameter?_
_TODO: Which sources support 'auto' parameter?_

There are some fundamental configuration options that are common to all collector types. These options can be set globally in the `[logs management]` section of `netdata.conf` or customized per collector using `edit-config logsmanagement.conf`:

|  Configuration Option | Default 		| Description  |
|      ------------  	| ------------  | ------------ |
| `enabled` | `no` 		| Whether this log source will be monitored or not.
| `update every` 		| Equivalent value in `[logs management]` section of `netdata.conf` (or Netdata global value, if higher). | How often collected metrics will be updated.
| `log type` 			| `flb_generic`	| Type of this log collector, see [relevant table](#collector-types) for a complete list of supported collectors.
| `circular buffer max size` | Default: equivalent value in `[logs management]` section of `netdata.conf`. | Maximum RAM that can be used to buffer collected logs until they are saved to the disk database.
| `circular buffer drop logs if full` | Equivalent value in `[logs management]` section of `netdata.conf` (`no` by default). | If there are new logs pending to be collected and the circular buffer is full, enabling this setting will allow old buffered logs to be dropped in favor of new ones. If disabled, collection of new logs will be blocked until there is free space again in the buffer (no logs will be lost in this case, but logs will not be ingested in real-time).
| `compression acceleration` | Equivalent value in `[logs management]` section of `netdata.conf` (`1` by default). | Fine-tunes tradeoff between log compression speed and compression ratio, see [here](https://github.com/lz4/lz4/blob/90d68e37093d815e7ea06b0ee3c168cccffc84b8/lib/lz4.h#L195) for more. 
| `db mode` | Equivalent value in `[logs management]` section of `netdata.conf` (`none` by default). | Mode of logs management database per collector. If set to `none`, logs will be collected, buffered, parsed and then discarded. If set to `full`, buffered logs will be saved to the logs management database instead of being discarded. When mode is `none`, logs management queries cannot be executed.
| `buffer flush to DB` | Equivalent value in `[logs management]` section of `netdata.conf` (`6` by default). | Interval in seconds at which logs will be transferred from RAM buffers to the database.
| `disk space limit` | Equivalent value in `[logs management]` section of `netdata.conf` (`500 MiB` by default). | Maximum disk space that all compressed logs in database can occupy (per log source). Once exceeded, oldest BLOB of logs will be truncated for new logs to be written over. Each log source database can contain a maximum of 10 BLOBs at any point, so each truncation equates to a deletion of about 10% of the oldest logs. The number of BLOBS will be configurable in a future release.

There are also two settings that cannot be set per log source, but can only be defined in the `[logs management]` section of `netdata.conf`:

|  Configuration Option | Default 		| Description  |
|      ------------  	| :------------:  | ------------ |
| `circular buffer spare items` | `2` 	| Spare items to be allocated for each circular buffer. This is required to ensure new log collection will not be blocked if operations such as parsing take longer than normal to complete in case of high load, so the circular buffers cannot be flushed to the database or discarded in time. It is recommended not to modify the default value.
| `db dir` 	| `/var/cache/netdata/logs_management_db` | Logs management database path, will be created if it does not exist.



<a name="collector-configuration"/>

## Collector-specific Configuration

</a>

<a name="collector-configuration-systemd"/>

### systemd

</a>

`priority value chart` : Enable chart showing Syslog Priority values (PRIVAL) of collected logs. The Priority value ranges from 0 to 191 and represents both the Facility and Severity. It is calculated by first multiplying the Facility number by 8 and then adding the numerical value of the Severity. Please see the [rfc5424: Syslog Protocol](https://www.rfc-editor.org/rfc/rfc5424#section-6.2.1) document for more information.

`severity chart` : Enable chart showing Syslog Severity values of collected logs. Severity values are in the range of 0 to 7 inclusive.

`facility chart` : Enable chart showing Syslog Facility values of collected logs. Facility values show which subsystem generated the log and are in the range of 0 to 23 inclusive.

<a name="collector-configuration-docker-events"/>

### docker events

</a>

`event type chart` : Enable chart showing the Docker object type of the collected logs.

<a name="collector-configuration-web-log"/>

### web log

</a>

**TODO**

<a name="collector-configuration-syslog"/>

### syslog

</a>

`mode` : Type of socket to be created to listen for incoming syslog messages. Supported modes are: `unix_tcp`, `unix_udp`, `tcp` and `udp`. See also documentation of [Fluent Bit syslog input plugin](https://docs.fluentbit.io/manual/v/1.9-pre/pipeline/inputs/syslog).

`log path` : If `mode == unix_tcp` or `mode == unix_udp`, Netdata will create a UNIX socket on this path to listen for syslog messages. Otherwise, this option is not used.

`unix_perm` : If `mode == unix_tcp` or `mode == unix_udp`, this sets the permissions of the generated UNIX socket. Otherwise, this option is not used.

`listen` : If `mode == tcp` or `mode == udp`, this sets the network interface to bind.

`port` : If `mode == tcp` or `mode == udp`, this specifies the port to listen for incoming connections.

`log format` : This is a Ruby Regular Expression to define the expected syslog format, for example:
```
/^\<(?<PRIVAL>[0-9]+)\>(?<SYSLOG_TIMESTAMP>[^ ]* {1,2}[^ ]* [^ ]* )(?<HOSTNAME>[^ ]*) (?<SYSLOG_IDENTIFIER>[a-zA-Z0-9_\/\.\-]*)(?:\[(?<PID>[0-9]+)\])?(?:[^\:]*\:)? *(?<MESSAGE>.*)$/
```

 For the parsing to work properly, please ensure that fields `<PRIVAL>`, `<SYSLOG_TIMESTAMP>`, `<HOSTNAME>`, `<SYSLOG_IDENTIFIER>`, `<PID>` and `<MESSAGE>` are defined.


 `priority value chart` : Please see the respective [systemd](#collector-configuration-systemd) configuration.

`severity chart` : Please see the respective [systemd](#collector-configuration-systemd) configuration.

`facility chart` : Please see the respective [systemd](#collector-configuration-systemd) configuration.

<a name="collector-configuration-serial"/>

### serial

</a>

<a name="custom-charts"/>

## Custom Charts

</a>

In addition to the predefined charts, each log source supports the option to extract 
user-defined metrics, by matching log records to POSIX Extended Regular Expressions. 
This can be very useful particularly for `FLB_GENERIC` type log sources, where
there is no parsing at all by default.

To create a custom chart, the following key-value configuration options must be 
added to the respective log source configuration section:

    custom 1 chart = identifier
	custom 1 regex name = kernel
	custom 1 regex = .*\bkernel\b.*
	custom 1 ignore case = no

where the value denoted by:
    - `custom x chart` is the title of the chart.
    - `custom x regex name` is an optional name for the dimension of this particular metric 
    (if absent, the regex will be used as the dimension name instead).
    - `custom x regex` is the POSIX Extended Regular Expression to be used to match log records.
    - `custom x ignore case` is equivalent to setting `REG_ICASE` when using 
    POSIX Extended Regular Expressions for case insensitive searches. It is optional and defaults to `yes`. 

`x` must start from number 1 and monotonically increase by 1 every time a new regular expression is configured. 
If the titles of two or more charts of a certain log source are the same, the dimensions will be grouped together 
in the same chart, rather than a new chart being created.

Example of configuration for a generic log source collection with custom regex-based parsers:

```
[Auth.log]
	enabled = yes
	update every = 5
	log type = generic
	circular buffer max size = 256 # in MiB
	compression acceleration = 1 # see https://github.com/lz4/lz4/blob/90d68e37093d815e7ea06b0ee3c168cccffc84b8/lib/lz4.h#L195
	buffer flush to DB = 6 # in sec, default 6 min 4
	disk space limit = 500 # in MiB, default 500MiB
	log path = /var/log/auth.log

	custom 1 chart = sudo and su
	custom 1 regex name = sudo
	custom 1 regex = \bsudo\b
	custom 1 ignore case = yes

	custom 2 chart = sudo and su
	# custom 2 regex name = su
	custom 2 regex = \bsu\b
	custom 2 ignore case = yes

	custom 3 chart = sudo or su
	custom 3 regex name = sudo or su
	custom 3 regex = \bsudo\b|\bsu\b
	custom 3 ignore case = yes
```

And the generated charts based on this configuration:

![Auth.log](https://user-images.githubusercontent.com/5953192/197003292-13cf2285-c614-42a1-ad5a-896370c22883.PNG)

<a name="troubleshooting"/>

## Troubleshooting

</a>

1. I am building Netdata from source but the `FLB_SYSTEMD` plugin is not  available / does not work: 

If during the Fluent Bit build step you are seeing the following message: 
```
-- Could NOT find Journald (missing: JOURNALD_LIBRARY JOURNALD_INCLUDE_DIR)
``` 
it means that the systemd development libraries are missing from your system. Please see [systemd collector](#requirements-systemd-collector).

2. Logs management does not work at all and I am seeing the following error in the logs:
```
[2020/10/20 10:39:06] [error] [plugins/in_kmsg/in_kmsg.c:291 errno=1] Operation not permitted
[2020/10/20 10:39:06] [error] Failed initialize input kmsg.0
```
Netdata is executed without root permissions, so the kernel ring buffer logs may not be accessible from normal users. Please try executing:
```
sudo sysctl kernel.dmesg_restrict=0
```