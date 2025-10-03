# Scheduled Reports

Automate your reporting workflow. Scheduled AI reports let you run Insights and Investigations on a recurring cadence and deliver the results automatically—turning manual, repetitive work into a hands‑off process.

![Schedule dialog 1](https://raw.githubusercontent.com/netdata/docs-images/refs/heads/master/netdata-cloud/netdata-ai/schedule1.png)

## What you can schedule

- Any pre‑built Insight: Infrastructure Summary, Performance Optimization, Capacity Planning, Anomaly Analysis
- Custom Investigations (your own prompts and scope)

## How to schedule a report

1. Go to the `Insights` tab in Netdata Cloud
2. Pick an Insight type or click `New Investigation`
3. Configure the time range and scope
4. Click `Schedule` (next to `Generate`)
5. Choose cadence (daily/weekly/monthly) and time

At the scheduled time, Netdata AI runs the report and delivers it to your email and the Insights tab.

![Schedule dialog 2](https://raw.githubusercontent.com/netdata/docs-images/refs/heads/master/netdata-cloud/netdata-ai/schedule2.png)

## Example setups

### Weekly infrastructure health
- Type: Infrastructure Summary
- Time range: Last 7 days
- Schedule: Mondays 09:00

### Monthly performance optimization
- Type: Performance Optimization
- Time range: Last month
- Schedule: 1st of each month 10:00

### Automated SLO conformance
- Type: New Investigation
- Prompt: Generate SLO conformance for services X and Y with targets …
- Schedule: Mondays 10:00

## Managing schedules

- View, pause, or edit schedules from the Insights tab
- Scheduled runs consume AI credits when they execute

## Availability and usage

- Available to Business and Free Trial plans
- Each scheduled run consumes 1 AI credit (10 free/month on eligible plans)

## Tips

- Start with weekly summaries to establish a baseline
- Schedule targeted reports for critical services or high‑cost areas
- Use schedules to feed regular Slack/email updates and leadership briefs

