# AI-Powered Alert Troubleshooting

## Overview

When an alert fires, you can use AI to generate a detailed troubleshooting report that analyzes whether the alert requires immediate action or is just noise. The AI examines your alert's history, correlates it with thousands of other metrics across your infrastructure, and provides actionable insights—all within minutes.

### Key Benefits

- **Save hours of manual investigation** - Skip the initial data collection and correlation work
- **Reduce alert fatigue** - Quickly identify false positives versus legitimate issues
- **Get actionable guidance** - Receive specific next steps based on the analysis
- **Start from insight, not zero** - Begin troubleshooting with a comprehensive baseline analysis

### How It Works

The AI troubleshooting engine performs three key analyses when you trigger an investigation:

1. **Alert Analysis** - Examines the alert's history and underlying metric behavior to determine if it's a transient false positive or legitimate issue
2. **Correlation Discovery** - Scans thousands of metrics and log patterns across your infrastructure to identify what else was behaving abnormally at the same time
3. **Root Cause Hypothesis** - Provides a summary of findings and suggests likely root causes, pointing you to the most relevant metrics or dimensions

### Starting an Alert Investigation

You can trigger AI troubleshooting in three ways:

#### From the Alerts Tab
Click the **"Ask AI"** button next to any active or recent alert.

![Alert tab with Ask AI button highlighted](screenshot-alerts-tab-ask-ai.png)

#### From the Insights Tab
1. Navigate to the **Insights** tab
2. Select **"Alert Troubleshooting"** from the investigations section
3. Choose any recent alert from the dropdown menu

![Insights tab showing Alert Troubleshooting option](screenshot-insights-alert-troubleshooting.png)

#### From Alert Notifications
When you receive an alert email, click the **"Troubleshoot with AI"** link to automatically start the investigation.

![Email notification with Troubleshoot with AI link](screenshot-email-troubleshoot-link.png)

### Understanding Your Report

Reports typically generate in 1-2 minutes. Once complete:
- The report appears in your **Alerts** tab
- A copy is saved in the **Insights** tab under "Investigations"
- You receive an email notification with the analysis summary

### Access and Availability

This feature is available in preview mode for:
- All Business and Homelab plan users
- New users get 10 AI troubleshooting sessions per month during their Business plan trial

:::note

Community users can request access by contacting product@netdata.cloud

:::