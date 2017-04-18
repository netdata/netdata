
Tracing is activated via the command line option `-df` or a config option `debug flags`. Both accept a hex number, to enable or disable specific sections.

You can find the options supported at [log.h](https://github.com/firehol/netdata/blob/master/src/log.h). They are the `D_*` defines.

The value `0xffffff` will enable all possible debug flags.

Once debugging is enabled, the file `/var/log/netdata/debug.log` will contain the messages.
