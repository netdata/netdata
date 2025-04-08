# Alerts and Notifications

Netdata provides two ways to send alert notifications. You can use either one—or both—at the same time.

Alerts are based on each node's health status. You can change thresholds, add new alerts, or silence specific ones using Netdata's alerting system.

→ [See how to configure alerts](/src/health/REFERENCE.md)

---

## How Alert Notifications Work

| Method               | Where Alerts Are Sent From | Customization | Highlights |
|----------------------|----------------------------|---------------|------------|
| **Netdata Cloud**    | Cloud UI                   | Medium        | Centralized alerting using connected nodes’ health status |
| **Netdata Agent**    | Local Netdata Agent        | High          | Node-level alerting with wide integration support |

You can enable one or both methods depending on your needs.

---

## Quick Start

Use this table to choose and set up your preferred alerting method:

| Option               | Setup Location       | Setup Effort | Best For                      |
|----------------------|----------------------|---------------|-------------------------------|
| **Netdata Cloud**    | In the Cloud UI      | Low           | Teams managing multiple nodes |
| **Netdata Agent**    | On each Netdata node | Medium        | Full control and flexibility  |

---

## Example 1: Set Up Alerts via Netdata Cloud

1. Connect your nodes to [Netdata Cloud](https://app.netdata.cloud/).
2. In the UI, go to:  
   `Space → Notifications`.
3. Choose an integration (e.g. Slack, Amazon SNS, Splunk).
4. Set alert severity filters as needed.

→ [See all supported Cloud integrations](/docs/alerts-&-notifications/notifications/centralized-cloud-notifications)

---

## Example 2: Set Up Alerts via Netdata Agent

1. Open the notification config:

```bash
sudo ./edit-config health_alarm_notify.conf
```

2. Enable your preferred method, for example email:

```ini
SEND_EMAIL="YES"
DEFAULT_RECIPIENT_EMAIL="you@example.com"
```

3. Ensure your system can send mail (via `sendmail`, SMTP relay, etc.).
4. Restart the agent:

```bash
sudo systemctl restart netdata
```

→ [See all Agent-based integrations](/docs/alerts-&-notifications/notifications/agent-dispatched-notifications)

---

## About the Agent's Health Monitoring

The Netdata Agent continuously monitors system health and performance. It includes:

- **Hundreds of pre-configured alerts** (covering system, app, and service metrics)
- **No setup required** — works out of the box
- **Dynamic customization** — you can fully control how, when, and what triggers an alert

→ [See which collectors support alerts](/src/collectors/COLLECTORS.md)

---

## Customizing Alerts

You can tune alerts to match your environment:

- Adjust thresholds
- Write custom alert conditions
- Silence alerts temporarily or permanently
- Use statistical functions for smarter alerting

→ [Customize alerts](/src/health/REFERENCE.md)  
→ [Silence or disable alerts](/src/health/REFERENCE.md#disable-or-silence-alerts)

---

## Related Documentation

- [All alert notification methods](/docs/alerts-and-notifications/notifications/README.md)  
- [Supported collectors](/src/collectors/COLLECTORS.md)  
- [Full alert reference](/src/health/REFERENCE.md)