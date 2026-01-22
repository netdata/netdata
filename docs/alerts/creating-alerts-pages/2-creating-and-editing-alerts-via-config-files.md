# 2.2 Creating and Editing Alerts via Configuration Files

This section shows you how to **locate, create, and edit alert definitions** using configuration files on your Netdata Agents or Parents, and how to **reload** them so changes take effect.

:::tip

Use this workflow when you prefer **version control**, need **advanced syntax**, or manage alerts as part of your **infrastructure-as-code** setup.

:::

## 2.2.1 Where Alert Configuration Files Live

Netdata loads alert (health) configuration from two main directories:

| Directory | Purpose | Managed By | Survives Upgrades? |
|-----------|---------|------------|-------------------|
| `/usr/lib/netdata/conf.d/health.d/` | Stock alerts | Netdata packages | **No** (overwritten) |
| `/etc/netdata/health.d/` | Custom alerts (yours) | You | **Yes** |

**Rule of thumb:**
- **Never edit** files in `/usr/lib/netdata/conf.d/health.d/` directly, your changes will be lost on upgrade
- **Always place** your custom alerts in `/etc/netdata/health.d/`

You can organize files inside `/etc/netdata/health.d/` however you like:

```text
/etc/netdata/health.d/
  system.conf         # CPU, RAM, disk, network alerts
  apps.conf           # database, web server alerts
  containers.conf     # Docker, Kubernetes alerts
  my_first_alert.conf # experiments and quick tests
```

:::note

Netdata loads **all** `.conf` files from this directory automatically.

:::

## 2.2.2 Using `edit-config` to Safely Create and Edit Files

Netdata provides an **`edit-config`** helper script to safely create or edit configuration files:

```bash
sudo /etc/netdata/edit-config health.d/<filename>.conf
```

<details>
<summary><strong>Examples</strong></summary>

```bash
# Create or edit a custom alerts file
sudo /etc/netdata/edit-config health.d/system.conf

# Edit the quick-start example from 2.1
sudo /etc/netdata/edit-config health.d/my_first_alert.conf
```

</details>

### What `edit-config` Does

- Opens (or creates) the file in `/etc/netdata/health.d/`
- Uses your default editor (respects `$EDITOR` environment variable)
- Ensures your custom file is **never overwritten** by package upgrades

:::note

You need `sudo` (root privileges) to edit files under `/etc/netdata/` because Netdata configuration files are owned by root.

:::

### If `edit-config` Is Not Available

You can create or edit files directly with your preferred editor:

```bash
sudo nano /etc/netdata/health.d/system.conf
# or
sudo vim /etc/netdata/health.d/system.conf
```

Just ensure the file is under `/etc/netdata/health.d/` and has a `.conf` extension.

## 2.2.3 Adding or Modifying Alert Definitions

Alert definitions use the `template` keyword, which is the recommended and future-proof way to define alerts for any scope:

| Scope | How to Target |
|-------|---------------|
| **All charts of a context** | Use `on: <context>` (for example, `on: disk.space`) |
| **Specific chart instance** | Use `families: <chart-name>` to filter (for example, `families: _` for root filesystem only) |

For the conceptual difference between context-based and chart-specific targeting, see **1.2 Alert Types: `alarm` vs `template`**.

<details>
<summary><strong>Example: Adding an Alert for a Specific Context</strong></summary>

Open or create a file:

```bash
sudo /etc/netdata/edit-config health.d/system.conf
```

Add this alert definition:

```conf
# Alert when any filesystem free space drops below 20%
template: disk_space_usage
    on: disk.space
   lookup: average -1m percentage of avail
     units: %
     every: 1m
      warn: $this < 20
      crit: $this < 10
      info: Filesystem has less than 20% free space
```

**What this does:**
- `template:` gives the alert a unique name
- `on:` attaches it to the `disk.space` context (applies to all filesystems)
- `lookup:` aggregates data over the last minute
- `warn:` and `crit:` define warning and critical thresholds

**To target a specific chart only** (for example, only root filesystem), use `families:`:

```conf
template: root_disk_space
    on: disk.space
 families: _  # Restrict to root filesystem only
   lookup: average -1m percentage of avail
    units: %
    every: 1m
     warn: $this < 20
     crit: $this < 10
```

</details>

:::note

**Chart IDs vs Contexts**

Both use **dots** in their naming convention.

**Contexts** (`on:`): Reference **chart types** — applies to all matching charts
- Example: `on: disk.space` monitors all filesystems
- Example: `on: net.net` monitors all network interfaces

**Chart IDs** (`families:`): Reference **specific chart instances**
- Example: `families: _` targets root filesystem only
- Example: `families: eth0` targets specific network interface

**How to find them:**
- Dashboard: Hover over chart → check tooltip
- API: `curl "http://localhost:19999/api/v1/charts" | jq '.charts[] | {id, context}'`

:::

:::note

Detailed syntax reference for all configuration lines (`lookup`, `calc`, `every`, `warn`, `crit`, etc.) is covered in **Chapter 3: Alert Configuration Syntax**.

:::

## 2.2.4 Finding Chart and Context Names

Before you can write an alert, you need to know **which chart or context** to target.

<details>
<summary><strong>Method 1: From the Local Dashboard</strong></summary>

1. Open your Netdata dashboard (`http://your-node-ip:19999`)
2. Find the chart you want to monitor (for example, `Disk Space` for `/`)
3. Hover over the chart and click the **info icon** or open the chart menu
4. Note the:
   - **Chart ID** (for `on:` in `alarm` rules)
   - **Context** (for `on:` in `template` rules)

</details>

<details>
<summary><strong>Method 2: Using the Charts API</strong></summary>

Query chart metadata via the API:

```bash
curl -s "http://localhost:19999/api/v1/charts" | jq '.charts | keys[]' | head -n 20
```

Or filter by context:

```bash
curl -s "http://localhost:19999/api/v1/charts" | jq '.charts[] | select(.context == "disk.space") | {id, name, context}'
```

Example output:

```json
{
  "id": "disk.space._",
  "name": "disk_space._",
  "context": "disk.space"
}
```

- Use `id` for chart-specific `alarm` rules
- Use `context` for `template` rules

</details>

## 2.2.5 Reloading Health Configuration

After editing alert configuration files, you must **reload** the health configuration for changes to take effect.

### Preferred Method: `netdatacli reload-health`

```bash
sudo netdatacli reload-health
```

If successful, you will see:

```text
Health configuration reloaded
```

This method:
- Reloads health configuration **without restarting** the Netdata daemon
- Is fast (typically under 1 second)
- Preserves existing connections and data collection

### Alternative: Restart Netdata Service

If `netdatacli` is not available or reload fails:

```bash
sudo systemctl restart netdata
```

:::note

Restarting is safe but slower. Use `netdatacli reload-health` when iterating on alert definitions.

:::

## 2.2.6 Verifying Alerts Loaded Correctly

After reloading, verify your alerts are active and error-free.

### Check for Errors in the Log

```bash
sudo tail -n 100 /var/log/netdata/health.log | grep -iE "(health|alarm|template)"
```

Common issues:
- Syntax errors in configuration lines
- Unknown variables or invalid expressions
- Missing charts or contexts

### List Alerts via the API

Check if your alert is present:

```bash
curl -s "http://localhost:19999/api/v1/alarms" | jq '.alarms | keys[]' | grep -i "disk_space_usage"
```

Or view full details for a specific alert:

```bash
curl -s "http://localhost:19999/api/v1/alarms" | jq '.alarms.disk_space_usage'
```

You should see:
- Alert name
- Current status (`CLEAR`, `WARNING`, `CRITICAL`, etc.)
- Chart/context
- Expression details

### Check in the Local Dashboard

1. Open the Netdata dashboard
2. Click the **🔔 bell icon** (top-right)
3. Search for your alert name (for example, `disk_space_usage`)

If the alert is present and your metric is healthy, its status will be `CLEAR`.

## 2.2.7 Modifying Existing Alerts

To modify an alert you've already created:

1. **Open the file** containing the alert:
   ```bash
   sudo /etc/netdata/edit-config health.d/system.conf
   ```

2. **Edit** the alert definition (change thresholds, lookup windows, etc.)

3. **Save** the file

4. **Reload** health configuration:
   ```bash
   sudo netdatacli reload-health
   ```

5. **Verify** the changes took effect via the API or dashboard

## 2.2.8 Customizing Stock Alerts

To modify a **stock alert** without losing your changes on upgrade:

1. **Locate** the stock alert in `/usr/lib/netdata/conf.d/health.d/`
   ```bash
   sudo less /usr/lib/netdata/conf.d/health.d/cpu.conf
   ```

2. **Find and copy** the specific alert block you want to customize (for example, `alarm: 10min_cpu_usage`). You don't need to copy the entire file, just the alert definition you want to change

3. **Paste** it into a file under `/etc/netdata/health.d/`
   ```bash
   sudo /etc/netdata/edit-config health.d/cpu.conf
   ```

4. **Modify** the thresholds, expressions, or other settings

5. **Reload** health configuration

**How precedence works:**
- Stock alerts load first from `/usr/lib/netdata/conf.d/health.d/`
- Custom alerts load next from `/etc/netdata/health.d/`
- If a custom alert has the **same name** as a stock alert, the custom version **overrides** it

:::tip

Only copy and customize the specific alerts you need to change. This keeps your configuration minimal and makes it easier to benefit from improvements in new Netdata versions.

:::

## Key takeaway

File-based alerts give you **maximum control and reproducibility**. Define rules once, keep them in version control, and deploy them across your infrastructure.

## What's Next

- **[2.3 Creating and Editing Alerts via Netdata Cloud](/docs/alerts/creating-alerts-pages/3-creating-and-editing-alerts-via-cloud.md)** Learn the Cloud UI workflow for centralized alert management
- **[2.4 Managing Stock versus Custom Alerts](/docs/alerts/creating-alerts-pages/4-managing-stock-vs-custom-alerts.md)** Patterns for combining stock and custom rules
- **[2.5 Reloading and Validating Alert Configuration](/docs/alerts/creating-alerts-pages/5-reloading-and-validating-alert-configuration.md)** Deeper troubleshooting and validation techniques
- **[Chapter 3: Alert Configuration Syntax](/docs/alerts/alert-configuration-syntax/README.md)** Detailed reference for every configuration line and option