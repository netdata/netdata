# Log Files

There are 3 kinds of log files:

1. error.log
2. access.log
3. debug.log

Any of them can be disabled by setting it to `/dev/null` or `none` in the [[Configuration]].
By default `error.log` and `access.log` are enabled. `debug.log` is only enabled if debugging is also enabled.

## Error Log

The `error.log` is the `stderr` of the netdata daemon and all plugins run by netdata.

So if any process in the netdata process tree writes anything to its standard error, it will appear in the `error.log`.

## Access Log

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

## Debug Log

Check the [[Tracing Options]] section.
