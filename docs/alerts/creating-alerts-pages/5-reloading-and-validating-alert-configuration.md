# 2.5 Reloading and Validating Alert Configuration

:::tip

Refer to this section when you've edited alert definitions in `/etc/netdata/health.d/` or `/usr/lib/netdata/conf.d/health.d/`, created or modified alerts in Netdata Cloud, or changed health-related settings and want to confirm the result.

:::

## 2.5.1 How Alert Changes Are Applied

Netdata evaluates alerts using the health engine inside each Agent or Parent. That engine reads alert definitions from:

| Source | How It's Applied | Needs Manual Reload? |
|--------|------------------|----------------------|
| **Stock alerts** (`/usr/lib/netdata/conf.d/health.d/`) | Loaded at Agent start | Only when you upgrade Netdata or restart the Agent |
| **Custom alerts** (`/etc/netdata/health.d/`) | Read at Agent start, or when you explicitly reload | **Yes**, after you edit files |
| **Cloud-defined alerts** (Health configuration in Cloud UI) | Pushed from Cloud to connected nodes at runtime | **No**, Agents reload automatically when they receive changes |

:::note

**Where alerts run:** 
Regardless of where you define them (files or Cloud), alerts are evaluated on Agents/Parents, not in Netdata Cloud. Cloud manages configuration and shows events, but the logic runs locally.

:::

**Rule of thumb:** 
If you change files on disk, you need to reload health or restart Netdata. If you change Cloud-defined alerts, Netdata handles propagation and reloads automatically (see **2.3.6 Cloud Alert Lifecycle and Propagation**).

## 2.5.2 Reloading Health Configuration (Without Restart)

When you edit alert configuration files under `/etc/netdata/health.d/` (or override stock files), you should reload only the health engine instead of restarting the whole Agent.

### Basic Reload Command

On the node where Netdata is running:

```bash
sudo netdatacli reload-health
```

What this does:
- Tells the running Netdata process to re-read all health configuration files
- Rebuilds the list of active alerts (stock + custom)
- Reports errors to the log if any file has invalid syntax or references

:::tip

Use `reload-health` whenever you only changed alert definitions. It's faster than restarting and avoids gaps in metrics and dashboards.

:::

### Using Reload in Automation

Because `netdatacli` returns a non-zero exit code on error, you can safely include it in scripts or CI/CD:

```bash
sudo netdatacli reload-health && echo "Health reload OK" || echo "Health reload FAILED"
```

If the reload fails, check logs (see **2.5.4 Method B: Using logs**).

## 2.5.3 When a Full Restart Is Required

Most alert changes do not require a restart. Use a full Agent restart only when:

- You changed global Netdata configuration that affects health behavior indirectly (for example, disabled the health subsystem)
- The health engine is in a bad state and repeated `reload-health` attempts continue to fail
- You are doing deep troubleshooting and want a completely clean start of the Agent

To restart with systemd:

```bash
sudo systemctl restart netdata
```

:::warning

Restarting Netdata may briefly interrupt dashboards and can cause short gaps in alert evaluations and metric collection, depending on your setup. Prefer `netdatacli reload-health` whenever possible.

:::

## 2.5.4 Validating That Alerts Loaded Correctly

After reloading health or restarting Netdata, you should confirm that your new or modified alerts are present, they're attached to the expected chart/context and host, and there are no load or syntax errors.

:::tip

You can validate using the Alarms API on the node, logs on the node, or the Netdata UI (Agent dashboard and Cloud).

:::

<details>
<summary><strong>Using the Alarms API (on the Node)</strong></summary>

The Alarms API shows the alerts the Agent or Parent currently knows about and their active state.

**List All Active Alerts**

On the node, run:

```bash
curl -s "http://localhost:19999/api/v1/alarms?all" | jq '.'
```

Check that your alert appears by name (for example, `10min_cpu_usage`), the associated chart/context is correct, and the status is one of `CLEAR`, `WARNING`, `CRITICAL`, etc.

**Filter for a Specific Alert**

To verify a specific alert by name:

```bash
curl -s "http://localhost:19999/api/v1/alarms" \
  | jq '.alarms | to_entries[] | select(.key | test("10min_cpu_usage"; "i"))'
```

If you see your alert here, it is loaded and evaluating.

:::tip

For more advanced API usage including filtering, querying history, and inspecting variables, see **Chapter 9: APIs for Alerts and Events**.

:::

</details>

<details>
<summary><strong>Using Logs</strong></summary>

Netdata logs every health configuration reload, including errors.

**Check Recent Logs (systemd)**

On most systems using systemd:

```bash
sudo journalctl -u netdata --since "5 minutes ago"
```

Look for messages mentioning `health configuration`, `loading health` or `reload-health`, and specific health files (for example, `health.d/cpu_custom.conf`).

**Check Netdata Log Files**

On systems where Netdata writes to its own log file, you might see something like:

```bash
sudo tail -n 200 /var/log/netdata/health.log
```

Common issues you might see:
- Syntax errors in `warn` or `crit` expressions
- Unknown configuration options
- Missing charts or dimensions referenced by alerts

Fix the referenced issues, then run:

```bash
sudo netdatacli reload-health
```

:::tip

Re-check logs until the reload is clean. For detailed troubleshooting of specific alert problems, see **Chapter 7: Troubleshooting Alert Behaviour**.

:::

</details>

<details>
<summary><strong>Using Netdata Cloud and Agent Dashboards</strong></summary>

**Check from Netdata Cloud**

1. Go to your Space in Netdata Cloud
2. Open the Events view or the Alerts tab
3. Filter or search by your alert name
4. Confirm the alert is listed, it shows up on the expected node(s), and its status (`CLEAR`, `WARNING`, `CRITICAL`) matches current conditions

**Check from the Local Agent Dashboard**

1. Open the node's local dashboard (for example, `http://<host>:19999/`)
2. Navigate to the Health / Alerts section
3. Locate your alert by name
4. Verify it's attached to the expected chart and dimensions, and thresholds and descriptions match your configuration

:::tip

To validate behavior end-to-end, temporarily lower a threshold or simulate the condition (for example, generate CPU load) and watch the alert change status in the UI and Events feed.

:::

</details>

## 2.5.5 Verifying Cloud-Defined Alerts Are Active

Cloud-defined alerts (from **2.3 Creating and Editing Alerts via Netdata Cloud**) are applied automatically, but you should still verify they reached your nodes and are evaluating.

:::note

This is a short checklist. For full details on Cloud alert lifecycle and propagation, see **2.3.6 Cloud Alert Lifecycle and Propagation** and **2.3.7 Testing and Validating Cloud-Defined Alerts**.

:::

<details>
<summary><strong>Quick Verification Steps</strong></summary>

**1. Check in Cloud UI**

Navigate to the node's Health configuration in Cloud (Nodes → select node → arrow-up icon → Configuration → Health) and confirm your alert appears in the list with active/enabled status.

**2. Verify on the Node**

Cloud-defined alerts are pushed to the node and loaded into the health engine. Verify using the Alarms API:

```bash
curl -s "http://localhost:19999/api/v1/alarms" | jq '.alarms'
```

Look for your Cloud-defined alert by name. If it appears with `CLEAR`, `WARNING`, or `CRITICAL` status, it's loaded and evaluating.

**3. Test by Triggering (Optional)**

Simulate the alert condition (for example, generate CPU load) and watch the Events feed in Netdata Cloud for the status transition.

**Common issues:**
- Node not connected to Cloud (check connection status in UI)
- Alert not saved correctly in Cloud UI (check for validation errors)
- Configuration not yet propagated (wait a few seconds, check logs)

For detailed troubleshooting, see **2.3.7** and **Chapter 7: Troubleshooting Alert Behaviour**.

</details>

## 2.5.6 Quick Checklist: From Edit to Verified

Use this checklist every time you change alerts:

**1. Apply the change**
- **Files:** edit `/etc/netdata/health.d/*.conf`, then run:
  ```bash
  sudo netdatacli reload-health
  ```
- **Cloud UI:** create/edit the alert in Health configuration, wait a few seconds for propagation (see **2.3.6**)

**2. Check for errors**
- Run `journalctl -u netdata` or check `/var/log/netdata/health.log`
- Fix any syntax or load errors and reload health again

**3. Confirm presence**
- Use the Alarms API on the node to ensure your alert appears, or
- Confirm it in Netdata Cloud (Alerts tab / Events) or the local Agent dashboard

**4. Test behavior (optional but recommended)**
- Temporarily adjust thresholds or simulate conditions
- Watch the alert transition between `CLEAR`, `WARNING`, and `CRITICAL`

If all four steps succeed, your alert configuration is reloaded, valid, and active in your Netdata deployment.

## Key takeaway

Always validate after making changes. Use `netdatacli reload-health` for file changes, check the Alarms API or logs for errors, and test behavior by triggering the alert when possible.

## What's Next

- **[Chapter 3: Alert Configuration Syntax (Reference)](../alert-configuration-syntax/index.md)** covers full syntax for `alarm` and `template` definitions
- **[Chapter 4: Controlling Alerts and Noise](../controlling-alerts-noise/index.md)** explains how to use silencing, delays, and hysteresis to reduce noise once alerts are active
- **[Chapter 7: Troubleshooting Alert Behaviour](../troubleshooting-alerts/index.md)** provides deeper debugging when alerts don't behave as expected