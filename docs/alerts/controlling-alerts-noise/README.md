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
| **[4.1 Disabling Alerts](1-disabling-alerts.md)** | How to completely stop alert evaluation (globally, per-host, or per-alert) |
| **[4.2 Silencing vs Disabling](2-silencing-vs-disabling.md)** | The critical difference between stopping evaluation vs. suppressing notifications |
| **[4.3 Silencing in Netdata Cloud](3-silencing-cloud.md)** | How to use Cloud silencing rules for space-wide or scheduled quiet periods |
| **[4.4 Reducing Flapping and Noise](4-reducing-flapping.md)** | Practical techniques: delays, hysteresis, repeat intervals, and smoothing |