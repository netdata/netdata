# Troubleshooting

Netdata AI accelerates troubleshooting with three complementary tools:

- **Alert Troubleshooting:** one-click analysis that starts from an alert.
- **Anomaly Advisor:** interactive, machine-learning-driven investigation of anomalous metrics.
- **Metric Correlations:** ranking of charts related to a selected time window.

Use Alert Troubleshooting to start from an alert with an automated baseline. Pivot to Anomaly Advisor for propagation analysis and to Metric Correlations to narrow the search space across charts.

## Choose the starting tool

| Starting evidence | Use | Result |
|:------------------|:----|:-------|
| A warning or critical alert | Alert Troubleshooting | An assessment of the alert, correlated signals, and a root-cause hypothesis. |
| A visible incident window or a suspected anomaly cascade | Anomaly Advisor | Infrastructure anomaly timelines and a ranked list of anomalous metrics. |
| A spike, drop, or other interesting interval on a chart | Metric Correlations | Charts whose behavior differs most between the selected interval and its baseline. |

These tools reduce the search space; they do not replace verification. Confirm a proposed cause against the relevant
metrics, logs, topology, deployment events, and application state before taking corrective action.

## Alert Troubleshooting

Generate a report that assesses alert validity, uncovers correlated signals, and proposes a root‑cause hypothesis with supporting evidence. Start from the Alerts tab (`Ask AI`), Insights (`Alert Troubleshooting`), or the link in alert emails.

The analysis examines the alert history and underlying metric, searches for other abnormal behavior around the same time,
and distinguishes a likely incident from transient noise. Use it when the alert is your clearest starting signal or when you
need an initial incident summary before deeper investigation.

Read the [Alert Troubleshooting guide](/docs/troubleshooting/troubleshoot.md) for the available entry points, analysis stages,
and report workflow.

## Anomaly Advisor

Explore incident timelines visually and see how anomalies cascade across your infrastructure. Start from the Anomalies tab in Netdata Cloud.

Netdata's anomaly detection models evaluate each metric, while the Advisor aggregates the results into node-level anomaly
rates and ranks metrics for the highlighted interval. It is well suited to sudden changes and cascading failures where many
related metrics deviate in sequence.

The Advisor cannot infer a problem from a service that stops emitting data, and gradual degradation can resemble recently
learned normal behavior. Treat missing data and slow trends as separate checks. See the
[Anomaly Advisor reference](/docs/ml-ai/anomaly-advisor.md) for its scoring model, workflow, and limitations.

## Metric Correlations

From any dashboard or time window, surface the charts most related to your selection to speed root cause analysis.

Highlight an interval containing the behavior you want to explain. Metric Correlations compares it with a reference window
that immediately precedes it and is four times as long, then filters the dashboard toward the metrics with the strongest
change. You can analyze raw metric values or anomaly-rate data, depending on whether the incident is best represented by a
value change or by departure from learned behavior.

Use the [Metric Correlations guide](/docs/metric-correlations.md) for the selection workflow, ranking methods, adjustable
options, API behavior, and interpretation guidance.

## A practical investigation sequence

1. Define the incident window precisely. Include the first visible deviation, not only the point when a user noticed it.
2. Start from the strongest evidence: an alert, an anomaly cluster, or a chart interval.
3. Review the highest-ranked signals and separate causes from downstream effects using timing and system relationships.
4. Check the same interval in logs and deployment or configuration history.
5. Test the working hypothesis against unaffected nodes or services. A cause should explain both what changed and the
   observed blast radius.
6. After remediation, verify that the original metric, correlated signals, and alert state return to expected behavior.

## See also

- [Alerts Automation](/docs/netdata-ai/alerts-automation/alerts-automation.md) for AI-powered alert creation and suggestions.
- [Investigations](/docs/netdata-ai/investigations/index.md) for open-ended questions with richer infrastructure context.
