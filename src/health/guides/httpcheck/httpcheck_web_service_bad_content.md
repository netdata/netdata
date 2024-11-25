### Understand the alert

The Netdata Agent monitors your HTTP endpoints. You can specify endpoints that the Agent will monitor in Agent's Go module under `go.d/httpcheck.conf`. You can also specify the expected response pattern. This HTTP endpoint will send in the `response_match` option. If the endpoint's response does not match the `response_match` pattern, then the Agent marks the response as unexpected.

The Netdata Agent calculates the average ratio of HTTP responses with unexpected content over the last 5 minutes.

This alert is escalated to warning if the percentage of unexpected content is greater than 10% and then raised to critical if it is greater than 40%.

### Troubleshoot the alert

Check the actual response and the expected response.

1. Try to implement a request with a verbose result:

```
curl -v <your_http_endpoint>:<port>/<path>
```

2. Compare it with the expected response.

Check your configuration under `go.d/httpcheck.conf`:

```
cd /etc/netdata # Replace this path with your Netdata config directory
sudo ./edit-config go.d/httpcheck.conf
```

### Useful resources

1. [HTTP endpoint monitoring with Netdata](/src/go/plugin/go.d/collector/httpcheck/integrations/http_endpoints.md)