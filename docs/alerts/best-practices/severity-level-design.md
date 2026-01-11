# 12.2 Severity Level Design

Severity levels should reflect the urgency and impact of the condition being detected.

## Critical Alerts

Critical alerts require immediate response, day or night. A critical alert means someone should stop what they are doing and address the problem within minutes.

Reserve critical severity for conditions causing immediate service impact or data loss risk. A web server process that has exited is critical because visitors cannot access the site. A database replication failure is critical because it creates data loss risk. A disk at 95% capacity is critical because file writes may fail at any moment.

Critical alerts should be rare. A typical production environment might have ten to twenty critical alerts, covering the most essential monitoring.

## Warning Alerts

Warning alerts indicate problems requiring attention but not immediate response. Warning alerts should be handled during normal working hours or added to a backlog for investigation.

Use warning alerts for conditions that precede critical problems. Rising memory usage not yet at critical levels, error rates increased but not yet causing outages, and capacity utilization with room but trending upward all warrant warnings.

Warning alerts should be actionable within hours to days. If a warning alert can wait indefinitely without consequence, it does not need to exist.

## Informational Alerts

Informational alerts require no immediate action but should be visible for awareness and trend analysis. These alerts help build operational visibility without interrupting workflows.

Use informational alerts for significant state changes that do not represent problems. A node coming online after maintenance, a backup completing successfully, or a service reaching expected load levels fall into this category.

Information-level alerts should not generate notifications requiring acknowledgment. They should appear in dashboards and event feeds for reference.