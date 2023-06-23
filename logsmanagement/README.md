# Logs Management

## Table of Contents

- [Summary](#summary)  
    - [Types of available collectors](#collector-types)  
- [Package Requirements](#package-requirements)
    - [Systemd](#requirements-systemd)
- [General Configuration](#general-configuration)
- [Collector-specific Configuration](#collector-configuration)
	- [Kernel logs (kmsg)](#collector-configuration-kmsg)
    - [Systemd](#collector-configuration-systemd)
	- [Docker events](#collector-configuration-docker-events)
	- [Web log](#collector-configuration-web-log)
	- [Syslog socket](#collector-configuration-syslog)
	- [Serial](#collector-configuration-serial)
- [Custom Charts](#custom-charts)
- [Streaming](#streaming)
	- [Example: Systemd log streaming](#streaming-systemd)
	- [Example: Kernel log streaming](#streaming-kmsg)
	- [Example: Generic log streaming](#streaming-generic)
	- [Example: Docker Events log streaming](#streaming-docker-events)
- [Troubleshooting](#troubleshooting)

<a name="summary"/>

## Summary

</a>

The Netdata logs management engine enables collection, processing, storage, streaming and querying of logs through the Netdata agent. The following pipeline depicts a high-level overview of the different stages that the logs have to pass through for this to be achieved:

![Logs management pipeline](https://github.com/netdata/netdata/assets/5953192/dd73382c-af4b-4840-a3fe-1ba5069304e8 "Logs management pipeline")

The [Fluent Bit](https://github.com/fluent/fluent-bit) project has been used as the logs collection and exporting / streaming engine, due to its stability and the variety of [collection (input) plugins](https://docs.fluentbit.io/manual/pipeline/inputs) that it offers. Each collected log record passes through the Fluent Bit engine first, before it gets buffered, parsed, compressed and (optionally) stored locally by the logs management engine. It can also be streamed to another Netdata or Fluent Bit instance (using Fluent Bit's [Forward](https://docs.fluentbit.io/manual/pipeline/outputs/forward) protocol), or exported using any other [Fluent Bit output](https://docs.fluentbit.io/manual/pipeline/outputs).

A bespoke circular buffering implementation has been used to maximize performance and optimize memory utilization. More technical details about how it works can be found [here](https://github.com/netdata/netdata/pull/13291#buffering).

To configure Netdata's logs management engine properly, please make sure you are aware of the following points:

* One collection cycle (at max) occurs per `update every` interval (in seconds) and any log records collected in a collection cycle are grouped and compressed together. As a result, a longer `update every` interval will reduce memory and disk space requirements.
* When collected logs contain parsable timestamps, these will be used to display metrics from parsed logs at the correct time in each chart, even if collection of said logs takes place *much* later than the time they were produced. How much later? Up to a configurable value of `update timeout` seconds. This mechanism ensures correct  parsing and querying of delayed logs that contain parsable timestamps (such as streamed inputs or buffered logs sources that write logs in batches), but the respective charts may lag behind some seconds up to that timeout. If no parsable timestamp is found, the collection timestamp will be used instead.

<a name="collector-types"/>

### Types of available collectors

</a>

The following log collectors are supported at the moment. The table will be updated as more collectors are added:
|  Collector    	| Log type      		| Description  |
| ------------  	| ------------  		| ------------ |
| kernel logs (kmsg)| `flb_kmsg`    		| Collection of new kernel ring buffer logs.|
| systemd       	| `flb_systemd` 		| Collection of journald logs.|
| docker events 	| `flb_docker_events` 	| Collection of docker events logs, similar to executing the `docker events` command.|
| web log       	| `flb_web_log` 		| Collection of Apache or Nginx access logs.|
| generic (tail)	| `flb_generic` 		| Collection of new logs from files by "tailing" them.|
| syslog socket   	| `flb_syslog`  		| Collection of RFC-3164 syslog logs by creating listening sockets.|
| serial        	| `flb_serial`  		| Collection of logs from a serial interface.|

<a name="package-requirements"/>

## Package Requirements

</a>

Netdata logs management introduces minimal additional package dependencies and those are actually [Fluent Bit dependencies](https://docs.fluentbit.io/manual/installation/requirements). The only extra build-time dependencies are:
- `flex` 
- `bison` 
- `musl-fts-dev` (Alpine Linux only)

However, there may be some exceptions to this rule as more collectors are added to the logs management engine, so if a specific collector is disabled due to missing dependencies, please refer to this section or check [Troubleshooting](#troubleshooting).

<a name="requirements-systemd"/>

### Systemd

</a>

If systemd development libraries are missing at build time, the systemd log collector will not be available. This can be fixed by installing the missing libraries prior to building the agent:

Debian and derivatives:
```
apt install libsystemd-dev
```
Red Hat Enterprise Linux and derivatives:
```
yum install systemd-devel
```
openSUSE:
```
zypper install systemd-devel
```

<a name="general-configuration"/>

## General Configuration

</a>

There are some fundamental configuration options that are common to all collector types. These options can be set globally in the `[logs management]` section of `netdata.conf` or customized per collector using `edit-config logsmanagement.conf`:

|  Configuration Option | Default 		| Description  |
|      :------------:  	| :------------:  | ------------ |
| `enabled` | `no` 		| Whether this log source will be monitored or not.
| `update every` 		| Equivalent value in `[logs management]` section of `netdata.conf` (or Netdata global value, if higher). | How often metrics in charts will be updated every (in seconds).
| `update timeout` 		| Equivalent value in `[logs management]` section of `netdata.conf` (or Netdata global value, if higher). | Maximum timeout charts may be delayed by while waiting for new logs.
| `log type` 			| `flb_generic`	| Type of this log collector, see [relevant table](#collector-types) for a complete list of supported collectors.
| `circular buffer max size` | Equivalent value in `[logs management]` section of `netdata.conf`. | Maximum RAM that can be used to buffer collected logs until they are saved to the disk database.
| `circular buffer drop logs if full` | Equivalent value in `[logs management]` section of `netdata.conf` (`no` by default). | If there are new logs pending to be collected and the circular buffer is full, enabling this setting will allow old buffered logs to be dropped in favor of new ones. If disabled, collection of new logs will be blocked until there is free space again in the buffer (no logs will be lost in this case, but logs will not be ingested in real-time).
| `compression acceleration` | Equivalent value in `[logs management]` section of `netdata.conf` (`1` by default). | Fine-tunes tradeoff between log compression speed and compression ratio, see [here](https://github.com/lz4/lz4/blob/90d68e37093d815e7ea06b0ee3c168cccffc84b8/lib/lz4.h#L195) for more details. 
| `db mode` | Equivalent value in `[logs management]` section of `netdata.conf` (`none` by default). | Mode of logs management database per collector. If set to `none`, logs will be collected, buffered, parsed and then discarded. If set to `full`, buffered logs will be saved to the logs management database instead of being discarded. When mode is `none`, logs management queries cannot be executed.
| `buffer flush to DB` | Equivalent value in `[logs management]` section of `netdata.conf` (`6` by default). | Interval in seconds at which logs will be transferred from RAM buffers to the database.
| `disk space limit` | Equivalent value in `[logs management]` section of `netdata.conf` (`500 MiB` by default). | Maximum disk space that all compressed logs in database can occupy (per log source). Once exceeded, oldest BLOB of logs will be truncated for new logs to be written over. Each log source database can contain a maximum of 10 BLOBs at any point, so each truncation equates to a deletion of about 10% of the oldest logs. The number of BLOBS will be configurable in a future release.
| `collected logs total chart enable` | Equivalent value in `[logs management]` section of `netdata.conf` (`no` by default). | Chart that shows the number of log records collected for this log source, since the last Netdata agent restart. Useful for debugging purposes.
| `collected logs rate chart enable` | Equivalent value in `[logs management]` section of `netdata.conf` (`yes` by default). | Chart that shows the rate that log records are collected at for this log source, since the last Netdata agent restart.

There are also two settings that cannot be set per log source, but can only be defined in the `[logs management]` section of `netdata.conf`:

|  Configuration Option | Default 		| Description  |
|      :------------:  	| :------------:  | ------------ |
| `circular buffer spare items` | `2` 	| Spare items to be allocated for each circular buffer. This is required to ensure new log collection will not be blocked if operations such as parsing take longer than normal to complete in case of high load, so the circular buffers cannot be flushed to the database or discarded in time. It is recommended not to modify the default value.|
| `db dir` 	| `/var/cache/netdata/logs_management_db` | Logs management database path, will be created if it does not exist.|

Also, the `log path` configuration option must be defined per log source in `logsmanagement.conf` for nearly any type of collector (with the exception of `kmsg` and the collectors that listen to network sockets). Some default examples in `logsmanagement.conf` use `log path = auto`. In those cases, an autodetection of the path will be attempted by searching through common paths where each log source is typically encountered at.

<a name="collector-configuration"/>

## Collector-specific Configuration

</a>

<a name="collector-configuration-kmsg"/>

### Kernel logs (kmsg)

</a>

TODO

NOTE / WARNING: `kmsg` timestamps will be wrong if system has been suspended and resumed.

<a name="collector-configuration-systemd"/>

### Systemd

</a>

|  Configuration Option | Description  |
|      :------------:  	| ------------ |
| `priority value chart` | Enable chart showing Syslog Priority values (PRIVAL) of collected logs. The Priority value ranges from 0 to 191 and represents both the Facility and Severity. It is calculated by first multiplying the Facility number by 8 and then adding the numerical value of the Severity. Please see the [rfc5424: Syslog Protocol](https://www.rfc-editor.org/rfc/rfc5424#section-6.2.1) document for more information.|
| `severity chart` | Enable chart showing Syslog Severity values of collected logs. Severity values are in the range of 0 to 7 inclusive.|
| `facility chart` | Enable chart showing Syslog Facility values of collected logs. Facility values show which subsystem generated the log and are in the range of 0 to 23 inclusive.|

<a name="collector-configuration-docker-events"/>

### Docker events

</a>

|  Configuration Option | Description  |
|      :------------:  	| ------------ |
|`event type chart` | Enable chart showing the Docker object type of the collected logs.|

<a name="collector-configuration-web-log"/>

### Web log

</a>

**TODO**

<a name="collector-configuration-syslog"/>

### Syslog socket

</a>

This collector will collect logs through a Unix socket server (UDP or TCP) or over the network using TCP or UDP. See also documentation of [Fluent Bit syslog input plugin](https://docs.fluentbit.io/manual/v/1.9-pre/pipeline/inputs/syslog).

|  Configuration Option | Description  |
|      :------------:  	| ------------ |
|`mode` | Type of socket to be created to listen for incoming syslog messages. Supported modes are: `unix_tcp`, `unix_udp`, `tcp` and `udp`.|
| `log path` | If `mode == unix_tcp` or `mode == unix_udp`, Netdata will create a UNIX socket on this path to listen for syslog messages. Otherwise, this option is not used.|
| `unix_perm` | If `mode == unix_tcp` or `mode == unix_udp`, this sets the permissions of the generated UNIX socket. Otherwise, this option is not used.|
| `listen` | If `mode == tcp` or `mode == udp`, this sets the network interface to bind.|
| `port` | If `mode == tcp` or `mode == udp`, this specifies the port to listen for incoming connections.|
| `log format` | This is a Ruby Regular Expression to define the expected syslog format. Fluent Bit provides some [pre-configured syslog parsers](https://github.com/fluent/fluent-bit/blob/master/conf/parsers.conf#L65). |
|`priority value chart` | Please see the respective [systemd](#collector-configuration-systemd) configuration.|
| `severity chart` | Please see the respective [systemd](#collector-configuration-systemd) configuration.|
| `facility chart` | Please see the respective [systemd](#collector-configuration-systemd) configuration.|

 For parsing and metrics extraction to work properly, please ensure fields `<PRIVAL>`, `<SYSLOG_TIMESTAMP>`, `<HOSTNAME>`, `<SYSLOG_IDENTIFIER>`, `<PID>` and `<MESSAGE>` are defined in `log format`. For example, to parse incoming `syslog-rfc3164` logs, the following regular expression can be used:

```
/^\<(?<PRIVAL>[0-9]+)\>(?<SYSLOG_TIMESTAMP>[^ ]* {1,2}[^ ]* [^ ]* )(?<HOSTNAME>[^ ]*) (?<SYSLOG_IDENTIFIER>[a-zA-Z0-9_\/\.\-]*)(?:\[(?<PID>[0-9]+)\])?(?:[^\:]*\:)? *(?<MESSAGE>.*)$/
```

<a name="collector-configuration-serial"/>

### Serial

</a>

This collector will collect logs through a serial interface. See also documentation of [Fluent Bit serial interface input plugin](https://docs.fluentbit.io/manual/v/1.9-pre/pipeline/inputs/serial-interface).

|  Configuration Option | Description  |
|      :------------:  	| ------------ |
| `log path` | Absolute path to the device entry, e.g: `/dev/ttyS0`.|
| `bitrate` | The bitrate for the communication, e.g: 9600, 38400, 115200, etc..|
| `min bytes` | The minimum bytes the serial interface will wait to receive before it begines to process the log message.|
| `separator` | An optional separator string to determine the end of a log message.|
| `format` | Specify the format of the incoming data stream. The only option available is 'json'. Note that Format and Separator cannot be used at the same time.|

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
	update every = 1
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

<a name="streaming"/>

## Streaming

</a>

TODO

_How to configure streaming if child is Fluent-Bit?_

_How to configure streaming if child is Netdata?_

_What about Fluent-Bit retries, if network is down? Logs received out of order?_

_Compression via gzip_

Netdata supports 2 streaming configurations:
1. `syslog` messages over Unix or network sockets. 
2. Fluent Bit's [Forward protocol](https://docs.fluentbit.io/manual/pipeline/outputs/forward).

For option 1, please refer to the [syslog collector](#collector-configuration-syslog) section. This section will be focused on using option 2.

A Netdata agent can be used as a logs aggregation parent to listen to `Forward` messages, using either Unix or network sockets. This option is separate to [Netdata's metrics streaming](https://github.com/netdata/netdata/blob/master/docs/metrics-storage-management/enable-streaming.md) and can be used independently of whether that's enabled or not (and it uses a different listening socket too). 

To enable this option, `forward in enable = no` must be uncommented and set to `yes`, under `[logs management]` section in `netdata.conf`:
```
forward in enable = yes
# forward in unix path = 
# forward in unix perm = 0644
# forward in listen = 0.0.0.0
# forward in port = 24224
```

The default settings will listen for incoming `Forward` messages on TCP port 24224. If `forward in unix path` is set to a valid path, `forward in listen` and `forward in port` will be ignored and a unix socket will be created under that path. Make sure that `forward in unix perm` has the correct permissions set for that unix socket. Please also see Fluent Bit's [Forward input plugin documentation](https://docs.fluentbit.io/manual/pipeline/inputs/forward).

The Netdata agent will now listen for incoming `Forward` messages, but by default it won't process or store them. To do that, at least one log collection must be configured in `logsmanagement.conf`, to define how the incoming logs will be processed and stored. This is similar to configuring a local log source, with the difference that `log source = forward` must be set and also a `stream guid` must be defined, matching that of the children log sources. 

The rest of this section contains some examples on how to configure log collections of different types, using a Netdata parent and Fluent Bit children instances. Please use the recommended settings on children instances for parsing on parents to work correctly. Also, note that `Forward` output on children supports optional `gzip` compression, by using the `-p Compress=gzip` configuration parameter, as demonstrated in some examples.

<a name="streaming-systemd"/>

### Example: Systemd log streaming

</a>

Example configuration of an `flb_docker_events` type parent log collection:
```
[Forward systemd]

	## Required settings
	enabled = yes
	log type = flb_systemd

	## Optional settings, common to all log source. 
	## Uncomment to override global equivalents.
	# update every = 1
	# circular buffer max size MiB = 64
	# circular buffer drop logs if full = no
	# compression acceleration = 1
	# db mode = none
	# circular buffer flush to db = 6
	# disk space limit MiB = 500

	## Streaming input settings.
	log source = forward
	stream guid = 6ce266f5-2704-444d-a301-2423b9d30735

	## Other settings specific to this log source type
	priority value chart = yes
	severity chart = yes
	facility chart = yes
```

Any children can be configured as follows:
```
fluent-bit -i systemd -p Read_From_Tail=on -p Strip_Underscores=on -o forward -p Compress=gzip -F record_modifier -p 'Record="stream guid" 6ce266f5-2704-444d-a301-2423b9d30735' -m '*'
```

<a name="streaming-kmsg"/>

### Example: Kernel log streaming

</a>

Example configuration of an `flb_kmsg` type parent log collection:
```
[Forward kmsg]

	## Required settings
	enabled = yes
	log type = flb_kmsg

	## Optional settings, common to all log source. 
	## Uncomment to override global equivalents.
	# update every = 1
	# circular buffer max size MiB = 64
	# circular buffer drop logs if full = no
	# compression acceleration = 1
	# db mode = none
	# circular buffer flush to db = 6
	# disk space limit MiB = 500

	## Streaming input settings.
	log source = forward
	stream guid = 6ce266f5-2704-444d-a301-2423b9d30736

	## Other settings specific to this log source type
	severity chart = yes
	subsystem chart = yes
	device chart = yes
```
Any children can be configured as follows:
```
fluent-bit -i kmsg -o forward -p Compress=gzip -F record_modifier -p 'Record="stream guid" 6ce266f5-2704-444d-a301-2423b9d30736' -m '*'
```

> **Note**
> Fluent Bit's `kmsg` input plugin will collect all kernel logs since boot every time it's started up. Normally, when configured as a local source in a Netdata agent, all these initially collected logs will be discarded at startup so they are not duplicated. This is not possible when streaming from a Fluent Bit child, so every time a child is restarted, all kernel logs since boot will be re-collected and streamed again.

<a name="streaming-generic"/>

### Example: Generic log streaming

</a>

This is the most flexible option for a parent log collection, as it allows aggregation of logs from multiple children Fluent Bit instances of different log types. Example configuration of a generic parent log collection:

```
[Forward collection]

	## Required settings
	enabled = yes
	log type = flb_generic

	## Optional settings, common to all log source. 
	## Uncomment to override global equivalents.
	# update every = 1
	# circular buffer max size MiB = 64
	# circular buffer drop logs if full = no
	# compression acceleration = 1
	db mode = full
	# circular buffer flush to db = 6
	# disk space limit MiB = 500

	## Streaming input settings.
	log source = forward
	stream guid = 6ce266f5-2704-444d-a301-2423b9d30738
```

Children can be configured to `tail` local logs using Fluent Bit and stream them to the parent:
```
fluent-bit -i tail -p Path=/tmp/test.log -p Inotify_Watcher=true -p Refresh_Interval=1 -p Key=msg -o forward -p Compress=gzip -F record_modifier -p 'Record="stream guid" 6ce266f5-2704-444d-a301-2423b9d30738' -m '*'
```

Children instances do not have to use the `tail` input plugin specifically. Any of the supported log types can be used for the streaming child. The following configuration for example can stream `systemd` logs to the same parent as the configuration above:
```
fluent-bit -i systemd -p Read_From_Tail=on -p Strip_Underscores=on -o forward -p Compress=gzip -F record_modifier -p 'Record="stream guid" 6ce266f5-2704-444d-a301-2423b9d30738' -m '*'
```

The caveat is that an `flb_generic` log collection on a parent won't generate any type-specific charts by default, but [custom charts](#custom-charts) can be of course manually added by the user.

<a name="streaming-docker-events"/>

### Example: Docker Events log streaming

</a>

Example configuration of a `flb_docker_events` type parent log collection:
```
[Forward Docker Events]

	## Required settings
	enabled = yes
	log type = flb_docker_events

	## Optional settings, common to all log source. 
	## Uncomment to override global equivalents.
	# update every = 1
	# circular buffer max size MiB = 64
	# circular buffer drop logs if full = no
	# compression acceleration = 1
	# db mode = none
	# circular buffer flush to db = 6
	# disk space limit MiB = 500

	## Streaming input settings.
	log source = forward
	stream guid = 6ce266f5-2704-444d-a301-2423b9d30737

	## Other settings specific to this log source type
	event type chart = yes
```

Any children streaming to this collection must be set up to use one of the [default `json` or `docker` parsers](https://github.com/fluent/fluent-bit/blob/master/conf/parsers.conf), to send the collected log as structured messages, so they can be parsed by the parent:

```
fluent-bit -R ~/fluent-bit/conf/parsers.conf -i docker_events -p Parser=json -o forward -F record_modifier -p 'Record="stream guid" 6ce266f5-2704-444d-a301-2423b9d30737' -m '*'
``` 
or
```
fluent-bit -R ~/fluent-bit/conf/parsers.conf -i docker_events -p Parser=docker -o forward -F record_modifier -p 'Record="stream guid" 6ce266f5-2704-444d-a301-2423b9d30737' -m '*'
```

If instead the user desires to stream to a parent that collects logs into an `flb_generic` log collection, then a parser is not necessary and the unstructured logs can also be streamed in their original JSON format:
```
fluent-bit -i docker_events -o forward -F record_modifier -p 'Record="stream guid 6ce266f5-2704-444d-a301-2423b9d30737' -m '*'
```

Logs will appear in the parent in their unstructured format:

```
{"status":"create","id":"de2432a4f00bd26a4899dde5633bb16090a4f367c36f440ebdfdc09020cb462d","from":"hello-world","Type":"container","Action":"create","Actor":{"ID":"de2432a4f00bd26a4899dde5633bb16090a4f367c36f440ebdfdc09020cb462d","Attributes":{"image":"hello-world","name":"lucid_yalow"}},"scope":"local","time":1680263414,"timeNano":1680263414473911042}
```

</a>

<a name="troubleshooting"/>

## Troubleshooting

</a>

1. I am building Netdata from source but the `FLB_SYSTEMD` plugin is not  available / does not work: 

If during the Fluent Bit build step you are seeing the following message: 
```
-- Could NOT find Journald (missing: JOURNALD_LIBRARY JOURNALD_INCLUDE_DIR)
``` 
it means that the systemd development libraries are missing from your system. Please see [systemd collector](#requirements-systemd-collector).

2. Logs management and kernel log collection do not work at all and I am seeing the following error in `collector.log`:

```
[2020/10/20 10:39:06] [error] [plugins/in_kmsg/in_kmsg.c:291 errno=1] Operation not permitted
[2020/10/20 10:39:06] [error] Failed initialize input kmsg.0
```
Netdata is executed without root permissions, so the kernel ring buffer logs may not be accessible from normal users. Please try executing:
```
sudo sysctl kernel.dmesg_restrict=0
```

<!-- 3. The timestamp of some of the collected log records is wrong.

Some collection cycles may include log records with timestamps _preceding_ the cycle collection timestamp. This is expected, as the collection datetime is used instead of each log record's timestamp. -->
