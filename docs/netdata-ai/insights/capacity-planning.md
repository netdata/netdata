# Capacity Planning

Stop guessing and plan with confidence. The Capacity Planning report projects growth, highlights inflection points, and recommends concrete hardware or configuration changes backed by your actual utilization trends.

![Capacity Planning tab](https://raw.githubusercontent.com/netdata/docs-images/refs/heads/master/netdata-cloud/netdata-ai/capacity-planning.png)

## When to use it

- Quarterly/annual planning and budgeting cycles
- Preparing procurement requests and vendor discussions
- Evaluating consolidation and right‑sizing opportunities

## How to generate

1. Open `Insights` in Netdata Cloud
2. Select `Capacity Planning`
3. Pick a historical window and forecast horizon (3–24 months)
4. Scope to nodes, rooms, or services
5. Click `Generate`

## What’s analyzed

- Historical utilization and growth trends (CPU, memory, storage, network)
- Variability, seasonality, and workload patterns
- Anomaly‑adjusted baselines for accurate projections
- Cross‑node comparisons and consolidation candidates

## What you get

- Exhaustion date estimates for key resources
- Headroom analysis and risk categorization
- Concrete recommendations (e.g., instance types, disk tiers, scaling)
- Opportunity map for consolidation and cost savings

![Capacity Planning report example](https://raw.githubusercontent.com/netdata/docs-images/refs/heads/master/netdata-cloud/netdata-ai/capacity-planning-report.png)

## Example: Quarterly planning

Produce a report that justifies next‑quarter spend: show utilization trends, where headroom is tight, when you’ll breach capacity, and specific remediation options with trade‑offs.

## Best practices

- Run monthly; compare sequential reports for trend confidence
- Pair with `Performance Optimization` to validate trade‑offs
- Use room‑level scoping to build service‑oriented plans

## Availability and usage

- Available on Business and Free Trial plans
- Each report consumes 1 AI credit (10 free per month on eligible plans)
- Reports are saved in Insights and downloadable as PDFs

