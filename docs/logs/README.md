# `journal-viewer` plugin

[KEY FEATURES](#key-features) | [PREREQUISITES](#prerequisites) | [JOURNAL SOURCES](#journal-sources) | [JOURNAL FIELDS](#journal-fields) | [VISUALIZATION](#visualization-capabilities) | [PLAY MODE](#play-mode) | [FULL TEXT SEARCH](#full-text-search) | [QUERY PERFORMANCE](#query-performance) | [PERFORMANCE AT SCALE](#performance-at-scale) | [BEST PRACTICES](#best-practices-for-better-performance) | [CONFIGURATION](#configuration-and-maintenance) | [FAQ](#faq) | [HOW TO TROUBLESHOOT COMMON ISSUES](#how-to-troubleshoot-common-issues) | [HOW TO VERIFY SETUP](#how-to-verify-setup)

The `journal-viewer` plugin provides an efficient way to view, explore, and analyze `systemd` journal logs directly from the Netdata dashboard. It combines powerful filtering, real-time updates, and visual analysis tools to help you troubleshoot system issues effectively.

![Netdata systemd journal plugin interface](https://github.com/netdata/netdata/assets/2662304/691b7470-ec56-430c-8b81-0c9e49012679)

## Key features

- **Unified view of logs** from multiple sources (system, user, namespace, remote)
- **Real-time streaming** with PLAY mode for continuous monitoring
- **Powerful filtering** on all journal fields with counters showing matching entries
- **Full-text search** with wildcard and pattern matching
- **Visual analysis** with interactive histograms showing log frequency
- **Enriched field display** for improved readability (priorities, UIDs, timestamps)
- **High performance** with intelligent sampling for large datasets
- **Zero configuration** for immediate use on supported systems
- **Multi-node support** for centralized log analysis
- **UI-based exploration** without needing to learn complex journalctl syntax
- **Integrated with Netdata's dashboard** for correlation with metrics

## Prerequisites

| Requirement | Details |
|-------------|---------|
| Netdata Agent v1.44+ | This plugin requires Netdata version 1.44 or newer |
| Netdata Cloud account | Required to use Netdata Functions, including this plugin |
| **Not supported in**: | Static builds (use Debian-based containers instead) |

:::tip

This plugin is a Netdata Function Plugin. A free Netdata Cloud account is required. See the [Netdata Functions discussion](https://github.com/netdata/netdata/discussions/16136).

:::

The plugin is designed for native package installations, source installations, and Docker installations (Debian-based). If using Docker, make sure you're using the Debian-based containers.

## Journal sources

The plugin automatically detects available journal sources based on the journal files in `/var/log/journal` (persistent logs) and `/run/log/journal` (volatile logs).

![Journal sources selection interface](https://github.com/netdata/netdata/assets/2662304/28e63a3e-6809-4586-b3b0-80755f340e31)

By default, all sources merge into a unified view of log messages.

:::tip

Select a specific source before analyzing logs in depth to improve query performance.

:::

### System journals

Default journals on all `systemd`-based systems. Includes:

- Kernel log messages (`kmsg`)
- Audit records
- Syslog messages via `systemd-journald`
- Output from service units
- Native journal API messages

### User journals

- Shows journal files for all users, not just the current user
- Each regular user (UID > 999) typically has their own user journal
- Merged into `remote` journals on centralization servers

### Namespace journals

- Isolate log streams per project or service
- Set with `LogNamespace=` in systemd unit files
- Requires special setup to propagate to central servers

### Remote journals

- Created by `systemd-journal-remote`
- Typically named by sender IP, then resolved to hostname

## Journal fields

`systemd` journals support dynamic fields per log entry. All fields and values are indexed for fast querying.

Fields are enriched for readability:

| Field | Enrichment applied |
|-------|-------------------|
| `_BOOT_ID` | Timestamp of first message for boot |
| `PRIORITY` | Human-readable priority name |
| `SYSLOG_FACILITY` | Named facility |
| `ERRNO` | Error name |
| UID and GID fields | Resolved to user and group names |
| `_CAP_EFFECTIVE` | Human-readable capabilities |
| `_SOURCE_REALTIME_TIMESTAMP` | UTC timestamp |
| `MESSAGE_ID` | Well-known event name if known |

:::tip

Enrichments are visual only and not searchable. UID/GID values are based on the system where the plugin runs.

:::

### Fields in the table

Use the ⚙️ icon above the table to select fields to display as columns.

![Table field customization interface](https://github.com/netdata/netdata/assets/2662304/cd75fb55-6821-43d4-a2aa-033792c7f7ac)

The table view provides a powerful way to analyze logs with:
- Scrollable list of log entries with customizable columns
- Color-coded PRIORITY levels for quick identification of issues
- Clickable entries for detailed viewing
- Time-ordered display (newest first by default)
- Pagination controls to navigate large datasets

### Fields in the sidebar

Click a log entry to open the right-hand info panel showing all fields for that entry.

![Sidebar field display](https://github.com/netdata/netdata/assets/2662304/3207794c-a61b-444c-8ffe-6c07cbc90ae2)

The sidebar shows:
- Every field present in the selected journal entry
- Raw and enriched field values
- Copyable text for sharing or further analysis
- Quick filtering options for any field value

### Fields as filters

The plugin offers select fields as filters with counters. Field allowlists and blocklists protect performance.

:::tip

"Full data queries" mode enables negative/empty matches but may slow performance.

:::

![Field filtering interface](https://github.com/netdata/netdata/assets/2662304/ac710d46-07c2-487b-8ce3-e7f767b9ae0f)

Key filter features:
- Real-time counters showing matching entry counts
- Multi-select capability for each field
- Toggleable inclusion/exclusion mode
- Persistent filter selections across page reloads

### Fields as histogram sources

Histograms visualize log frequency per field value over time. Supports:

- Zoom
- Pan
- Click-to-navigate

![Log frequency histogram](https://github.com/netdata/netdata/assets/2662304/d3dcb1d1-daf4-49cf-9663-91b5b3099c2d)

## Visualization capabilities

The plugin offers several visualization features to help you understand and navigate your logs effectively.

### Timeline view

The timeline at the top of the interface shows:
- Log frequency distribution over time
- Interactive zoom and pan controls
- Time selection capabilities
- Anomaly highlighting

### Histograms

Field-specific histograms provide:
- Visual breakdown of log entries by field value
- Color-coded frequency indicators
- Click-to-filter capability
- Time correlation with the main timeline

### Color coding

The plugin uses color to enhance readability:
- Priority levels (emergency, alert, critical, etc.) have distinct colors
- Selected filters are highlighted
- Active elements use consistent color indicators
- Error states and warnings have clear visual differentiation

### UI navigation

The interface offers several ways to navigate logs:
- Scroll through paginated results
- Jump to specific timeframes
- Click on histogram bars to focus on specific values
- Use filter panels to narrow down results
- Toggle between data views

## PLAY mode

The plugin supports PLAY mode for real-time log streaming. Click the ▶️ button at the top of the dashboard to activate it.

- Continuously updates the screen with newly received logs
- Works for both single nodes and centralized log servers

:::tip

**PLAY** mode offers a similar experience to `journalctl -f`, but with visual enhancements.

:::

## Full text search

The plugin supports full-text search using flexible pattern matching:

| Feature | Description |
|---------|-------------|
| Contains match | Default pattern style (e.g., `error` matches `error`, `error_count`) |
| Wildcards | `*` matches any characters (e.g., `a*b` matches `acb`, `a_long_b`) |
| Multiple patterns | Separate with `\|` for OR logic (e.g., `error\|warning` matches lines containing either "error" OR "warning") |
| Negation | Prefix with `!` to exclude (e.g., `!systemd\|*` excludes lines with `systemd`) |

:::tip

Full-text search applies across all fields. Combine with filters for precise results.

:::

## Query performance

The plugin reads journal files directly using `libsystemd`, supporting concurrent readers and one writer.

Query performance depends on several factors:

| Factor | Impact on Performance |
|--------|----------------------|
| Number of journal files queried | Fewer files lead to faster queries |
| Disk speed | Faster disks improve query times |
| Available memory | More memory allows better caching |
| Filters applied | Using fewer filters speeds up the query |

For best performance:
- Keep the visible **timeframe short**
- **Limit the number of rows** displayed
- **Apply filters** to reduce the dataset
- **Use specific sources** instead of querying across all journals

## Performance at scale

The plugin handles large datasets efficiently using a sampling algorithm, ensuring responsive queries even on busy log servers.

### How sampling works

| Step | Description |
|------|-------------|
| 1 | **Fully evaluates the latest 500,000 log entries** |
| 2 | **Distributes evaluation** across journal files for **up to 1 million entries** |
| 3 | **Marks additional entries** as `[unsampled]` beyond the evaluation budget |
| 4 | **Estimates counts** as `[estimated]` once unsampled limits are hit |
| 5 | Uses sequence numbers (if available) for **precise estimation** |
| 6 | **Continues responsive histogram generation** while managing performance |

The plugin uses a sophisticated algorithm that prioritizes newer logs while maintaining reasonable estimates for historical data. This approach ensures that even with terabytes of journal data, the interface remains responsive and usable.

At scale, this plugin achieves up to **25–30x faster query performance** compared to `journalctl`, especially on multi-journal queries.

:::tip

The sampling algorithm is designed to be resilient to large datasets. Even if you see `[unsampled]` or `[estimated]` indicators, the results remain statistically representative of the full dataset.

:::

### Accuracy implications

Netdata's sampling budget evaluates **up to 1,000,000 log entries** before it ever marks rows as `[unsampled]`. The proportion of the dataset we examine is:

```
evaluated_entries = min(total_entries, 1_000_000)
evaluated_ratio   = evaluated_entries / total_entries
```

Because the sampling set is so large, percentage breakdowns stay tight even on massive datasets. For a 10 M–entry window where 60 % of logs share a value, the 95 % confidence interval around that percentage is:

```
standard_error ≈ sqrt(p * (1 - p) / evaluated_entries)
CI95 ≈ 1.96 * standard_error = 1.96 * sqrt(0.6 * 0.4 / 1_000_000) ≈ ±0.9 %
```

By contrast, evaluating only 5,000 entries (a small-sample approach typical of many log explorers when speed is prioritized) would yield:

```
CI95 ≈ 1.96 * sqrt(0.6 * 0.4 / 5_000) ≈ ±8.7 %
```

The result is that even at extreme scale, mainly because Netdata samples 200x more data, it can provide significantly more accurate estimations on value distributions, at comparable performance.

## Best practices for better performance

`systemd-journal` is designed for **reliability first** and **performance second**. It uses deduplication, field linking, and compression to minimize disk footprint, but the structure of journal files can still result in higher disk I/O during queries.

### Filesystem and storage recommendations

| Recommendation | Benefit |
|---------------|---------|
| Use compressed filesystems (`ext4`, `btrfs`, `zfs`) | Reduces disk I/O by minimizing file size |
| Use SSD or NVMe storage | Speeds up journal file reads |
| Avoid small fragmented journal files | Prevents query slowdowns on busy centralization servers |

### Memory and caching

| Recommendation | Benefit |
|---------------|---------|
| Allocate more RAM for the system | Improves OS caching of journal files |
| Query the same timeframe repeatedly | Benefits from cached journal data |
| Limit query timeframes on large datasets | Reduces memory overhead and improves speed |

:::tip

Journal data is cached by the operating system. The more RAM available for caching, the faster your queries will be.

:::

### Query strategies

| Strategy | Why it helps |
|----------|-------------|
| Narrow the timeframe of your queries | Minimizes data scanned per request |
| Use specific filters and source selections | Limits the scope of journal files being queried |
| Limit the number of rows returned in the UI | Keeps response times fast and manageable |
| Enable PLAY mode only when necessary | Reduces continuous query load on the system |

## Configuration and maintenance

The Netdata `systemd` journal plugin is designed to work **out of the box** with minimal configuration.

### Requirements

| Requirement | Description |
|------------|-------------|
| Netdata Agent | Installed on the node or centralization server |
| Journal files | Located in `/var/log/journal` (persistent) or `/run/log/journal` (volatile) |
| Netdata Cloud account | Required to access Netdata Functions, including this plugin |

:::tip

No additional configuration is required for this plugin to operate on supported systems.

:::

### Maintenance considerations

| Task | Purpose |
|------|---------|
| **Keep Netdata up to date** | Ensures plugin compatibility and performance optimizations |
| **Monitor disk usage** of journal files | Prevents performance issues caused by excessive log volume |
| **Verify** journal file **locations** | Confirms the plugin can access the intended sources |
| **Review** source selections **periodically** | Adjusts scope as infrastructure changes |

## FAQ

<details>
<summary><strong>Can I use this plugin on journal centralization servers?</strong></summary>

Yes — you can centralize your logs using `systemd-journal-remote` and install Netdata on the centralization server to explore logs from your entire infrastructure.  
The plugin provides **multi-node views** and allows you to combine logs from multiple servers.

:::tip

For details on configuring a journal centralization server, see the [journal centralization setup guide](/docs/observability-centralization-points/logs-centralization-points-with-systemd-journald/passive-journal-centralization-without-encryption.md).

:::

</details>

<details>
<summary><strong>Can I use this plugin from a parent Netdata node?</strong></summary>

Yes — if your nodes are connected to a Netdata parent, all their functions are accessible via the parent's UI.  
This includes access to the `systemd` journal plugin for each child node.

</details>

<details>
<summary><strong>Does this plugin expose any data to Netdata Cloud?</strong></summary>

No — when accessing the Agent directly, **no data is exposed to Netdata Cloud**.  
The Cloud account is only used for authentication. Data flows directly from your Netdata Agent to your web browser.

:::tip

When using `https://app.netdata.cloud`, communication is encrypted but data is not stored in Netdata Cloud.  
See [this discussion](https://github.com/netdata/netdata/discussions/16136) for more details.

:::

</details>

<details>
<summary><strong>What are `volatile` and `persistent` journals?</strong></summary>

- **Persistent journals** are stored on disk in `/var/log/journal`
- **Volatile journals** are kept in memory in `/run/log/journal` and cleared on reboot

:::tip

For more, check `man systemd-journald`.

:::

</details>

<details>
<summary><strong>I centralize my logs with Loki. Why use Netdata for journals?</strong></summary>

`systemd` journals support **dynamic, high-cardinality labels** with all fields indexed by default.  
When sending logs to Loki, you must predefine which fields to include, reducing flexibility.

Netdata reads journals directly, providing:

| Feature | Netdata journal plugin | Loki |
|---------|----------------------|------|
| **Indexed on all fields** | ✔️ Yes | ❌ Only selected labels |
| Supports **dynamic fields** | ✔️ Yes | ❌ Assumes fixed schema |
| Requires **configuration** | ❌ No | ✔️ Yes (relabel rules, label selection) |

:::tip

Loki and `systemd` journals serve different use cases — they can complement, not replace, each other.

:::

</details>

<details>
<summary><strong>Is it worth setting up a `systemd` logs centralization server?</strong></summary>

Yes — the tools required are included in modern Linux systems, and setup is straightforward.  
Centralized logs provide high visibility with minimal overhead.

</details>

<details>
<summary><strong>How do I configure a journal centralization server?</strong></summary>

Two main strategies:

| Strategy | Description |
|----------|------------|
| **Active sources** | Central server **fetches logs** from each node |
| **Passive sources** | Nodes **push logs** to the central server |

:::tip

See the [passive journal centralization without encryption guide](/docs/observability-centralization-points/logs-centralization-points-with-systemd-journald/passive-journal-centralization-without-encryption.md)  
or the [encrypted setup guide](https://github.com/netdata/netdata/blob/master/docs/observability-centralization-points/logs-centralization-points-with-systemd-journald).

:::

</details>

<details>
<summary><strong>What are the limitations when using centralization?</strong></summary>

| Limitation | Notes |
|------------|-------|
| Namespaces not supported by Docker | [Related issue](https://github.com/moby/moby/issues/41879) |
| `systemd-journal-upload` does not handle namespaces automatically | Requires manual configuration per namespace |

</details>

<details>
<summary><strong>How can I report bugs or request features?</strong></summary>

If you encounter issues or have ideas for improvements:

1. Check the [existing GitHub issues](https://github.com/netdata/netdata/issues) 
2. Submit a new issue with detailed reproduction steps
3. For feature requests, describe your use case clearly

The plugin is actively maintained, and feedback helps improve it for everyone.

</details>

<details>
<summary><strong>Can I customize the plugin's appearance or behavior?</strong></summary>

Currently, customization options are limited to:
- Column selection in the table view
- Filter configurations
- Time range selection
- Source selection

Additional customization features may be added in future releases based on user feedback.

</details>

## How to troubleshoot common issues

Use the following solutions to resolve common issues with the systemd journal plugin.

### How to resolve plugin availability problems

| Possible Cause | Solution |
|---------------|----------|
| Running an older Netdata version (pre-1.44) | Update Netdata to version **1.44 or later** |
| Using Alpine-based Docker container | Use the **Debian-based Netdata container** (from 1.44+) |
| Using a static build of Netdata (`/opt/netdata`) | Switch to a package-based installation or source build |
| Missing `libsystemd` or required dependencies | Make sure `libsystemd` is installed on the host |

### How to fix slow or timing out queries

| Possible Cause | Solution |
|---------------|----------|
| Querying too many journal files at once | Select specific **sources** before running your query |
| Long timeframes selected | Narrow the **timeframe** to improve performance |
| Low disk speed or insufficient RAM | Use faster disks and increase memory for better caching |
| Too many active filters | Reduce the number of **filters** applied |

:::tip

Sampling ensures responsiveness at scale, but selecting sources and filters remains the best way to optimize performance.

:::

### How to solve missing journal sources

| Possible Cause | Solution |
|---------------|----------|
| Journals stored outside default paths | Create a **symlink** to `/var/log/journal` or `/run/log/journal` |
| Journals not persistent across reboots | Configure `systemd-journald` to enable **persistent logs** with `Storage=persistent` in `/etc/systemd/journald.conf` |

### How to address missing or incomplete logs

| Possible Cause | Solution |
|---------------|----------|
| Journals rotated or deleted | Ensure **persistent storage** is enabled |
| Misconfigured journal centralization | Check `systemd-journal-remote` and `systemd-journal-upload` settings |
| Namespace logs not forwarded | Manually configure forwarding for **each namespace** |

### How to fix UI rendering issues

| Possible Cause | Solution |
|---------------|----------|
| Outdated browser | Update to the latest version of Chrome, Firefox, Safari, or Edge |
| Zoom level affecting layout | Reset browser zoom to 100% |
| Ad blockers or script blockers | Temporarily disable to test if they're interfering |
| Network connectivity issues | Check network connections to Netdata server |

### How to understand error messages

| Error Message | Meaning | Solution |
|--------------|---------|----------|
| "Plugin not available" | The journal-viewer plugin isn't loaded | Check your Netdata installation type (must not be Alpine or static) |
| "Unable to open journal" | Permission issues accessing journal files | Ensure Netdata has proper permissions for journal directories |
| "Timeout while querying" | Query is taking too long to complete | Reduce the query scope with filters or shorter timeframes |
| "No sources detected" | Cannot find valid journal files | Check journal file locations and setup |
| "Source selection failed" | Selected source cannot be accessed | Verify the source exists and permissions are correct |

## How to verify setup

### How to check if the plugin is running

```bash
sudo netdata -W plugins
```

Check that the `journal-viewer-plugin` is listed as active.

### How to confirm journal sources are detected

1. Open the **Logs** tab in the Netdata UI
2. Use the **Sources** filter on the right sidebar
3. Ensure you can see your expected sources (e.g., `system`, `user`, `remote`, or specific namespaces)

:::tip

If sources are missing, check the journal file locations and verify symlinks if needed.

:::

### How to test basic queries

- Apply a **simple filter** (like `PRIORITY=3`) and confirm logs are returned
- Use **full-text search** (e.g., search for `error`) and verify results populate correctly
- Toggle **PLAY mode** to confirm live logs are streaming

### How to validate plugin logs

Check the Netdata Agent logs for plugin startup messages:

```bash
sudo journalctl -u netdata | grep journal
```

Look for lines confirming the journal plugin started successfully and detected sources.
