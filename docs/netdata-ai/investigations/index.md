# Investigations

Ask Netdata anything about your infrastructure and get a deeply researched answer in minutes. Investigations turn your question and context into an analysis that correlates metrics, anomalies, and events across your systems.

## What Investigations are good for

- Troubleshooting live incidents without manual data wrangling
- Analyzing the impact of deployments or config changes
- Cost and efficiency reviews (identify underutilized resources)
- Exploring longer‑term behavioral changes and trends

## Starting an investigation

Two easy entry points:

- `Troubleshoot with AI` button (top‑right): Captures the current chart, dashboard, or service context automatically, then you add your question
- `Insights` → `New Investigation`: Blank canvas for any custom prompt

Reports complete in ~2 minutes and are saved in Insights; you’ll get an email when ready.

## Provide good context (get great results)

Think of it like briefing a teammate. Include timeframes, environments, related services, symptoms, and recent changes. Example formats:

### Example: Troubleshoot a problem
Request: Why are my checkout‑service pods crashing repeatedly?

Context:
```
- Started after: deployment at 14:00 UTC of version 2.3.1
- Impact: Customer checkout failures, lost revenue ~$X/hour
- Recent changes: payment gateway integration update; workers 10→20
- Logs: "connection refused to payment-service:8080", "Java heap space"
- Environment: production / eks-prod-us-east-1
- Related: payment-service, inventory-service, redis-session-store
```

### Example: Analyze a change
Request: Compare metrics before/after the user‑authentication‑service deploy.

Context:
```
- Service: user-authentication-service v2.2.0
- Deployed: 2025‑01‑24 09:00 UTC
- Changes: JWT→Redis sessions; Argon2 hashing added
- Concern: intermittent logouts; rising redis_connected_clients
- Windows: 24h before vs 24h after
```

### Example: Cost optimization
Request: Identify underutilized nodes for cost savings.

Context:
```
- Monthly compute: ~$12K
- Mixed workloads (prod + staging)
- Dev envs run 24/7; batch nodes idle 20h/day
- Goal: save $2–3K/month without reliability impact
```

## Availability and credits

- Available to Business and Free Trial plans
- Each run consumes 1 AI credit (10 free per month on eligible plans)

## Related documentation

- [Custom Investigations](/docs/netdata-ai/investigations/custom-investigations.md)
- [Scheduled Investigations](/docs/netdata-ai/investigations/scheduled-investigations.md)
- [Alert Troubleshooting](/docs/troubleshooting/troubleshoot.md)
