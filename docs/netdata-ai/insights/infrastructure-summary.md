# Infrastructure Summary

The Infrastructure Summary report synthesizes the last hours, days, or weeks of your infrastructure into a concise, shareable narrative. It combines critical timelines, anomaly context, alert analysis, and actionable recommendations so your team can quickly align on what happened and what to do next.

![Infrastructure Summary tab](https://raw.githubusercontent.com/netdata/docs-images/refs/heads/master/netdata-cloud/netdata-ai/infrastructure-summary.png)

## When to use it

- Monday morning recap of weekend incidents and health trends
- Post-incident executive summary for leadership and stakeholders
- Weekly team handoff and situational awareness
- Baseline health before planned infrastructure changes

## How to generate

1. Open Netdata Cloud and go to the `Insights` tab
2. Select `Infrastructure Summary`
3. Choose the time range (last 24h, 48h, 7d, or custom)
4. Scope the analysis to all nodes or a subset (rooms/spaces)
5. Click `Generate`

Reports typically complete in 2–3 minutes. You’ll see them in Insights and receive an email when ready.

## What’s included in the report

- Executive summary of the period with key findings
- Incident timeline with affected services and impact
- Alerts overview: frequency, severity, and patterns
- Detected anomalies with confidence and correlations
- Cross-node correlations and dependency highlights
- Notable configuration changes and deploy events (when available)
- Top recommendations with expected impact and rationale

![Infrastructure Summary report example](https://raw.githubusercontent.com/netdata/docs-images/refs/heads/master/netdata-cloud/netdata-ai/infrastructure-summary-report.png)

## Example: Weekend incident recovery

Generate a 7‑day summary Monday morning to reconstruct what happened while the team was off: which alerts fired, which services were impacted, and where to focus remediation. Use the recommendations section to triage follow-ups.

## Tips for best results

- Scope to the most relevant rooms/services when investigating a targeted issue
- Pair with a dedicated `Anomaly Analysis` report for deep dives
- Save summaries as PDFs for sharing with management or compliance

## Availability and usage

- Available in Netdata Cloud for Business and Free Trial
- Each generated report consumes 1 AI credit (10 free per month on eligible plans)
- Data privacy: metrics are summarized into structured context; your data is not used to train foundation models

## See also

- Performance Optimization
- Capacity Planning
- Anomaly Analysis
- Scheduled Reports

