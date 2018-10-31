# Log Files

There are 3 kinds of log files:

1. `error.log`
2. `access.log`
3. `debug.log`

Any of them can be disabled by setting it to `/dev/null` or `none` in the [[Configuration]].
By default `error.log` and `access.log` are enabled. `debug.log` is only enabled if debugging/tracing is also enabled.

Log files are stored in `/var/log/netdata/` by default.

---

## error.log

The `error.log` is the `stderr` of the netdata daemon and all external plugins run by netdata.

So if any process, in the netdata process tree, writes anything to its standard error, it will appear in `error.log`.

For most netdata programs (including standard external plugins shipped by netdata), the following lines may appear:

tag|description
:--:|:----
`INFO`|Something important the user should know.
`ERROR`|Something that might disable a part of netdata.<br/>The log line includes `errno` (if it is not zero).
`FATAL`|Something prevented a program from running.<br/>The log line includes `errno` (if it is not zero) and the program exited.

So, when auto-detection of data collection fail, `ERROR` lines are logged and the relevant modules are disabled, but the program continues to run. When a netdata program cannot run at all, a `FATAL` line is logged.

---

## access.log

The `access.log` logs web requests. The format is:

```txt
DATE: ID: (sent/all = SENT_BYTES/ALL_BYTES bytes PERCENT_COMPRESSION%, prep/sent/total PREP_TIME/SENT_TIME/TOTAL_TIME ms): ACTION CODE URL
```

where:

 - `ID` is the client ID. Client IDs are auto-incremented every time a client connects to netdata.
 - `SENT_BYTES` is the number of bytes sent to the client, without the HTTP response header.
 - `ALL_BYTES` is the number of bytes of the response, before compression.
 - `PERCENT_COMPRESSION` is the percentage of traffic saved due to compression.
 - `PREP_TIME` is the time in milliseconds needed to prepared the response.
 - `SENT_TIME` is the time in milliseconds needed to sent the response to the client.
 - `TOTAL_TIME` is the total time the request was inside netdata (from the first byte of the request to the last byte of the response).
 - `ACTION` can be `filecopy`, `options` (used in CORS), `data` (API call).

---

## debug.log

Check the [[Tracing Options]] section.

## log rotation

The installer, when run as root, will install /etc/logrotate.d/netdata.
