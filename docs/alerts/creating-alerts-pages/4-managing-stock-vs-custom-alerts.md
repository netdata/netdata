# 2.4 Managing Stock versus Custom Alerts

This section explains **how to strategically combine** Netdata's stock alerts with your own custom alerts and Cloud-defined alerts to create a maintainable, effective alerting setup.

You've seen how to create and edit alerts in files (2.2) and in Cloud (2.3). This section focuses on how those sources work together over time.

## 2.4.1 Understanding the Three Alert Sources

Your Netdata deployment can have alerts from three sources:

| Source | Where They Live | Managed By | Use Case |
|--------|----------------|------------|----------|
| **Stock Alerts** | `/usr/lib/netdata/conf.d/health.d/` | Netdata packages (updated on upgrade) | Out-of-the-box coverage for common scenarios |
| **Custom Alerts** | `/etc/netdata/health.d/` | You (survives upgrades) | Environment-specific rules, overrides, and extensions |
| **Cloud-Defined Alerts** | Netdata Cloud (pushed to nodes at runtime) | You (via Cloud UI) | Centralized fleet-wide management |

:::note

All three sources **coexist** on your nodes:
- Stock alerts load first
- Custom file-based alerts load next and can override stock alerts **with the same alert name** (regardless of filename). Additionally, if a user file has the same filename as a stock file, the stock file is skipped entirely.
- Cloud-defined alerts load at runtime and override file-based alerts when they reuse the same `alarm`/`template` name (otherwise they coexist).

:::

## 2.4.2 Key Concepts: Contexts, Instances, Templates, and Alarms

Before diving into patterns, understand how Netdata's alerting model works.

### Contexts vs Instances

Every chart in Netdata has two identifiers:

| Concept | What It Is | Example |
|---------|------------|---------|
| **Context** | What the metrics ARE—their semantic meaning and units | `disk.space` (disk space utilization in %), `system.cpu` (CPU utilization breakdown) |
| **Instance** | A specific occurrence of that metric on your system | `disk_space./`, `disk_space./home`, `system.cpu` (per-node) |

A **context** groups all instances that share the same metric definition. For example, the `disk.space` context includes every mounted filesystem on your system.

```bash
# View all chart contexts
curl -s "http://localhost:19999/api/v1/charts" | jq '.charts[].type' | sort -u

# View instances of a specific context (disks)
curl -s "http://localhost:19999/api/v1/charts" | jq '.charts[] | select(.type == "disk.space") | .id'
```

### Templates vs Alarms

Netdata provides two ways to define alerts that operate at different scopes:

#### Template: Matches a Context

Applies to **ALL instances** of a context automatically. Ideal when you want the same alert logic for every disk, every CPU, every network interface.

```conf
template: disk_space_usage
      on: disk.space          # ← context: matches ALL instances
   lookup: max -1m percentage of avail
     warn: $this < 20
     crit: $this < 10
```

When you define this template, it creates alerts for every disk on every node—no manual configuration per disk needed. When a new disk is mounted, it automatically inherits this alert.

#### Alarm: Matches One Instance

Applies to a **specific instance** only. Used when one particular resource needs different behavior than the norm.

```conf
alarm: disk_space_usage
   on: disk_space._mnt_data     # ← specific chart ID (instance)
   lookup: max -1m percentage of avail
     warn: $this < 5
     crit: $this < 2
```

Use alarms for exceptions—a data disk that runs fuller than others, a specific service with unique thresholds.

**Key distinction:** The `on:` line specifies a **context** for templates, but a **chart ID** (instance) for alarms.

### Precedence: Alarms Beat Templates (Same Name Required)

Both a template and an alarm can coexist on the same context—their interaction follows clear rules:

1. Precedence only applies when alerts have the **same name**
2. Different names coexist independently on the same instance
3. When names match: **Alarms win over templates**, **User config wins over stock**

```conf
# Stock template (applies to ALL disks, warn at 20%)
template: disk_space_usage
      on: disk.space
   lookup: max -1m percentage of avail
     warn: $this < 20
     crit: $this < 10

# User alarm (applies to ONE specific disk, warn at 5%)
alarm: disk_space_usage        # same name = precedence applies
   on: disk_space._mnt_data    # specific instance
   lookup: max -1m percentage of avail
     warn: $this < 5
     crit: $this < 2
```

Result:
- `/mnt/data` uses your alarm's 5%/2% thresholds
- All other disks use the template's 20%/10% thresholds

### Complete Precedence Order

| Priority | Type | Source | Behavior |
|----------|------|--------|----------|
| 1 (highest) | Alarm | User config | Processed first for matching instance |
| 2 | Alarm | Stock config | Falls through if user alarm doesn't match |
| 3 | Template | User config | Applied if no alarm matched |
| 4 (lowest) | Template | Stock config | Final fallback for unmatched instances |

This ordering is **first-match-wins** within each precedence level for the same alert name.

### File Shadowing

If a file with the **same filename** exists in both user and stock directories, **only the user file is loaded**. The stock file is completely ignored:

| User Config | Stock Config | Result |
|-------------|-------------|--------|
| `/etc/netdata/health.d/cpu.conf` | `/usr/lib/netdata/conf.d/health.d/cpu.conf` | Only user file loads |

Unlike alert-level precedence, file shadowing skips the entire stock file—you must include ALL alerts you want from that file.

### Dynamic Configuration Exception

Alerts edited through the **Netdata UI or API** behave differently:

- **UI/API overrides replace** any file-based definition with the same name
- This is the only case where a definition completely overwrites another

Editing an alert in the dashboard creates a dynamic configuration that takes precedence over all file-based configs. This allows quick adjustments without SSH access. To restore file-based control, remove the dynamic config through the UI or API.

<details>
<summary><strong>Pattern 1: Start with Stock, Add Custom for Special Cases</strong></summary>

**Strategy:**
- Use stock alerts as your **baseline** (they cover 80% of common monitoring needs)
- Add **custom file-based alerts** in `/etc/netdata/health.d/` for:
  - Metrics not covered by stock alerts
  - Business-specific thresholds (your SLAs differ from defaults)
  - Application-specific monitoring (custom apps, internal services)

**Example:**
```
Stock alerts handle:
  ✓ System CPU, memory, disk, network
  ✓ Common databases (MySQL, PostgreSQL, Redis)
  ✓ Web servers (Apache, Nginx)

Custom alerts handle:
  ✓ Your internal microservice health endpoints
  ✓ Custom application metrics
  ✓ Business KPIs (orders/sec, conversion rates)
```

**Why this works:**
- Minimal maintenance (stock alerts auto-update)
- Custom alerts stay focused on what's unique to your environment
- Clear separation of concerns

</details>

<details>
<summary><strong>Pattern 2: Override Stock Alerts with Custom Thresholds</strong></summary>

**Strategy:**
- Keep most stock alerts as-is
- **Override specific stock alerts** by copying them to `/etc/netdata/health.d/` and modifying thresholds

**When to use:**
- Stock alert thresholds don't match your environment (you run high CPU workloads normally)
- You need different warning/critical levels for specific nodes or contexts

**How to override safely:**

1. **Find the stock alert** you want to customize (for example, `20min_steal_cpu`):
   ```bash
   sudo grep -n "20min_steal_cpu" /usr/lib/netdata/conf.d/health.d/*
   ```

2. **Copy the alert definition** to your custom config:
   ```bash
   sudo /etc/netdata/edit-config health.d/my-overrides.conf
   ```

3. **Paste and modify** the alert (keep the same `template:` name):
   ```conf
   # Override stock 20min_steal_cpu with lower thresholds for our workload
   template: 20min_steal_cpu
       on: system.cpu
   lookup: average -20m unaligned of steal
    units: %
    every: 5m
     warn: $this > (($status >= $WARNING) ? (3) : (5))
     crit: $this > (($status >= $WARNING) ? (5) : (8))
     info: CPU steal time over 20 minutes (custom thresholds)
       to: ops_team
   ```

4. **Reload health configuration:**
   ```bash
   sudo netdatacli reload-health
   ```

**What happens:**
- Your custom version **overrides** the stock alert (same name = precedence)
- Only `20min_steal_cpu` is affected—all other alerts in stock files remain unchanged
- Future Netdata upgrades won't affect your override
- You still benefit from stock alert updates for alerts you **didn't** override

:::tip

**Only override what you need to change.** Don't copy entire stock files, just the specific alerts you want to customize. This keeps your custom configuration minimal and maintainable.

:::

</details>

<details>
<summary><strong>Pattern 3: Fleet-Wide Standards via Cloud, Node-Specific via Files</strong></summary>

**Strategy:**
- Use **Cloud-defined alerts** for:
  - Standard monitoring across all nodes (all production nodes should alert on disk > 90%)
  - Policies that change frequently (easy to update centrally)
- Use **file-based alerts** for:
  - Node-specific exceptions (database servers have different memory thresholds)
  - Advanced syntax not yet available in Cloud UI
  - Alerts that must work without Cloud connectivity

**Example architecture:**
```
Cloud-defined alerts:
  ✓ Standard disk space thresholds (all nodes)
  ✓ Network traffic baselines (all nodes)
  ✓ Container resource limits (Kubernetes nodes)

File-based custom alerts:
  ✓ Database server: custom query latency thresholds
  ✓ Cache servers: custom eviction rate alerts
  ✓ Edge nodes: offline-capable alerts
```

:::note

**Important:** Cloud-defined and file-based alerts use different identifiers, so they **don't automatically override each other**. If you want to "replace" a file-based alert with a Cloud one, you must explicitly disable or remove the file-based version.

:::

</details>

<details>
<summary><strong>Pattern 4: Disable Stock Alerts You Don't Need</strong></summary>

**Strategy:**
- If a stock alert is **not relevant** to your environment, disable it rather than letting it generate noise

**How to disable a stock alert:**

**Method 1: Disable via custom config**
```bash
sudo /etc/netdata/edit-config health.d/disabled_alerts.conf
```

Add:
```conf
# Disable stock alert that doesn't apply to our environment
alarm: mysql_10s_slow_queries
   to: silent
```

Setting `to: silent` prevents notifications without stopping evaluation.

**Method 2: Disable evaluation entirely**
```conf
# Completely disable alert evaluation
alarm: disk_space_usage
enabled: no
```

**Method 3: Disable via Cloud silencing rules**
- Use **silencing rules** in Netdata Cloud to suppress notifications space-wide (see **4. Controlling Alerts and Noise**)

:::warning

**Don't delete stock alert files** from `/usr/lib/netdata/conf.d/health.d/`, they'll be restored on upgrade. Always disable via custom config in `/etc/netdata/health.d/`.

:::

</details>

## 2.4.3 Migration Strategies

<details>
<summary><strong>Migrating from File-Only to Cloud-First</strong></summary>

If you're transitioning from a file-based setup to Cloud-managed alerts:

**Phase 1: Audit**
1. List all custom alerts in `/etc/netdata/health.d/`
2. Categorize:
   - **Fleet-wide standards** → candidates for Cloud
   - **Node-specific exceptions** → keep as files
   - **Advanced syntax** → keep as files (until Cloud UI supports them)

**Phase 2: Migrate Fleet-Wide Alerts**
1. Recreate fleet-wide alerts in Cloud UI
2. Test on a subset of nodes
3. Once verified, disable the file-based versions:
   ```conf
   # Migrated to Cloud, disabled locally
    alarm: disk_space_usage
   enabled: no
   ```

**Phase 3: Keep Node-Specific Alerts as Files**
- Leave node-specific customizations in `/etc/netdata/health.d/`
- Document which alerts are Cloud-managed vs file-managed

**Result:**
- Centralized management for common cases
- File-based flexibility for exceptions
- Clear separation of responsibilities

</details>

<details>
<summary><strong>Migrating from Cloud to Files (Air-Gapped Environments)</strong></summary>

If you need to move **away** from Cloud (for air-gapped deployments):

1. **Export alert definitions** from Cloud (manually document or screenshot)
2. **Recreate as file-based alerts** in `/etc/netdata/health.d/`
3. **Test** on one node before deploying fleet-wide
4. **Deploy** via configuration management (Ansible, Terraform, etc.)
5. **Disable Cloud alerts** (delete from Cloud UI or disconnect nodes)

</details>

## 2.4.4 Best Practices for Mixed Environments

<details>
<summary><strong>1. Document Your Alert Sources</strong></summary>

Maintain a simple inventory:

```markdown
# Alert Inventory

## Stock Alerts
- Location: `/usr/lib/netdata/conf.d/health.d/`
- Status: Active (default thresholds)
- Overrides: See `/etc/netdata/health.d/cpu_custom.conf`

## Custom File-Based Alerts
- Location: `/etc/netdata/health.d/`
- Purpose: App-specific metrics, custom thresholds
- Deployment: Via Ansible playbook `deploy-alerts.yml`

## Cloud-Defined Alerts
- Scope: All production nodes
- Purpose: Fleet-wide standards (disk, memory, network)
- Managed by: SRE team via Cloud UI
```

</details>

<details>
<summary><strong>2. Use Naming Conventions</strong></summary>

Adopt consistent naming to identify alert sources:

```conf
# Stock alerts: keep original names
template: 10min_cpu_usage

# Custom overrides: add suffix
template: 10min_cpu_usage_custom

# Cloud alerts: use descriptive names
alert_name: prod_disk_space_standard
```

</details>

<details>
<summary><strong>3. Version Control Custom Alerts</strong></summary>

Keep `/etc/netdata/health.d/` in Git:
```bash
cd /etc/netdata/health.d
git init
git add *.conf
git commit -m "Initial alert configuration"
```

This gives you:
- Change history
- Rollback capability
- Code review workflow
- Easy deployment across nodes

</details>

<details>
<summary><strong>4. Test Before Deploying</strong></summary>

Always test alert changes on a **non-production node** first:
1. Apply the change
2. Reload health configuration
3. Verify the alert loads correctly
4. Trigger the alert (if possible) to confirm behavior
5. Deploy to production

</details>

<details>
<summary><strong>5. Periodic Review</strong></summary>

Schedule quarterly reviews:
- Are stock alerts still relevant?
- Can any custom alerts be replaced by improved stock alerts?
- Are Cloud-defined alerts still aligned with current policies?
- Remove obsolete alerts

</details>

## 2.4.5 Troubleshooting Mixed Alert Sources

<details>
<summary><strong>I don't know which alert is firing</strong></summary>

**Solution:** Check the alert source in the dashboard or API:

```bash
curl -s "http://localhost:19999/api/v1/alarms" | jq '.alarms["alert_name"]'
```

Look for:
- `chart`: tells you which chart it's attached to
- `source`: may indicate file path (file-based) or Cloud origin

</details>

<details>
<summary><strong>My custom override isn't working</strong></summary>

**Step-by-step diagnosis:**

1. **Confirm reload worked:**
   ```bash
   sudo netdatacli reload-health
   ```

2. **Check which definition is active:**
   ```bash
   curl -s "http://localhost:19999/api/v1/alarms?all" | \
     jq '.alarms | to_entries[] | select(.value.name == "YOUR_ALERT_NAME") | .value.source'
   ```

3. **Look for errors in logs:**
   ```bash
   # systemd-based systems (most modern Linux):
   journalctl --namespace netdata -g health --no-pager | tail -30

   # Fallback to log files:
   grep -i health /var/log/netdata/error.log | tail -30
   ```

4. **Verify file permissions:**
   ```bash
   ls -la /etc/netdata/health.d/your-config.conf
   # Should be readable by netdata user
   ```

5. **Validate syntax:**
   ```bash
   cat /etc/netdata/health.d/your-config.conf | python3 -c "import json, yaml, sys; yaml.safe_load(sys.stdin)"
   # Basic YAML validation (doesn't catch all Netdata-specific errors)
   ```

</details>

<details>
<summary><strong>Cloud alert and file-based alert both firing</strong></summary>

**This is expected behavior.** Cloud and file-based alerts **coexist** (they use different identifiers).

**Solution:**
- If you want only one, **disable** the other:
  - File-based: set `enabled: no` in the custom config
  - Cloud-based: delete from Cloud UI or use silencing rules

</details>

## Key takeaway

The three alert sources are **complementary, not competing**. Use stock alerts as your foundation, extend with custom alerts for your specific needs, and leverage Cloud for centralized management, all in the same deployment.

## What's Next

- **[2.5 Reloading and Validating Alert Configuration](/docs/alerts/creating-alerts-pages/5-reloading-and-validating-alert-configuration.md)** Deeper troubleshooting and validation techniques
- **[3. Alert Configuration Syntax](/docs/alerts/alert-configuration-syntax/README.md)** Full syntax reference for writing alert definitions
- **[4. Controlling Alerts and Noise](/docs/alerts/controlling-alerts-noise/README.md)** Silencing rules, delays, and hysteresis