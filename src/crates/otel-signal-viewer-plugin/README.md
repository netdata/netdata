# OpenTelemetry Signal Viewer (otel-signal-viewer.plugin)

The `otel-signal-viewer` plugin is a [Netdata](https://github.com/netdata/netdata) external plugin
that reads, indexes, and visualizes OpenTelemetry logs stored in
`systemd`-compatible [journal files](https://systemd.io/JOURNAL_FILE_FORMAT/)
written by the [`otel`](../../netdata-otel/otel-plugin/README.md) plugin.

> These journal files can also be inspected directly with
> [`journalctl`](https://www.freedesktop.org/software/systemd/man/latest/journalctl.html).
> Note that the output will contain logs with remapped field names for
> `systemd`-compatibility.

## Configuration

Edit the [otel-signal-viewer.yaml](https://github.com/netdata/netdata/blob/master/src/crates/netdata-log-viewer/otel-signal-viewer-plugin/configs/otel-signal-viewer.yaml.in)
configuration file using `edit-config` from the Netdata
[config directory](/docs/netdata-agent/configuration/README.md#locate-your-config-directory),
which is typically located under `/etc/netdata`.

```bash
cd /etc/netdata # Replace this path with your Netdata config directory
sudo ./edit-config otel-signal-viewer.yaml
```

### Journal

The `journal` section specifies the directories containing journal files to
watch and index. By default, it points to the same directory the `otel`
plugin uses to store OpenTelemetry logs. At least one path must be specified:

```yaml
journal:
  paths:
    - "/var/log/netdata/otel/v1"
```

### Cache

The `cache` section configures the in-memory LRU cache used for indexed journal
data.

| Option             | Default                           | Description                                          |
|--------------------|-----------------------------------|------------------------------------------------------|
| `memory_capacity`  | `1000`                            | Number of indexed journal files to keep in memory    |
| `workers`          | number of CPU cores               | Number of background workers for indexing             |
| `queue_capacity`   | `100`                             | Queue capacity for pending indexing requests          |

```yaml
cache:
  memory_capacity: 1000
  # workers: <number-of-CPU-cores>
  queue_capacity: 100
```

### Indexing

The `indexing` section controls how journal fields are indexed, with limits to
prevent unbounded memory growth from high-cardinality fields.

| Option                         | Default | Description                                                  |
|--------------------------------|---------|--------------------------------------------------------------|
| `max_unique_values_per_field`  | `500`   | Maximum unique values to index per field                     |
| `max_field_payload_size`       | `100`   | Maximum payload size (in bytes) for field values to index    |

```yaml
indexing:
  max_unique_values_per_field: 500
  max_field_payload_size: 100
```
