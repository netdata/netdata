# AI Insights

**From hours of debugging to minutes of clarity** - AI Insights transforms your infrastructure monitoring data into professional reports that explain what happened, why it happened, and what to do about it.

## The Challenge AI Insights Solves

Traditional monitoring requires you to manually query metrics, correlate data, and build dashboards during incidents - all while the clock is ticking. Even experienced engineers struggle with:

- Learning complex query languages (PromQL, SQL) just to ask basic questions
- Building custom dashboards during incidents instead of fixing problems
- Correlating metrics across multiple systems to find root causes
- Translating technical metrics into business impact for stakeholders
- Spending hours on post-incident analysis and reporting

**AI Insights eliminates these barriers** by automatically analyzing your infrastructure and delivering comprehensive reports that provide both executive summaries and technical deep-dives.

## Why AI Insights Transforms Operations

- **No query languages needed** - Skip the learning curve of PromQL, SQL, or custom dashboards
- **AI with SRE expertise** - Get analysis from an AI trained to think like a senior engineer
- **Root cause, not symptoms** - Understand the cascade of issues, not just surface metrics
- **Business context included** - Reports explain technical issues in terms of business impact
- **Collaborative by design** - Share professional PDFs with stakeholders who need answers, not dashboards
- **Powered by Netdata's ML** - Leverages anomaly scores from ML models trained on every metric
- **Zero configuration needed** - Works immediately with your existing Netdata deployment

## Four Specialized Report Types

![AI Insights Report Example](https://github.com/user-attachments/assets/c6997afb-94cb-41cc-a038-b384cb92e751)

### Infrastructure Summary

**Your automated health check and incident analyst**

Perfect for Monday morning reviews, post-incident analysis, or executive updates. This report provides:

- Complete system health assessment with prioritized issues
- Timeline of incidents and their business impact
- Critical alerts analysis with resolution recommendations
- Top 3 actionable items to improve infrastructure health
- Performance trends across all key metrics

**Use cases**: Weekend incident recovery, executive briefings, team handoffs, regular health checks

[Learn more →](/docs/netdata-ai/insights/infrastructure-summary.mdx)

### Capacity Planning

**Stop guessing future needs - get data-driven projections**

Make informed decisions about infrastructure investments with reports that include:

- Resource utilization trends and growth patterns
- Predicted capacity exhaustion dates for critical resources
- Specific hardware recommendations based on usage patterns
- Cost optimization opportunities
- Projections for 3 months to 2 years ahead

**Use cases**: Quarterly planning, budget justification, infrastructure roadmaps, vendor negotiations

[Learn more →](/docs/netdata-ai/insights/capacity-planning.mdx)

### Performance Optimization

**Find and fix bottlenecks before users complain**

Identify inefficiencies and optimization opportunities with:

- Bottleneck analysis across application, database, network, and storage
- Resource contention patterns and their impact
- Specific tuning recommendations with expected improvements
- Prioritized list of optimizations by potential impact
- Before/after projections for recommended changes

**Use cases**: Performance audits, system tuning, SRE optimization projects, efficiency improvements

[Learn more →](/docs/netdata-ai/insights/performance-optimization.mdx)

### Anomaly Analysis

**Post-incident forensics made simple**

Understand unusual patterns and prevent future issues with:

- ML-detected anomalies with severity scoring
- Root cause analysis showing how issues cascaded
- Timeline reconstruction of anomaly propagation
- Correlation between different system anomalies
- Recommendations to prevent recurrence

**Use cases**: Post-mortems, proactive issue detection, system behavior analysis, troubleshooting

[Learn more →](/docs/netdata-ai/insights/anomaly-analysis.mdx)

## Customize Reports to Your Needs

Each report type offers flexible customization options for content and analysis scope (note: report structure and visual style are standardized for consistency):

### Time Period Selection

- **Infrastructure Summary**: Last 24 hours, 48 hours, 7 days, or month
- **Capacity Planning**: Forecast for 3 months, 6 months, 1 year, or 2 years
- **Performance Optimization**: Last 24 hours, 7 days, month, or quarter
- **Anomaly Analysis**: Last 6 hours, 12 hours, 24 hours, or 7 days

### Scope and Filtering

- **Node Selection**: Analyze specific servers or your entire infrastructure
- **Metric Categories**: Focus on CPU, Memory, Disk, Network, or Applications
- **Resource Types**: Target Compute, Storage, Network, or Database resources
- **Focus Areas**: Drill into specific performance domains
- **Anomaly Thresholds**: Set sensitivity levels (10%, 20%, or 30%)

## How AI Insights Works

### 1. Intelligent Data Collection

When you request a report, AI Insights:

- Gathers relevant metrics from your selected time period and nodes
- Collects active alerts and their severity levels
- Retrieves ML-detected anomalies and their scores
- Maps system relationships and dependencies
- Compiles process and application performance data

### 2. AI-Powered Analysis

The collected data is analyzed by Anthropic's Claude 3.7 Sonnet model, optimized for infrastructure telemetry analysis using SRE methodologies. This AI model:

- Applies SRE-level expertise to identify patterns
- Correlates issues across different systems
- Determines root causes vs symptoms
- Prioritizes findings by business impact
- Generates actionable recommendations

### 3. Professional Report Generation

Within 2-3 minutes, you receive:

- **Structured content**: Headers, insights, charts, and tables in logical flow
- **Embedded visualizations**: Charts generated from your actual metrics
- **Executive summary**: High-level findings for stakeholders
- **Technical details**: Deep-dive analysis for engineers
- **Action items**: Prioritized recommendations with clear next steps
- **PDF format**: Professional reports ready for sharing

### 4. Security and Privacy

- **In-memory processing**: Data analyzed then immediately discarded
- **No training data**: Your infrastructure data is never used for model training
- **Secure API**: All communications encrypted end-to-end
- **Access controlled**: Respects your existing Netdata permissions

## Real-World Impact

From the Inrento fintech case study:
> "AI Insights provided **significant time savings** in identifying and resolving issues. It **drastically reduced the time spent** identifying problems and implementing solutions, leading to **enhanced productivity and performance** with **minimized downtime**."
Teams report that incident analysis that previously took hours of manual investigation now completes in minutes with AI Insights.

## Automate with Schedules

Set up recurring runs for Insights and custom investigations to receive weekly health summaries, monthly optimization reviews, and SLO conformance reports automatically.

[Learn more →](/docs/netdata-ai/insights/scheduled-reports.mdx)

## Perfect For

- **Incident post-mortems**: Generate comprehensive analysis in minutes, not hours
- **Executive briefings**: Professional PDFs with clear summaries and visualizations  
- **Capacity reviews**: Data-driven planning for budget and resource allocation
- **Performance audits**: Regular health checks without manual analysis
- **Team handoffs**: Share context-rich reports instead of dashboard links
- **Compliance reporting**: Document infrastructure state and changes
- **Vendor discussions**: Data-backed evidence for infrastructure decisions

## Unlike Traditional Monitoring

AI Insights represents a paradigm shift in infrastructure monitoring:

| Traditional Monitoring | AI Insights |
|------------------------|-------------|
| Build dashboards during incidents | Get instant analysis |
| Learn query languages | Use natural language selection |
| Manual correlation across metrics | Automatic relationship detection |
| Raw metrics without context | Narrative explanations with context |
| Technical data only | Business impact included |
| Hours of manual analysis | 2-3 minute automated reports |

## What Sets AI Insights Apart

Unlike traditional AI monitoring assistants that require extensive configuration or operate as black-box cloud services, AI Insights:

- **Runs entirely on your infrastructure** - No external dependencies or mysterious cloud processing
- **Uses your actual data** - Not generic patterns or industry averages
- **Provides transparent analysis** - Clear reasoning, not black-box decisions
- **Respects your security** - Data never leaves your control
- **Works instantly** - No training period or configuration required

## Getting Started

1. **Access AI Insights** from the Netdata Cloud navigation menu
2. **Select a report type** based on your current need
3. **Customize parameters** like time period and node selection
4. **Generate report** and receive it within 2-3 minutes
5. **Share or download** the PDF for stakeholders

## Technical Requirements

- Active Netdata Cloud account
- At least one connected Netdata Agent
- Historical data (minimum 24 hours recommended)
- No additional configuration needed

## Frequently Asked Questions

**Q: How far back can AI Insights analyze data?**
A: AI Insights can analyze any data retained by your Netdata agents, from 6 hours to 2 years depending on the report type and your retention settings.

**Q: Can I schedule regular reports?**
A: Currently reports are generated on-demand. Scheduled reports are on the roadmap.

**Q: What metrics are included in the analysis?**
A: AI Insights analyzes all metrics collected by your Netdata agents, including system metrics, application metrics, and custom collectors.

**Q: How does it handle sensitive data?**
A: All data is processed securely and discarded after report generation. No data is stored or used for training.

**Q: Can I customize the report format?**
A: Report structure and visual style are standardized for consistency and professional presentation. However, you have extensive control over the analysis scope, time periods, metrics, and focus areas through customization parameters.

## What's Next

AI Insights continues to evolve with new capabilities planned:

- Scheduled report generation
- Custom report templates
- API access for automation
- Integration with ticketing systems
- Comparative analysis between time periods

Experience the future of infrastructure monitoring - transform your data into intelligence with AI Insights.
