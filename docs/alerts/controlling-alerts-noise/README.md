# 4. Controlling Alerts and Noise

Now that you know **how to create alerts** (Chapter 2) and **how they work** (Chapter 1), this chapter shows you how to **control alert behavior** to reduce noise without losing important signals.

Every alerting system faces the same challenge: **alert fatigue**. When alerts fire too frequently or for conditions that don't need immediate attention, responders start to ignore them—and that's when real problems get missed.

Netdata gives you multiple layers of control:

| Layer | What It Does | When to Use |
|-------|---------------|-------------|
| **Disabling** | Stops alert evaluation entirely | You never want this alert to run |
| **Silencing** | Evaluates but suppresses notifications | You want to defer or skip notifications for a period |
| **Delays & Hysteresis** | Requires conditions to hold before changing status | Preventing flapping between states |
| **Repeat Intervals** | Limits notification frequency | Avoids notification storms for sustained conditions |

## What You'll Find in This Chapter

| Section | What It Covers |
|---------|----------------|
| **4.1 Disabling Alerts** | How to completely stop alert evaluation (globally, per-host, or per-alert) |
| **4.2 Silencing vs Disabling** | The critical difference between stopping evaluation vs. suppressing notifications |
| **4.3 Silencing in Netdata Cloud** | How to use Cloud silencing rules for space-wide or scheduled quiet periods |
| **4.4 Reducing Flapping and Noise** | Practical techniques: delays, hysteresis, repeat intervals, and smoothing |

## How to Navigate This Chapter

- Start at **4.1** if you need to permanently stop an alert from running
- Jump to **4.2** if you're unsure whether to disable or silence
- Use **4.3** for Cloud-managed silencing rules
- Go to **4.4** if alerts are flapping between statuses

:::note

For temporary quiet periods (maintenance windows, overnight staffing), see **4.3 Silencing in Netdata Cloud**.

For persistent issues (an alert is never relevant to your environment), see **4.1 Disabling Alerts**.

:::

## What's Next

- **4.1 Disabling Alerts** covers permanent removal of alert evaluation
- **4.2 Silencing vs Disabling** explains when to use each approach
- **4.3 Silencing in Netdata Cloud** for Cloud-managed rules
- **4.4 Reducing Flapping and Noise** for preventing rapid status changes