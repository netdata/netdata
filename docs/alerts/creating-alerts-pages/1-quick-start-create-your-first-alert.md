# 2.1 Quick Start: Create Your First Alert

This section walks you through creating a simple alert in **under 5 minutes** using either configuration files or the Netdata Cloud UI.

:::note

Both paths produce the same result: a working alert that monitors your filesystems and warns you when free space drops below 20%.

:::

## Choose Your Path

<details>
<summary><strong>Via Configuration Files (File-Based)</strong></summary>

**What You'll Create**

An alert that triggers a WARNING when any filesystem has less than 20% free space.

**Prerequisites**

- SSH or local access to a Netdata Agent or Parent node
- Basic command-line familiarity

**Step 1: Create a Custom Alert File**

```bash
sudo /etc/netdata/edit-config health.d/my_first_alert.conf
```

:::tip

`edit-config` is Netdata's helper script that safely creates or edits configuration files. If it's not available, you can manually create `/etc/netdata/health.d/my_first_alert.conf` with your preferred editor.

:::

**Step 2: Add the Alert Definition**

Paste the following into the file:

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
       to: sysadmin
```

:::note

This example uses a `template` which monitors **all filesystems**, not just root. This is the recommended approach because:
- It automatically monitors all mount points
- New disks are monitored automatically without configuration changes
- It matches Netdata's stock alert patterns

If you only want to monitor a specific filesystem, see **Chapter 3: Alert Configuration Syntax** for how to create chart-specific `alarm` definitions.

:::

What this does:

- Monitors the `disk.space` context (which tracks all filesystem usage)
- Checks the average available space percentage over the last minute
- Triggers WARNING if below 20%, CRITICAL if below 10%
- Sends notifications to the `sysadmin` recipient group

**Step 3: Save and Exit**

Save the file and exit your editor.

**Step 4: Reload Health Configuration**

Tell Netdata to load your new alert:

```bash
sudo netdatacli reload-health
```

You should see:

```
Health configuration reloaded
```

:::note

If `netdatacli` is not available, restart the Netdata service:

```bash
sudo systemctl restart netdata
```

:::
:::

**Step 5: Verify the Alert Loaded**

Check that your alert is active:

```bash
curl -s "http://localhost:19999/api/v1/alarms?all" | grep -i "disk_space_usage"
```

You should see JSON output containing your alert name and current status. If all filesystems have more than 20% free space, the alert status will be `CLEAR` (healthy). This is expected, the alert is working correctly.

Alternative: Open your local Netdata dashboard at `http://your-node-ip:19999`, click the 🔔 bell icon (top-right), and look for `disk_space_usage` in the alerts list.

**Step 6: Test the Alert (Optional)**

To verify the alert fires, you can temporarily lower the threshold:

1. Edit the file again: `sudo /etc/netdata/edit-config health.d/my_first_alert.conf`
2. Change `warn: $this < 20` to `warn: $this < 99` (so it triggers immediately)
3. Reload: `sudo netdatacli reload-health`
4. Check the dashboard, your alert should now show WARNING status
5. Change the threshold back and reload again

**What's Next**

- 2.2 Creating and Editing Alerts via Configuration Files Deep dive into file-based workflow, syntax, and best practices
- Chapter 3: Alert Configuration Syntax Full reference for all alert definition lines (`lookup`, `calc`, `every`, etc.)

</details>

<details>
<summary><strong>Via Netdata Cloud UI (Cloud-Based)</strong></summary>

**What You'll Create**

An alert that triggers a WARNING when any filesystem has less than 20% free space, managed centrally via Netdata Cloud.

**Prerequisites**

- A Netdata Cloud account (free or paid)
- At least one connected Agent or Parent node
- Appropriate permissions (Space Admin or higher)

**Step 1: Open the Alerts Configuration Manager**

1. Log in to Netdata Cloud
2. Navigate to your Space
3. In the left sidebar, click Alerts → Alert Configuration

**Step 2: Create a New Alert**

Click + Add Alert (top-right).

**Step 3: Configure the Alert**

Fill in the form as follows:

| Field | Value |
|-------|-------|
| Alert Name | `disk_space_usage` |
| Description | Filesystem has less than 20% free space |
| Metric | Search for and select `disk.space` |
| Dimension | `avail` (available space) |
| Detection Mode | Static Threshold |
| Aggregation | `average` |
| Time Window | `1 minute` |
| Warning Threshold | `< 20` (percent) |
| Critical Threshold | `< 10` (percent) |
| Notification Recipients | Select your preferred recipient or integration |

:::tip 

Detection Mode options in the UI

- Static Threshold Fixed values (what we're using here)
- Dynamic Baseline ML-based anomaly detection
- Rate of Change Alerts on sudden spikes or drops

:::

**Step 4: Save the Alert**

Click Save at the bottom of the form.

Netdata Cloud will immediately push this alert definition to all connected nodes in your space.

**Step 5: Verify the Alert is Active**

1. Go to the Events Feed (left sidebar → Events)
2. You should see a new event: `Alert disk_space_usage created`
3. Navigate to any node's dashboard (click Nodes → select a node)
4. Click the 🔔 bell icon (top-right)
5. Look for `disk_space_usage` in the alerts list with status `CLEAR` (if your disks are healthy). If all filesystems have more than 20% free space, seeing `CLEAR` status is expected, the alert is working correctly.

**Step 6: Test the Alert (Optional)**

To verify the alert fires:

1. Return to Alerts → Alert Configuration
2. Find your alert and click Edit
3. Change Warning Threshold to `< 99` (so it triggers immediately)
4. Click Save
5. Wait 1 to 2 minutes, then check the Events Feed, you should see a new `WARNING` event
6. Edit the alert again and restore the threshold to `< 20`

**Next Steps**

- 2.3 Creating and Editing Alerts via Netdata Cloud Full guide to the Alerts Configuration Manager, including advanced options and bulk operations
- Chapter 5: Receiving Notifications How to configure notification integrations (Slack, PagerDuty, email, etc.)

</details>

## What You Just Did

Regardless of which path you chose, you've now:

- Created a working alert that monitors filesystem usage across all mount points
- Configured warning and critical thresholds
- Verified the alert loaded and is actively evaluating
- (Optionally) Tested that the alert fires when conditions are met

## Key Takeaway

Both workflows (file-based and Cloud UI) produce alerts that **run on your Agents or Parents**. The difference is only where you define and store the rule.

## Common Questions

<details>
<summary><strong>Can I use both workflows at the same time?</strong></summary>

**Yes.** File-based and Cloud-defined alerts coexist. You can manage fleet-wide alerts in Cloud and node-specific alerts in files.

</details>

<details>
<summary><strong>What if my alert didn't load?</strong></summary>

- **File-based:** Check `/var/log/netdata/health.log` for syntax errors
- **Cloud UI:** Ensure your node is connected to Cloud and has a recent agent version
- See **2.5 Reloading and Validating Alert Configuration** for detailed troubleshooting

</details>

<details>
<summary><strong>How do I get notified when this alert fires?</strong></summary>

- **File-based:** Configure `health_alarm_notify.conf` (see **5.2 Configuring Agent and Parent Notifications**)
- **Cloud UI:** Set up notification integrations in Cloud (see **5.3 Configuring Cloud Notifications**)

</details>

<details>
<summary><strong>Can I monitor something other than disk space?</strong></summary>

**Absolutely.** This example uses `disk.space` because it's universal and easy to test. You can monitor:

- CPU usage (`system.cpu`)
- Memory pressure (`system.ram`)
- Network traffic (`net.net`)
- Application metrics (databases, web servers, etc.)

See **Chapter 6: Alert Examples and Common Patterns** for more ideas.

**Note:** This Quick Start example monitors **all filesystems** using a `template`. To monitor only specific filesystems (like just root `/`), you'll need to use an `alarm` definition with a specific chart ID. See **3.1 Alert Definition Lines** for the difference between templates and alarms.

</details>

## What's Next

Now that you have a working alert, you can:

- **[Customize it further](../alert-configuration-syntax/index.md)** Learn the full syntax in **Chapter 3: Alert Configuration Syntax**
- **[Create more alerts](2-creating-and-editing-alerts-via-config-files.md)** Follow detailed workflows in **2.2** (files) or **[2.3](3-creating-and-editing-alerts-via-cloud.md)** (Cloud UI)
- **[Control notification noise](../controlling-alerts-noise/index.md)** See **Chapter 4: Controlling Alerts and Noise**
- **[Set up notifications](../receiving-notifications/index.md)** See **Chapter 5: Receiving Notifications**