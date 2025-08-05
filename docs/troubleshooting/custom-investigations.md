# Custom Investigations

## Overview

Custom Investigations let you ask open-ended questions about your infrastructure and receive deeply researched reports powered by AI. Unlike traditional dashboards or query languages, this conversational interface analyzes your real-time, high-fidelity data to answer complex operational questions in minutes.

### When to Use Custom Investigations

Create investigations for any scenario where you need deep analysis:

- **Troubleshoot complex issues** - Delegate multiple parallel investigations during incidents
- **Analyze deployment impact** - Understand how new releases affect performance
- **Optimize costs** - Identify underutilized resources and quantify savings
- **Explore trends** - Get summaries of system behavior changes over time

### Creating Effective Investigations

The key to powerful investigations is providing context. Think of it like briefing a teammate—the more details you share, the better the analysis.

#### Example 1: Troubleshooting Service Failures

**Your Request:**
```
Why are my checkout-service pods crashing repeatedly?
```

**Your Context:**
```
- Started after: deployment at 14:00 UTC of version 2.3.1
- Impact: Customer checkout failures, lost revenue ~$X/hour
- Recent changes: Updated payment gateway integration, increased worker threads from 10 to 20
- Error pattern in logs: "connection refused to payment-service:8080", "Java heap space"
- Environment: production / eks-prod-us-east-1
- Related services: payment-service, inventory-service, redis-session-store
```

#### Example 2: Analyzing Deployment Changes

**Your Request:**
```
Compare system metrics before and after the recent user-authentication-service deployment.
```

**Your Context:**
```
- Service: user-authentication-service v2.2.0
- Deployed: 2025-01-24 09:00 UTC
- Changes: Switched from JWT to Redis sessions, added Argon2 password hashing
- Specific concerns: Users reporting intermittent logouts, suspicious increase in redis_connected_clients
- Time windows: 24h before deployment vs 24h after
```

#### Example 3: Cost Optimization

**Your Request:**
```
Identify underutilized nodes for cost optimization.
```

**Your Context:**
```
- Monthly AWS bill: $12K for compute
- Environment: Mixed workloads (prod + staging on same cluster)
- Known issues: Dev environments run 24/7, batch processing nodes idle 20h/day
- Goal: Find $2-3K/month in savings without impacting reliability
```

### Starting a Custom Investigation

You can create investigations in two ways:

#### From the Insights Tab
1. Navigate to the **Insights** tab
2. Click **"New Investigation"**
3. Enter your question and context

#### From Anywhere with "Troubleshoot with AI"
Click the **"Troubleshoot with AI"** button in the top right corner from any screen. This automatically captures your current context—including the specific chart, dashboard, or service you're viewing. Add your question and any extra context, then start the investigation.

[SCREENSHOT FROM FIRST BLOG POST SHOULD BE PLACED HERE - showing the Troubleshoot with AI button]

### Getting Your Results

- Reports generate in approximately 2 minutes
- View completed reports in the **Insights** tab
- Receive email notifications when reports are ready

[SCREENSHOT FROM FIRST BLOG POST SHOULD BE PLACED HERE - showing the Insights tab interface]

### Best Practices

1. **Be specific** - Include timeframes, service names, and environments
2. **Add context** - Paste relevant details from tickets, Slack threads, or deployment logs
3. **Set clear goals** - Specify what you're trying to achieve (reduce costs, find root cause, etc.)
4. **Use parallel investigations** - Run multiple investigations simultaneously during incidents

### Access and Availability

This feature is available in preview mode for:
- All Business and Homelab plan users
- New users get 10 AI investigation sessions per month during their Business plan trial
- Community users can request access by contacting product@netdata.cloud

### Coming Soon

We're actively developing:
- Scheduled recurring investigations for regular reports
- Custom SLO report templates
- Weekly cost-optimization analyses
