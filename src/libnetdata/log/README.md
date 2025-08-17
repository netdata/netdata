# Netdata Logging

This document describes how Netdata generates its own logs, not how Netdata manages and queries logs databases.

Netdata provides enterprise-grade structured logging with full observability of all system events. The logging system is designed to be:

- **Structured** - All logs include rich contextual fields for filtering and analysis
- **Performant** - Minimal overhead with built-in flood protection
- **Flexible** - Multiple output formats and destinations
- **Secure** - Integrated with platform-native security features
- **Standards-compliant** - Compatible with syslog, journald, ETW, and JSON standards

## Log sources

Netdata supports the following log sources:

1. **daemon** - Core service lifecycle events, startup/shutdown, configuration changes, and fatal errors
2. **collector** - Data collection events from both internal and external collectors, including errors and warnings
3. **access** - Complete API access logs with request/response details, useful for security auditing
4. **health** - Alert state transitions, notifications, and health monitoring events

Each source can be independently configured with different outputs, formats, and verbosity levels.

## Log outputs

For each log source, Netdata supports the following output methods:

| Output       | Platform | Description                                    | Use Case                                                         |
| ------------ | -------- | ---------------------------------------------- | ---------------------------------------------------------------- |
| **off**      | All      | Disable this log source                        | Reduce log volume for specific sources                           |
| **journal**  | Linux    | systemd-journal with full structured fields    | **Recommended for Linux** - Native integration with journald     |
| **etw**      | Windows  | Event Tracing for Windows with structured data | **Recommended for Windows** - Rich field support in Event Viewer |
| **wel**      | Windows  | Windows Event Log with basic fields            | Fallback when ETW is unavailable                                 |
| **syslog**   | Unix     | Traditional syslog protocol                    | Legacy system compatibility                                      |
| **system**   | All      | Platform's default stderr/stdout               | Container environments                                           |
| **stdout**   | All      | Direct to Netdata's stdout                     | Debugging, containers                                            |
| **stderr**   | All      | Direct to Netdata's stderr                     | Debugging, containers                                            |
| **filename** | All      | Write to specified file path                   | Custom log management                                            |

On Linux, when systemd-journal is available, the default is `journal` for `daemon` and `collector` and `filename` for the rest. To decide if systemd-journal is available, Netdata checks:

1. `stderr` is connected to systemd-journald
2. `/run/systemd/journal/socket` exists
3. `/host/run/systemd/journal/socket` exists (`/host` is configurable in containers)

If any of the above is detected, Netdata will select `journal` for `daemon` and `collector` sources.

On Windows, the default is `etw` and if that is not available it falls back to `wel`. The availability of `etw` is decided at compile time.

## Log formats

Netdata supports multiple log formats to integrate with different systems:

| Format      | Description                                             | Example                                                    | Best For                        |
| ----------- | ------------------------------------------------------- | ---------------------------------------------------------- | ------------------------------- |
| **journal** | Native systemd-journal format with all fields preserved | Binary format with 65+ structured fields                   | Linux systems with journald     |
| **etw**     | Event Tracing for Windows structured format             | Structured events in Windows Event Viewer                  | Windows monitoring and analysis |
| **wel**     | Windows Event Log format with indexed fields            | String array format in Event Viewer                        | Windows legacy compatibility    |
| **json**    | Structured JSON with all fields as key-value pairs      | `{"time":1234567890000000,"level":"info","msg":"Started"}` | Modern log aggregation systems  |
| **logfmt**  | Space-separated key=value pairs                         | `time="2024-01-15T10:30:00.123Z" level=info msg="Started"` | Traditional log processors      |

The format is automatically selected based on the output destination, but can be manually specified in the configuration.

### Field Transformations (Annotators)

The LOGFMT, ETW, and WEL formats apply special transformations (annotators) to certain fields for better human readability:

| Field                          | Raw Value               | Transformation                     | Example                                           |
| ------------------------------ | ----------------------- | ---------------------------------- | ------------------------------------------------- |
| `time`                         | Unix epoch microseconds | RFC3339 with microsecond precision | `1737302400000000` → `"2025-01-19T16:00:00.000Z"` |
| `alert_notification_timestamp` | Unix epoch microseconds | RFC3339 with microsecond precision | `1737302400000000` → `"2025-01-19T16:00:00.000Z"` |
| `level`                        | Priority number (0-7)   | Text representation                | `6` → `info`                                      |
| `errno`                        | Error number            | Number + error string              | `2` → `2, No such file or directory`              |
| `winerror`                     | Windows error code      | Number + error message             | `5` → `5, Access is denied`                       |

**Formats using these transformations:**
- **LOGFMT** - All annotated fields are transformed for readability
- **ETW** (Event Tracing for Windows) - Uses the same transformations
- **WEL** (Windows Event Logs) - Uses the same transformations

**Formats NOT using these transformations:**
- **JSON** - Outputs raw values for all fields (no transformations applied)

## Log levels

Each time Netdata logs, it assigns a priority to the log. It can be one of this (in order of importance):

| Level     | Description                                                                            |
| --------- | -------------------------------------------------------------------------------------- |
| emergency | a fatal condition, Netdata will most likely exit immediately after.                    |
| alert     | a very important issue that may affect how Netdata operates.                           |
| critical  | a very important issue the user should know which, Netdata thinks it can survive.      |
| error     | an error condition indicating that Netdata is trying to do something, but it fails.    |
| warning   | something unexpected has happened that may or may not affect the operation of Netdata. |
| notice    | something that does not affect the operation of Netdata, but the user should notice.   |
| info      | the default log level about information the user should know.                          |
| debug     | these are more verbose logs that can be ignored.                                       |

For `etw` these are mapped to `Verbose`, `Informational`, `Warning`, `Error` and `Critical`.
For `wel` these are mapped to `Informational`, `Warning`, `Error`.

## Logs Configuration

Configuration is done in the `[logs]` section of `netdata.conf`:

```ini
[logs]
    # Global settings
    logs to trigger flood protection = 1000    # Number of logs to trigger protection
    logs flood protection period = 1m          # Time window for flood protection
    facility = daemon                          # Syslog facility (when using syslog)
    level = info                               # Minimum log level (daemon/collector only)
    
    # Per-source configuration
    daemon = journal                           # Daemon logs to systemd journal
    collector = journal                        # Collector logs to systemd journal  
    access = /var/log/netdata/access.log      # Access logs to file
    health = /var/log/netdata/health.log      # Health logs to file
```

### Key configuration options:

- **Flood Protection**: Prevents log storms from overwhelming the system. When triggered, logs are suppressed with a summary message.
- **Log Level**: Controls verbosity. Only messages at or above this level are logged.
- **Facility**: Used for syslog categorization (local0-local7, daemon, user, etc.)
- **Per-Source Control**: Each source can have independent settings for maximum flexibility.

### Advanced per-source configuration

Each source (`daemon`, `collector`, `access`, `health`) accepts this syntax:

```
source = {FORMAT},level={LEVEL},protection={LOGS}/{PERIOD}@{OUTPUT}
```

Where:
- `{FORMAT}` - One of the [log formats](#log-formats) (json, logfmt, etc.)
- `{LEVEL}` - Minimum [log level](#log-levels) to be logged
- `{LOGS}` - Number of logs to trigger flood protection for this source
- `{PERIOD}` - Time period for flood protection (e.g., 1m, 30s, 5m)
- `{OUTPUT}` - One of the [log outputs](#log-outputs) (journal, filename, etc.)

All parameters except `{OUTPUT}` are optional. The `@` can be omitted if only specifying output.

#### Examples:

```ini
# JSON format to file with debug level
daemon = json,level=debug@/var/log/netdata/daemon.json

# High-volume access logs with aggressive flood protection
access = logfmt,protection=10000/5m@/var/log/netdata/access.log

# Critical-only health alerts to syslog
health = level=critical@syslog

# Simple output specification
collector = journal
```

### Logs rotation

Netdata includes automatic log rotation support:

1. **Built-in logrotate configuration** at `/etc/logrotate.d/netdata`
2. **Signal handling**: Send `SIGHUP` to Netdata to reopen all log files
3. **Automatic handling** for journal and ETW outputs (managed by the OS)

Example logrotate configuration:
```
/var/log/netdata/*.log {
    daily
    rotate 7
    compress
    delaycompress
    missingok
    notifempty
    postrotate
        killall -USR2 netdata 2>/dev/null || true
    endscript
}
```

## Log Fields

<details>
<summary>All fields exposed by Netdata</summary>

|               `journal`                |      `logfmt` and `json`       |             `etw`             | `wel` | Description                                                                                               |
| :------------------------------------: | :----------------------------: | :---------------------------: | :---: | :-------------------------------------------------------------------------------------------------------- |
|      `_SOURCE_REALTIME_TIMESTAMP`      |             `time`             |          `Timestamp`          |   1   | the timestamp of the event (logfmt: RFC3339, json: Unix epoch microseconds)                               |
|          `SYSLOG_IDENTIFIER`           |             `comm`             |           `Program`           |   2   | the program logging the event                                                                             |
|            `ND_LOG_SOURCE`             |            `source`            |      `NetdataLogSource`       |   3   | one of the [log sources](#log-sources)                                                                    |
|         `PRIORITY`<br/>numeric         |        `level`<br/>text        |       `Level`<br/>text        |   4   | one of the [log levels](#log-levels)                                                                      |
|                `ERRNO`                 |            `errno`             |          `UnixErrno`          |   5   | the numeric value of `errno`                                                                              |
|                   -                    |           `winerror`           |        `WindowsError`         |   6   | Windows GetLastError() code                                                                               |
|            `INVOCATION_ID`             |               -                |        `InvocationID`         |   7   | a unique UUID of the Netdata session, reset on every Netdata restart, inherited by systemd when available |
|              `CODE_LINE`               |               -                |          `CodeLine`           |   8   | the line number of of the source code logging this event                                                  |
|              `CODE_FILE`               |               -                |          `CodeFile`           |   9   | the filename of the source code logging this event                                                        |
|            `CODE_FUNCTION`             |               -                |        `CodeFunction`         |  10   | the function name of the source code logging this event                                                   |
|                 `TID`                  |             `tid`              |          `ThreadID`           |  11   | the thread id of the thread logging this event                                                            |
|              `THREAD_TAG`              |            `thread`            |         `ThreadName`          |  12   | the name of the thread logging this event                                                                 |
|              `MESSAGE_ID`              |            `msg_id`            |          `MessageID`          |  13   | see [message IDs](#message-ids)                                                                           |
|              `ND_MODULE`               |            `module`            |           `Module`            |  14   | the Netdata module logging this event                                                                     |
|             `ND_NIDL_NODE`             |             `node`             |            `Node`             |  15   | the hostname of the node the event is related to                                                          |
|           `ND_NIDL_INSTANCE`           |           `instance`           |          `Instance`           |  16   | the instance of the node the event is related to                                                          |
|           `ND_NIDL_CONTEXT`            |           `context`            |           `Context`           |  17   | the context the event is related to (this is usually the chart name, as shown on netdata dashboards       |
|          `ND_NIDL_DIMENSION`           |          `dimension`           |          `Dimension`          |  18   | the dimension the event is related to                                                                     |
|           `ND_SRC_TRANSPORT`           |        `src_transport`         |       `SourceTransport`       |  19   | when the event happened during a request, this is the request transport                                   |
|            `ND_ACCOUNT_ID`             |          `account_id`          |          `AccountID`          |  20   | Netdata Cloud account identifier                                                                          |
|             `ND_USER_NAME`             |          `user_name`           |          `UserName`           |  21   | username making the request                                                                               |
|             `ND_USER_ROLE`             |          `user_role`           |          `UserRole`           |  22   | user's role in the space                                                                                  |
|            `ND_USER_ACCESS`            |         `user_access`          |         `UserAccess`          |  23   | user's access permissions                                                                                 |
|              `ND_SRC_IP`               |            `src_ip`            |          `SourceIP`           |  24   | when the event happened during an inbound request, this is the IP the request came from                   |
|             `ND_SRC_PORT`              |           `src_port`           |         `SourcePort`          |  25   | when the event happened during an inbound request, this is the port the request came from                 |
|        `ND_SRC_FORWARDED_HOST`         |      `src_forwarded_host`      |     `SourceForwardedHost`     |  26   | the contents of the HTTP header `X-Forwarded-Host`                                                        |
|         `ND_SRC_FORWARDED_FOR`         |      `src_forwarded_for`       |     `SourceForwardedFor`      |  27   | the contents of the HTTP header `X-Forwarded-For`                                                         |
|         `ND_SRC_CAPABILITIES`          |       `src_capabilities`       |     `SourceCapabilities`      |  28   | when the request came from a child, this is the communication capabilities of the child                   |
|           `ND_DST_TRANSPORT`           |        `dst_transport`         |    `DestinationTransport`     |  29   | when the event happened during an outbound request, this is the outbound request transport                |
|              `ND_DST_IP`               |            `dst_ip`            |        `DestinationIP`        |  30   | when the event happened during an outbound request, this is the IP the request destination                |
|             `ND_DST_PORT`              |           `dst_port`           |       `DestinationPort`       |  31   | when the event happened during an outbound request, this is the port the request destination              |
|         `ND_DST_CAPABILITIES`          |       `dst_capabilities`       |   `DestinationCapabilities`   |  32   | when the request goes to a parent, this is the communication capabilities of the parent                   |
|          `ND_REQUEST_METHOD`           |          `req_method`          |        `RequestMethod`        |  33   | when the event happened during an inbound request, this is the method the request was received            |
|           `ND_RESPONSE_CODE`           |             `code`             |        `ResponseCode`         |  34   | when responding to a request, this this the response code                                                 |
|           `ND_CONNECTION_ID`           |             `conn`             |        `ConnectionID`         |  35   | when there is a connection id for an inbound connection, this is the connection id                        |
|          `ND_TRANSACTION_ID`           |         `transaction`          |        `TransactionID`        |  36   | the transaction id (UUID) of all API requests                                                             |
|        `ND_RESPONSE_SENT_BYTES`        |          `sent_bytes`          |      `ResponseSentBytes`      |  37   | the bytes we sent to API responses                                                                        |
|        `ND_RESPONSE_SIZE_BYTES`        |          `size_bytes`          |      `ResponseSizeBytes`      |  38   | the uncompressed bytes of the API responses                                                               |
|      `ND_RESPONSE_PREP_TIME_USEC`      |           `prep_ut`            | `ResponsePreparationTimeUsec` |  39   | the time needed to prepare a response                                                                     |
|      `ND_RESPONSE_SENT_TIME_USEC`      |           `sent_ut`            |    `ResponseSentTimeUsec`     |  40   | the time needed to send a response                                                                        |
|     `ND_RESPONSE_TOTAL_TIME_USEC`      |           `total_ut`           |    `ResponseTotalTimeUsec`    |  41   | the total time needed to complete a response                                                              |
|             `ND_ALERT_ID`              |           `alert_id`           |           `AlertID`           |  42   | the alert id this event is related to                                                                     |
|          `ND_ALERT_EVENT_ID`           |        `alert_event_id`        |        `AlertEventID`         |  44   | a sequential number of the alert transition (per host)                                                    |
|          `ND_ALERT_UNIQUE_ID`          |       `alert_unique_id`        |        `AlertUniqueID`        |  43   | a sequential number of the alert transition (per alert)                                                   |
|        `ND_ALERT_TRANSITION_ID`        |     `alert_transition_id`      |      `AlertTransitionID`      |  45   | the unique UUID of this alert transition                                                                  |
|           `ND_ALERT_CONFIG`            |         `alert_config`         |         `AlertConfig`         |  46   | the alert configuration hash (UUID)                                                                       |
|            `ND_ALERT_NAME`             |            `alert`             |          `AlertName`          |  47   | the alert name                                                                                            |
|            `ND_ALERT_CLASS`            |         `alert_class`          |         `AlertClass`          |  48   | the alert classification                                                                                  |
|          `ND_ALERT_COMPONENT`          |       `alert_component`        |       `AlertComponent`        |  49   | the alert component                                                                                       |
|            `ND_ALERT_TYPE`             |          `alert_type`          |          `AlertType`          |  50   | the alert type                                                                                            |
|            `ND_ALERT_EXEC`             |          `alert_exec`          |          `AlertExec`          |  51   | the alert notification program                                                                            |
|          `ND_ALERT_RECIPIENT`          |       `alert_recipient`        |       `AlertRecipient`        |  52   | the alert recipient(s)                                                                                    |
|            `ND_ALERT_VALUE`            |         `alert_value`          |         `AlertValue`          |  54   | the current alert value                                                                                   |
|          `ND_ALERT_VALUE_OLD`          |       `alert_value_old`        |        `AlertOldValue`        |  55   | the previous alert value                                                                                  |
|           `ND_ALERT_STATUS`            |         `alert_status`         |         `AlertStatus`         |  56   | the current alert status                                                                                  |
|         `ND_ALERT_STATUS_OLD`          |       `alert_status_old`       |       `AlertOldStatus`        |  57   | the previous alert status                                                                                 |
|           `ND_ALERT_SOURCE`            |         `alert_source`         |         `AlertSource`         |  58   | the source of the alert                                                                                   |
|            `ND_ALERT_UNITS`            |         `alert_units`          |         `AlertUnits`          |  59   | the units of the alert                                                                                    |
|           `ND_ALERT_SUMMARY`           |        `alert_summary`         |        `AlertSummary`         |  60   | the summary text of the alert                                                                             |
|            `ND_ALERT_INFO`             |          `alert_info`          |          `AlertInfo`          |  61   | the info text of the alert                                                                                |
|          `ND_ALERT_DURATION`           |        `alert_duration`        |        `AlertDuration`        |  53   | the duration the alert was in its previous state                                                          |
| `ND_ALERT_NOTIFICATION_TIMESTAMP_USEC` | `alert_notification_timestamp` |  `AlertNotificationTimeUsec`  |  62   | the timestamp the notification delivery is scheduled                                                      |
|              `ND_REQUEST`              |           `request`            |           `Request`           |  63   | the full request during which the event happened                                                          |
|               `MESSAGE`                |             `msg`              |           `Message`           |  64   | the event message                                                                                         |
|            `ND_STACK_TRACE`            |         `stack_trace`          |         `StackTrace`          |  65   | stack trace at time of logging (on fatal errors)                                                          |

For `wel` (Windows Event Logs), all logs have an array of 64 fields strings, and their index number provides their meaning.
For `etw` (Event Tracing for Windows), Netdata logs in a structured way, and field names are available.

</details>

### Message IDs

Netdata assigns unique UUIDs to specific event types for easy filtering and correlation:

| Message ID                             | Event Type           | Description                                   |
| -------------------------------------- | -------------------- | --------------------------------------------- |
| `ed4cdb8f-1beb-4ad3-b57c-b3cae2d162fa` | Child Connection     | A Netdata child connects to this parent       |
| `6e2e3839-0676-4896-8b64-6045dbf28d66` | Parent Connection    | This Netdata connects to a parent             |
| `9ce0cb58-ab8b-44df-82c4-bf1ad9ee22de` | Alert Transition     | Alert changes state (CLEAR/WARNING/CRITICAL)  |
| `6db0018e-83e3-4320-ae2a-659d78019fb7` | Alert Notification   | Notification sent to external system          |
| `1e6061a9-fbd4-4501-b3cc-c368119f2b69` | Service Start        | Netdata service started                       |
| `02f47d35-0af5-4491-97bf-7a95b605a468` | Service Stop         | Netdata service stopped                       |
| `23e93dfc-cbf6-4e11-aac8-58b9410d8a82` | Fatal Error          | Critical error requiring attention            |
| `acb33cb9-5778-476b-aac7-02eb7e4e151d` | ACLK Connection      | Netdata Cloud (ACLK) connection state changed |
| `8daf5ba3-3a74-078b-6092-50db1e951f3`  | Sensor State Change  | Hardware sensor state transition              |
| `ec87a561-20d5-431b-ace5-1e2fb8bba243` | Log Flood Protection | Log flooding detected and suppressed          |
| `d1f59606-dd4d-41e3-b217-a0cfcae8e632` | Extreme Cardinality  | Metric cardinality exceeds safe limits        |
| `4fdf4081-6c12-4623-a032-b7fe73beacb8` | User Configuration   | Dynamic configuration changed by user         |

You can view these events using the Netdata systemd-journal.plugin at the `MESSAGE_ID` filter,
or using `journalctl` like this:

```bash
# Query specific event types
journalctl MESSAGE_ID=ed4cdb8f-1beb-4ad3-b57c-b3cae2d162fa  # Child connections
journalctl MESSAGE_ID=9ce0cb58-ab8b-44df-82c4-bf1ad9ee22de  # Alert transitions

# Query multiple event types
journalctl MESSAGE_ID=9ce0cb58-ab8b-44df-82c4-bf1ad9ee22de + MESSAGE_ID=6db0018e-83e3-4320-ae2a-659d78019fb7

# Query with time range
journalctl MESSAGE_ID=9ce0cb58-ab8b-44df-82c4-bf1ad9ee22de --since "1 hour ago"
```

## Platform-Specific Log Access

### Linux: Using journalctl to query Netdata logs

The Netdata service's processes execute within the `netdata` journal namespace. Common queries:

```bash
# Real-time log monitoring
journalctl -u netdata --namespace=netdata -f

# Logs since last restart
journalctl _SYSTEMD_INVOCATION_ID="$(systemctl show --value --property=InvocationID netdata)" --namespace=netdata

# All logs, newest first
journalctl -u netdata --namespace=netdata -r

# Export logs as JSON for processing
journalctl -u netdata --namespace=netdata -o json --since "1 hour ago" > netdata-logs.json

# Filter by severity
journalctl -u netdata --namespace=netdata -p warning  # Warnings and above

# Complex queries with field filters
journalctl -u netdata --namespace=netdata \
    ND_ALERT_STATUS=CRITICAL \
    ND_LOG_SOURCE=health \
    --since "2024-01-01"
```

### Windows: Using Event Viewer to View Netdata Logs

The Netdata service on Windows systems automatically logs events to the Windows Event Viewer. 

#### Accessing logs via GUI:
1. Click the **Start** menu
2. Type **Event Viewer** and select **Run as Administrator**
3. In the Event Viewer window, expand **Applications and Services Logs**
4. Click **Netdata**

The Netdata section contains all available log categories [listed above](#log-sources).

#### Accessing logs via PowerShell:
```powershell
# Get recent Netdata events
Get-WinEvent -LogName "Netdata/Health" -MaxEvents 100

# Filter by severity
Get-WinEvent -FilterHashtable @{LogName="Netdata/Health"; Level=2}  # Errors only

# Export to CSV
Get-WinEvent -LogName "Netdata/Health" | Export-Csv netdata-logs.csv

# Real-time monitoring
Get-WinEvent -LogName "Netdata/Health" -MaxEvents 1 | 
    ForEach-Object { $_ } | 
    Out-GridView -Title "Netdata Health Events"
```

## Using Event Tracing for Windows (ETW)

ETW requires the publisher `Netdata` to be registered. Our Windows installer does this automatically.

Registering the publisher is done via a manifest (`%SystemRoot%\System32\wevt_netdata_manifest.xml`)
and its messages resources DLL (`%SystemRoot%\System32\wevt_netdata.dll`).

If needed, the publisher can be registered and unregistered manually using these commands:

```bat
REM register the Netdata publisher
wevtutil im "%SystemRoot%\System32\wevt_netdata_manifest.xml" "/mf:%SystemRoot%\System32\wevt_netdata.dll" "/rf:%SystemRoot%\System32\wevt_netdata.dll"

REM unregister the Netdata publisher
wevtutil um "%SystemRoot%\System32\wevt_netdata_manifest.xml"
```

The structure of the logs are as follows:

- Publisher `Netdata`
    - Channel `Netdata/Daemon`: general messages about the Netdata service
    - Channel `Netdata/Collector`: general messages about Netdata external plugins
    - Channel `Netdata/Health`: alert transitions and general messages generated by Netdata's health engine
    - Channel `Netdata/Access`: all accesses to Netdata APIs
    - Channel `Netdata/Aclk`: for Cloud connectivity tracing (disabled by default)

Retention can be configured per Channel via the Event Viewer. Netdata does not set a default, so the system default is used.

> **IMPORTANT**<br/>
> Event Tracing for Windows (ETW) does not allow logging the percentage character `%`.
> The `%` followed by a number, is recursively used for fields expansion and ETW has not
> provided any way to escape the character for preventing further expansion.<br/>
> <br/>
> To work around this limitation, Netdata replaces all `%` which are followed by a number, with `℅`
> (the Unicode character `care of`). Visually, they look similar, but when copying IPv6 addresses
> or URLs from the logs, you have to be careful to manually replace `℅` with `%` before using them.

## Using Windows Event Logs (WEL)

WEL has a different logs structure and unfortunately WEL and ETW need to use different names if they are to be used
concurrently.

For WEL, Netdata logs as follows:

- Channel `NetdataWEL` (unfortunately `Netdata` cannot be used, it conflicts with the ETW Publisher name)
    - Publisher `NetdataDaemon`: general messages about the Netdata service
    - Publisher `NetdataCollector`: general messages about Netdata external plugins
    - Publisher `NetdataHealth`: alert transitions and general messages generated by Netdata's health engine
    - Publisher `NetdataAccess`: all accesses to Netdata APIs
    - Publisher `NetdataAclk`: for Cloud connectivity tracing (disabled by default)

Publishers must have unique names system-wide, so we had to prefix them with `Netdata`.

Retention can be configured per Publisher via the Event Viewer or the Registry.
Netdata sets by default 20MiB for all of them, except `NetdataAclk` (5MiB) and `NetdataAccess` (35MiB),
for a total of 100MiB.

For WEL some registry entries are needed. Netdata automatically takes care of them when it starts.

WEL does not have the problem ETW has with the percent character `%`, so Netdata logs it as-is.

## Differences between ETW and WEL

There are key differences between ETW and WEL.

### Publishers and Providers

**Publishers** are collections of ETW Providers. A Publisher is implied by a manifest file,
each of which is considered a Publisher, and each manifest file can define multiple **Providers** in it.
Other than that there is no entity related to **Publishers** in the system.

**Publishers** are not defined for WEL.

**Providers** are the applications or modules logging. Provider names must be unique across the system,
for ETW and WEL together.

To define a **Provider**:

- ETW requires a **Publisher** manifest coupled with resources DLLs and must be registered
  via `wevtutil` (handled by the Netdata Windows installer automatically).
- WEL requires some registry entries and a message resources DLL (handled by Netdata automatically on startup).

The Provider appears as `Source` in the Event Viewer, for both WEL and ETW.

### Channels

- **Channels** for WEL are collections of WEL Providers, (each WEL Provider is a single Stream of logs).
- **Channels** for ETW slice the logs of each Provider into multiple Streams.

WEL Channels cannot have the same name as ETW Providers. This is why Netdata's ETW provider is
called `Netdata`, and WEL channel is called `NetdataWEL`.

Despite the fact that ETW **Publishers** and WEL **Channels** are both collections of Providers,
they are not similar. In ETW a Publisher is a collection on the publisher's Providers, but in WEL
a Channel may include independent WEL Providers (e.g. the "Applications" Channel). Additionally,
WEL Channels cannot include ETW Providers.

### Log Retention

Retention is always defined per Stream.

- Retention in ETW is defined per ETW Channel (ETW Provider Stream).
- Retention in WEL is defined per WEL Provider (each WEL Provider is a single Stream).

### Messages Formatting

- ETW supports recursive fields expansion, and therefore `%N` in fields is expanded recursively
  (or replaced with an error message if expansion fails). Netdata replaces `%N` with `℅N` to stop
  recursive expansion (since `%N` cannot be logged otherwise).
- WEL performs a single field expansion, and therefore the `%` character in fields is never expanded.

### Usability

- ETW names all the fields and allows multiple datatypes per field, enabling log consumers to know
  what each field means and its datatype.
- WEL uses a simple string table for fields, and consumers need to map these string fields based on
  their index.

## Container Logging

Netdata provides seamless integration with container orchestration platforms:

### Docker
```yaml
# docker-compose.yml
services:
  netdata:
    image: netdata/netdata
    environment:
      - NETDATA_LOG_LEVEL=info
    logging:
      driver: json-file
      options:
        max-size: "10m"
        max-file: "3"
```

### Kubernetes
```yaml
# Configure via ConfigMap
apiVersion: v1
kind: ConfigMap
metadata:
  name: netdata-config
data:
  netdata.conf: |
    [logs]
        daemon = stdout
        collector = stdout
        health = stdout
        access = off  # Reduce volume in containers
```

Container logs are automatically collected by the orchestration platform's logging infrastructure (Docker logging drivers, Kubernetes log collectors, etc.).

## SIEM Integration

Netdata's structured logging system is designed for seamless integration with Security Information and Event Management (SIEM) platforms. The combination of structured fields, unique message IDs, and multiple output formats makes Netdata logs ideal for security monitoring and compliance.

### Key Integration Features

1. **Structured Logs** - No parsing required. All events include contextual fields that can be directly mapped to SIEM schemas.

2. **Unique Event Identifiers** - Message IDs (UUIDs) enable precise filtering and correlation across distributed infrastructure.

3. **Multiple Output Options** - Choose the best integration method for your SIEM:
   - **Direct Integration**: Use systemd-journal (Linux) or ETW (Windows) for native collection
   - **File-Based**: JSON or logfmt files for universal compatibility
   - **Network**: Syslog for traditional SIEM collectors

4. **Security-Relevant Events** - Health source provides critical security signals:
   - Alert transitions for threat detection
   - System resource anomalies
   - Service lifecycle events
   - Network connection patterns

5. **Compliance Support** - Full audit trail with:
   - API access logs with user details
   - Configuration change events
   - Alert acknowledgments and notifications
   - Timestamp precision to microseconds

### Integration Approach

1. **Select Log Format** - Configure Netdata to output in your SIEM's preferred format (typically JSON or journal)

2. **Select Relevant Sources** - Enable only the log sources needed for your security use cases:
   - `health` for security alerts
   - `access` for audit trails
   - `daemon` for service integrity

3. **Configure Collection** - Point your SIEM's log collector to:
   - systemd journal with namespace `netdata` (Linux)
   - Event Viewer → Netdata channels (Windows)
   - JSON log files (Universal)

4. **Map Fields** - Netdata's field names are consistent across formats, making it easy to create SIEM parsing rules

5. **Create Detection Rules** - Use Message IDs and structured fields to build detection patterns for:
   - Alert storms (potential DDoS)
   - Service instability (exploitation attempts)
   - Unusual connection patterns (lateral movement)
   - Resource exhaustion (cryptomining)

The structured nature of Netdata logs eliminates the need for complex parsing rules, regex patterns, or custom extractors that are typically required when integrating with SIEMs. This reduces implementation time and ensures reliable event correlation across your security infrastructure.
