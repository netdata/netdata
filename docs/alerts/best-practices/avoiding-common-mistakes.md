# 12.10 Avoiding Common Mistakes

Certain patterns consistently cause problems. Avoid these common mistakes.

## Alerting on Symptoms Not Causes

Alerting on symptoms instead of causes wastes time. A symptom alert fires and leaves responders to investigate the cause. A cause alert points directly to the problem.

For example, a server with high latency is a symptom. The cause might be CPU saturation, memory pressure, or network congestion. An alert on high latency forces investigation; alerts on the specific resource constraints allow direct action.

Start with cause alerts where possible.

## Over-Monitoring

More monitoring is not always better. Excessive alerts create noise that masks genuine problems and train responders to ignore everything.

Audit monitoring coverage periodically. Identify alerts that have never fired or have fired thousands of times without action.

Prioritize quality over quantity. Ten well-tuned alerts that always fire meaningfully outweigh a hundred noisy alerts creating confusion.

## Forgetting Maintenance Windows

Maintenance windows, deployments, and other scheduled activities generate false positives. Configure silence rules in advance.

Remember to remove silence rules when maintenance ends. Orphaned silence rules hide real problems.