# Troubleshoot with AI Button

Trigger an AI‑powered investigation from anywhere in Netdata Cloud. The `Troubleshoot with AI` button captures your current context (chart, dashboard, room, or service) and launches an investigation with that scope pre‑selected.

![Troubleshoot with AI button](https://raw.githubusercontent.com/netdata/docs-images/refs/heads/master/netdata-cloud/netdata-ai/troubleshoot-button.png)

## Where to find it

- Alerts tab: `Ask AI` next to any alert
- Insights tab: `Alert Troubleshooting` and `New Investigation`
- Top‑right of most views: `Troubleshoot with AI`
- Alert emails: `Troubleshoot with AI` link

## How it works

1. Click `Troubleshoot with AI`
2. Review the captured scope and time window
3. Add your question and any extra context (symptoms, recent changes)
4. Start the investigation

Within ~2 minutes, you’ll receive a report with:

- Summary of findings and likely root cause
- Correlated metrics/logs across affected systems
- Suggested next steps with rationale

## Tips for better results

- Be explicit about timeframe, environment, and related services
- Paste relevant notes from tickets/Slack/deploy logs
- Run multiple investigations in parallel during incidents

## Availability and credits

- Available on Business and Free Trial plans
- Each run consumes 1 AI credit (10 free per month on eligible plans)

## Privacy

Your infrastructure data is summarized to a compact context for analysis and is not used to train foundation models.

