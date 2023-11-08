### Understand the alert

This alert presents the average backlog size of the disk raising this alarm over the last 10 minutes.

This alert is escalated to warning when the metric exceeds the size of 5000.

### What is "disk backlog"?

Backlog is an indication of the duration of pending disk operations. On every I/O event the system is multiplying the time spent doing I/O since the last update of this field with the number of pending operations. While not accurate, this metric can provide an indication of the expected completion time of the operations in progress.

