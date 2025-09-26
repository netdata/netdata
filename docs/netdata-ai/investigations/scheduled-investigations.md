# Scheduled Investigations

Automate recurring custom analyses by scheduling your own investigation prompts. Great for weekly health checks, monthly cost reviews, and SLO conformance reporting.

![Schedule dialog 1](https://raw.githubusercontent.com/netdata/docs-images/refs/heads/master/netdata-cloud/netdata-ai/schedule1.png)

## How to schedule

1. Go to the `Insights` tab → `New Investigation`
2. Enter your prompt and set scope/time window
3. Click `Schedule` and choose cadence (daily/weekly/monthly)
4. Confirm recipients (email) and save

At the scheduled time, Netdata AI runs the investigation and delivers the report to your email and the Insights tab.

![Schedule dialog 2](https://raw.githubusercontent.com/netdata/docs-images/refs/heads/master/netdata-cloud/netdata-ai/schedule2.png)

## Examples

### Weekly health check
Prompt:
```
Generate a weekly infrastructure summary for services A, B, C. Include major incidents,
anomalies, capacity risks, and recommended follow‑ups.
```

### Monthly optimization review
Prompt:
```
Analyze performance regressions and right‑sizing opportunities over the past month for
our Kubernetes workloads in room X. Prioritize actions by potential impact.
```

### SLO conformance
Prompt:
```
Generate an SLO conformance report for 'user-auth' (99.9% uptime, p95 latency <200ms)
and 'payment-processing' (99.99% uptime, p95 <500ms) for the last 7 days. Include
breaches, contributing factors, and remediation recommendations.
```

## Manage schedules

- Edit, pause, or delete schedules from the Insights tab
- Scheduled runs consume AI credits when they execute

## Availability and credits

- Available on Business and Free Trial plans
- 10 free AI runs/month on eligible Spaces; additional usage via AI Credits

