# 2.4 Managing Stock versus Custom Alerts

This section explains **how to strategically combine** Netdata's built-in (stock) alerts with your own custom alerts and Cloud-defined alerts to create a maintainable, effective alerting setup.

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
- Cloud-defined alerts load at runtime (and use separate identifiers, so they don't automatically override file-based ones)

:::

## 2.4.2 Common Management Patterns

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

1. **Find the stock alert** you want to customize:
   ```bash
   sudo less /usr/lib/netdata/conf.d/health.d/cpu.conf
   ```

2. **Copy just the alert definition** you want to change:
   ```bash
   sudo /etc/netdata/edit-config health.d/cpu_custom.conf
   ```

3. **Paste and modify** the alert (keep the same `alarm:` or `template:` name):
   ```conf
   # Override stock CPU alert with higher threshold for our workload
   template: 10min_cpu_usage
       on: system.cpu
   lookup: average -10m unaligned of user,system,softirq,irq,guest
    units: %
    every: 1m
     warn: $this > 90   # Stock default is 80
     crit: $this > 98   # Stock default is 95
     info: CPU utilization for the last 10 minutes (custom threshold)
       to: sysadmin
   ```

4. **Reload health configuration:**
   ```bash
   sudo netdatacli reload-health
   ```

**What happens:**
- Your custom version **overrides** the stock alert (same name = precedence)
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
- Use **silencing rules** in Netdata Cloud to suppress notifications space-wide (see **Chapter 4: Controlling Alerts and Noise**)

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

**Check:**
1. **Alert name matches exactly** (stock and custom must use the same name)
2. **Custom alert loads after stock** (it's in `/etc/netdata/health.d/`, not `/usr/lib/...`)
3. **No syntax errors** (check `/var/log/netdata/health.log`)
4. **Health configuration reloaded** (`sudo netdatacli reload-health`)

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

- **2.5 Reloading and Validating Alert Configuration** Deeper troubleshooting and validation techniques
- **Chapter 3: Alert Configuration Syntax** Full syntax reference for writing alert definitions
- **Chapter 4: Controlling Alerts and Noise** Silencing rules, delays, and hysteresis
- **Chapter 11: Built-In Alerts Reference** Catalog of stock alerts shipped with Netdata