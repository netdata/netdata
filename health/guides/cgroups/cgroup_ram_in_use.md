### Understand the alert

The Netdata Agent calculates the percentage of used memory. This alert indicates high cgroup memory utilization. Out Of Memory (OOM) killer will kill some processes when the utilization reaches 100%. To fix this issue, try to increase the cgroup memory limit (if set).

This alert is triggered in warning state when the percentage of used memory is between 80-90% and in critical state between 90-98%.
