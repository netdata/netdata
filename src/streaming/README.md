# Streaming and Replication Reference

This guide covers advanced streaming options and recommended deployment strategies for production environments. If you're new to Netdata streaming, start with the [quick introduction to streaming](/docs/observability-centralization-points/README.md) to set up a basic parent-child configuration.

## Configuration Overview

Netdata's streaming capabilities are configured through two key files:

- **`stream.conf`** – Controls streaming behavior, including parent and child configurations.
- **`netdata.conf`** – Contains global settings that can impact streaming.

To edit these files, navigate to your Netdata configuration directory (typically `/etc/netdata`) and run:

```sh
sudo ./edit-config stream.conf
sudo ./edit-config netdata.conf
```

## Configuring `stream.conf`

The `stream.conf` file has three main sections:

- **`[stream]`** – Configures child nodes.
- **`[API_KEY]`** – Defines settings for all child nodes using the same API key.
- **`[MACHINE_GUID]`** – Sets configurations for a specific child node matching the given GUID.

### Identifying a Node's GUID

Each Netdata node has a unique identifier stored in:

```sh
/var/lib/netdata/registry/netdata.public.unique.id
```

This file is generated automatically the first time Netdata starts and remains unchanged.

## Recommended Deployment Strategies

For a production-ready streaming setup, consider the following best practices:

- **Use Multiple Parent Nodes** – Ensures redundancy and improves resilience.
- **Optimize Data Retention** – Configure retention periods to balance storage costs and data availability.
- **Secure Communications** – Enable encryption and authentication to protect data streams.
- **Monitor Performance** – Regularly review logs and metrics to ensure efficient streaming operations.

By following these guidelines, you can set up a scalable and reliable Netdata streaming environment.

```markdown
# Streaming and Replication Configuration

This guide covers advanced streaming options and recommended deployment settings for production use.  
If you’re new to streaming, start with the [Quick Introduction to Streaming](/docs/observability-centralization-points/README.md) for a basic parent-child setup.

## Configuration Files

Netdata’s streaming settings are managed in two files:

- **`stream.conf`** – Configures how Netdata nodes send and receive metrics.
- **`netdata.conf`** – Defines general settings, including web access and API permissions.

To edit these files, use the following commands from Netdata’s configuration directory (`/etc/netdata`):

```sh
sudo ./edit-config stream.conf
sudo ./edit-config netdata.conf
```

## `stream.conf`

The `stream.conf` file consists of three main sections:

1. **`[stream]`** – Configures child nodes (data senders).
2. **`[API_KEY]`** – Defines API keys for parent nodes (data receivers).
3. **`[MACHINE_GUID]`** – Customizes settings for specific child nodes.

### `[stream]` Section (Child Node Settings)

This section configures a child node to send metrics to a parent.

| Setting                                         | Default                   | Description                                                         |
|-------------------------------------------------|---------------------------|---------------------------------------------------------------------|
| `enabled`                                       | `no`                      | Enables streaming. Set to `yes` to allow this node to send metrics. |
| [`destination`](#destination)                   | (empty)                   | Defines one or more parent nodes to send data to.                   |
| `ssl skip certificate verification`             | `yes`                     | Accepts self-signed or expired SSL certificates.                    |
| `CApath`                                        | `/etc/ssl/certs/`         | Directory for trusted SSL certificates.                             |
| `CAfile`                                        | `/etc/ssl/certs/cert.pem` | File containing trusted certificates.                               |
| `api key`                                       | (empty)                   | API key used by the child to authenticate with the parent.          |
| `timeout`                                       | `1m`                      | Connection timeout duration.                                        |
| `default port`                                  | `19999`                   | Default port for streaming if not specified in `destination`.       |
| [`send charts matching`](#send-charts-matching) | `*`                       | Filters which charts are streamed.                                  |
| `buffer size bytes`                             | `10485760`                | Buffer size (10MB by default). Increase for higher latencies.       |
| `reconnect delay`                               | `5s`                      | Time before retrying connection to the parent.                      |
| `initial clock resync iterations`               | `60`                      | Syncs chart clocks during startup.                                  |
| `parent using h2o`                              | `no`                      | Set to `yes` if connecting to a parent using the H2O web server.    |

### `[API_KEY]` Section (Parent Node Authentication)

This section allows parent nodes to accept streaming data from child nodes using an API key.

| Setting                      | Default    | Description                                                 |
|------------------------------|------------|-------------------------------------------------------------|
| `enabled`                    | `no`       | Enables or disables this API key.                           |
| `type`                       | `api`      | Defines the section as an API key configuration.            |
| [`allow from`](#allow-from)  | `*`        | Specifies which child nodes (IP addresses) can connect.     |
| `retention`                  | `1h`       | How long to keep child node metrics in RAM-based storage.   |
| [`db`](#db)                  | `dbengine` | Specifies the database type for this API key.               |
| `health enabled`             | `auto`     | Controls alerts and notifications (`auto`, `yes`, or `no`). |
| `postpone alerts on connect` | `1m`       | Delay alerts for a period after the child connects.         |
| `health log retention`       | `5d`       | Duration (in seconds) to keep health log events.            |
| `proxy enabled`              | (empty)    | Enables routing metrics through a proxy.                    |
| `proxy destination`          | (empty)    | IP and port of the proxy server.                            |
| `proxy api key`              | (empty)    | API key for the proxy server.                               |
| `send charts matching`       | `*`        | Defines which charts to stream.                             |
| `enable compression`         | `yes`      | Enables or disables data compression.                       |
| `enable replication`         | `yes`      | Enables or disables data replication.                       |
| `replication period`         | `1d`       | Maximum time window replicated from each child.             |
| `replication step`           | `10m`      | Time interval for each replication step.                    |
| `is ephemeral node`          | `no`       | Marks the child as ephemeral (removes it after inactivity). |

### `[MACHINE_GUID]` Section (Per-Node Customization)

This section customizes settings for specific child nodes using their unique Machine GUID.

| Setting                      | Default    | Description                                              |
|------------------------------|------------|----------------------------------------------------------|
| `enabled`                    | `no`       | Enables or disables this specific node’s configuration.  |
| `type`                       | `machine`  | Defines the section as a machine-specific configuration. |
| [`allow from`](#allow-from)  | `*`        | Lists IP addresses allowed to stream metrics.            |
| `retention`                  | `3600`     | Retention period for child metrics in RAM-based storage. |
| [`db`](#db)                  | `dbengine` | Database type for this node.                             |
| `health enabled`             | `auto`     | Controls alerts (`auto`, `yes`, `no`).                   |
| `postpone alerts on connect` | `1m`       | Delay alerts for a period after connection.              |
| `health log retention`       | `5d`       | Duration to keep health log events.                      |
| `proxy enabled`              | (empty)    | Routes metrics through a proxy if enabled.               |
| `proxy destination`          | (empty)    | Proxy server IP and port.                                |
| `proxy api key`              | (empty)    | API key for the proxy.                                   |
| `send charts matching`       | `*`        | Filters streamed charts.                                 |
| `enable compression`         | `yes`      | Enables or disables compression.                         |
| `enable replication`         | `yes`      | Enables or disables replication.                         |
| `replication period`         | `1d`       | Maximum replication window.                              |
| `replication step`           | `10m`      | Time interval for each replication step.                 |
| `is ephemeral node`          | `no`       | Marks the node as ephemeral (removes after inactivity).  |

## Additional Settings

### `destination`

Defines parent nodes for streaming using the format:  
`[PROTOCOL:]HOST[%INTERFACE][:PORT][:SSL]`

- **PROTOCOL**: `tcp`, `udp`, or `unix` (only `tcp` and `unix` are supported for parents).
- **HOST**: IPv4, IPv6 (in brackets `[ ]`), hostname, or Unix domain socket path.
- **INTERFACE** (IPv6 only): Network interface to use.
- **PORT**: Port number or service name.
- **SSL**: Enables TLS/SSL encryption.

Example (TCP connection with SSL to `203.0.113.0` on port `20000`):

```ini
[stream]
    destination = tcp:203.0.113.0:20000:SSL
```

### `send charts matching`

Controls which charts are streamed.

- `*` (default) – Streams all charts.
- Specific charts:

  ```ini
  [stream]
      send charts matching = apps.cpu system.*
  ```

- Exclude charts using `!`:

  ```ini
  [stream]
      send charts matching = !apps.cpu *
  ```

### `allow from`

Defines which child nodes (by IP) can connect.

- Allow a single IP:

  ```ini
  [API_KEY]
      allow from = 203.0.113.10
  ```

- Allow a range but exclude one:

  ```ini
  [API_KEY]
      allow from = !10.1.2.3 10.*
  ```

### `db`

Defines the database mode:

- `dbengine` – Stores recent metrics in RAM and writes older data to disk.
- `ram` – Stores metrics only in RAM (lost on restart).
- `none` – No database.

```ini
[API_KEY]
    db = dbengine
```

Here’s an optimized version of the `netdata.conf` structure with clearer, more direct language. I've broken down the key sections for readability and made the content less dense:

### netdata.conf

The `netdata.conf` file is the primary configuration file for the Netdata agent. It controls the agent’s settings, including networking, data collection, logging, and resource usage.

### Sections

#### [global]

This section defines global settings for the Netdata agent.

- **hostname**: The hostname used by the agent.
- **memory mode**: Choose the memory mode for data collection (e.g., `ram` or `swap`).
- **error log file**: Path to the file where error logs are saved.

#### [web]

Configure the web interface settings here.

- **bind to**: Define the network address to which Netdata binds.
- **port**: Set the port for the web interface (default: 19999).
- **disable SSL**: Set to `yes` to disable SSL support.

#### [plugin]

This section configures individual plugins for data collection.

- **enabled**: Enable or disable the plugin.
- **update every**: Define the update frequency (in seconds).

#### [database]

Manage database settings for data storage and retention.

- **memory mode**: Choose between in-memory or disk-based storage.
- **data retention**: Set how long to keep historical data.
- **compression**: Enable or disable data compression.

#### [logging]

Configure logging behavior for Netdata.

- **log file**: Define the log file location.
- **log level**: Set the verbosity of logs (e.g., `info`, `debug`).

## Troubleshooting

Both parent and child nodes log information in `/var/log/netdata/error.log`.

If the child successfully connects to the parent, you’ll see logs similar to the following on the parent:

```
2017-03-09 09:38:52: netdata: INFO : STREAM [receive from [10.11.12.86]:38564]: new client connection.
2017-03-09 09:38:52: netdata: INFO : STREAM xxx [10.11.12.86]:38564: receive thread created (task id 27721)
2017-03-09 09:38:52: netdata: INFO : STREAM xxx [receive from [10.11.12.86]:38564]: client willing to stream metrics for host 'xxx' with machine_guid '1234567-1976-11e6-ae19-7cdd9077342a': update every = 1, history = 3600, memory mode = ram, health auto
2017-03-09 09:38:52: netdata: INFO : STREAM xxx [receive from [10.11.12.86]:38564]: initializing communication...
2017-03-09 09:38:52: netdata: INFO : STREAM xxx [receive from [10.11.12.86]:38564]: receiving metrics...
```

On the child side, you might see:

```
2017-03-09 09:38:28: netdata: INFO : STREAM xxx [send to box:19999]: connecting...
2017-03-09 09:38:28: netdata: INFO : STREAM xxx [send to box:19999]: initializing communication...
2017-03-09 09:38:28: netdata: INFO : STREAM xxx [send to box:19999]: waiting response from remote netdata...
2017-03-09 09:38:28: netdata: INFO : STREAM xxx [send to box:19999]: established communication - sending metrics...
```

The following sections cover common issues when connecting parent and child nodes.

### Slow Connections Between Parent and Child

Slow connections may lead to several errors, mainly logged in the child’s `error.log`:

```bash
netdata ERROR : STREAM_SENDER[CHILD HOSTNAME] : STREAM CHILD HOSTNAME [send to PARENT IP:PARENT PORT]: too many data pending - buffer is X bytes long, Y unsent - we have sent Z bytes in total, W on this connection. Closing connection to flush the data.
```

On the parent side, you might see:

```
netdata ERROR : STREAM_PARENT[CHILD HOSTNAME,[CHILD IP]:CHILD PORT] : read failed: end of file
```

Another issue in slow connections is the child sending partial messages to the parent. In this case, the parent will log:

```
ERROR : STREAM_RECEIVER[CHILD HOSTNAME,[CHILD IP]:CHILD PORT] : sent command 'B' which is not known by netdata, for host 'HOSTNAME'. Disabling it.
```

Slow connections can also cause the parent to miss a message. For example, if the parent misses a message about the child’s charts and then receives a `SET` command for a chart, the parent might log:

```
ERROR : STREAM_RECEIVER[CHILD HOSTNAME,[CHILD IP]:CHILD PORT] : requested a SET on chart 'CHART NAME' of host 'HOSTNAME', without a dimension. Disabling it.
```

### Child Cannot Connect to Parent

If the child can't connect to the parent (due to misconfiguration, networking issues, firewalls, or the parent being down), the child will log:

```
ERROR : STREAM_SENDER[HOSTNAME] : Failed to connect to 'PARENT IP', port 'PARENT PORT' (errno 113, No route to host)
```

### 'Is This a Netdata?'

This error typically occurs when the parent is using SSL and the child attempts a plain-text connection, or if the child tries to connect to a non-Netdata server. The error message looks like this:

```
ERROR : STREAM_SENDER[CHILD HOSTNAME] : STREAM child HOSTNAME [send to PARENT HOSTNAME:PARENT PORT]: server is not replying properly (is it a netdata?).
```

### Stream Charts Wrong

If chart data is inconsistent between the parent and child (e.g., gaps in metrics collection), it likely indicates a mismatch in the `[db].db` settings between the parent and child. Refer to our [db documentation](/src/database/README.md) for more information on how Netdata stores metrics to ensure data consistency.

### Forbidding Access

Access might be forbidden for several reasons, such as slow connections or other failures. Look for the following errors in the parent’s `error.log`:

```
STREAM [receive from [child HOSTNAME]:child IP]: `MESSAGE`. Forbidding access."
```

Possible causes for this error include:

- `request without KEY`: Incomplete message, missing API key, hostname, or machine GUID.
- `API key 'VALUE' is not valid GUID`: Invalid UUID format.
- `machine GUID 'VALUE' is not GUID.`: Invalid machine GUID.
- `API key 'VALUE' is not allowed`: Invalid API key.
- `API key 'VALUE' is not permitted from this IP`: IP not allowed to use STREAM with this parent.
- `machine GUID 'VALUE' is not allowed.`: GUID not permitted.
- `Machine GUID 'VALUE' is not permitted from this IP.`: IP not matching the allowed pattern.

### Netdata Could Not Create a Stream

If the parent cannot convert the initial connection into a stream, it will log the following error:

```
file descriptor given is not a valid stream
```

After logging this error, Netdata will close the stream.
