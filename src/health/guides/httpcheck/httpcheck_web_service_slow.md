### Understand the alert

The Netdata Agent monitors your HTTP endpoints. You can specify endpoints the Agent will monitor in the Agent's Go module under `go.d/httpcheck.conf`.
The Agent calculates the average response time for every HTTP request made to the endpoint being monitored per hour. The Agent also calculates the average response time in a 3-min window.

The Netdata Agent compares these two (average) values. If there is a significant increase in 3-min average, then it will trigger a warning alert when the response time 3-min average is at least twice as much as 1-hour average. The alert will escalate to critical when the response time 3-min average reaches three times the average amount per hour. 

### Troubleshoot the alert

To troubleshoot this issue, check for:

- Network congestion in your system's network and/or in the remote endpoint's network.
- If the endpoint is managed by you, then check the system load.

### Useful resources

1. [HTTP endpoint monitoring with Netdata](/src/go/plugin/go.d/collector/httpcheck/integrations/http_endpoints.md)

