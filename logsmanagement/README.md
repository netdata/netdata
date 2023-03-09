# Logs Management

## Table of Contents

- [Summary](#summary)  
- [Requirements](#requirements)
- [General Configuration](#general-configuration)
- [Collector-specific Configuration](#collector-configuration)
- [Custom Charts](#custom-charts)

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



|  Collector    | Log type      | Description  |
| ------------  | ------------  | ------------ |
| kmsg          | `flb_kmsg`    | Collection of new kernel ring buffer logs
| systemd       | `flb_systemd` | Collection of journald logs
| docker events | `flb_docker_events` | Collection of docker events logs, similar to executing the `docker events` command
| web log       | `flb_web_log` | Collection of Apache or Nginx access logs
| tail          | `flb_generic` | Collection of new logs from files by "tailing" them
| syslog        | `flb_syslog`  | Collection of RFC-3164 syslog logs by creating listening sockets
| serial        | `flb_serial`  | Collection of logs from a serial interface

<a name="requirements"/>

## Requirements

</a>

Logs management introduces minimal extra package dependencies: `flex` and `bison` packages are required to build Fluent Bit and that's it! 

However, there may be some exceptions to this rule as more collectors are added to the logs management engine, so if a specific collector is disabled due to missing dependencies, refer to this section.

### systemd collector

If systemd development libraries are missing at build time, the systemd log collector will not be available. This can be fixed by installing the missing libraries prior to building the agent:

Debian / Ubuntu:

```
apt install libsystemd-dev
```
or Centos / Fedora:
```
yum install systemd-devel
```

<a name="general-configuration"/>

## General Configuration

</a>

_TODO: Which sources support log path parameter?_
_TODO: Which sources support 'auto' parameter?_

There are some fundamental configuration options that are common to all collectors:

- `enabled`: Whether this log source will be monitored or not. Default: `no`.
- `update every`: How often the charts will be updated. Default: equivalent value in `[logs management]` section of `netdata.conf`. 
- `log type`: Type of this log. Default: `flb_generic`.
- `circular buffer max size`: Maximum RAM used to buffer collected logs until they are inserted in the database. Default: equivalent value in `[logs management]` section of `netdata.conf`.
- `circular buffer drop logs if full`: 
- `compression acceleration`: Fine-tunes tradeoff between log compression speed and compression ratio, see [here](https://github.com/lz4/lz4/blob/90d68e37093d815e7ea06b0ee3c168cccffc84b8/lib/lz4.h#L195) for more.
- `buffer flush to DB`: Interval at which logs will be transferred from in-memory buffers to the database.
- `disk space limit`: Maximum disk space that all compressed logs in database can occupy (per log source).

These options can be set globally in the `[logs management]` section of `netdata.conf` or customized per collector using `edit-config logsmanagement.conf`.


<a name="collector-configuration"/>

## Collector-specific Configuration

</a>

If /dev/kmsg permission denied for normal user (non-root):
```
sudo sysctl kernel.dmesg_restrict=0
```

For kmsg, pay attention to `KERNEL_LOGS_COLLECT_INIT_WAIT`.

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