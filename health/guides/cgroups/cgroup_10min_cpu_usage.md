### Understand the alert

The Netdata Agent calculates the average CPU utilization over the last 10 minutes. This alert indicates that your system is in high cgroup CPU utilization. The system will throttle the group CPU usage when the usage is over the limit. To fix this issue, try to increase the cgroup CPU limit.

This alert is triggered in warning state when the average CPU utilization is between 75-80% and in critical state when it is between 85-95%.