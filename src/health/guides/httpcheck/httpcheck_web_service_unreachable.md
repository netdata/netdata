### Understand the alert

The Netdata Agent monitors your HTTP endpoints. You can specify endpoints the Agent will monitor in the Agent's Go module under `go.d/httpcheck.conf`.

If your system fails to connect to your endpoint, or if the request to that endpoint times out, then the Agent will mark the requests and log them as "unreachable".

The Netdata Agent calculates the ratio of these requests over the last 5 minutes. This alert is escalated to warning when the ratio is greater than 10% and then raised to critical when it is greater than 40%.

### Troubleshoot the alert

To troubleshoot this error, check the following:

- Verify that your system has access to the particular endpoint.
  
    - Check for basic connectivity to known hosts.
    - Make sure that requests and replies both to and from the endpoint are allowed in the firewall settings. Ensure they're allowed on both your end as well as the endpoint's side.

- Verify that your DNS can resolve endpoints. 
    - Check your current DNS (for example in linux you can use the host command):
      
      ```
      host -v <your_endpoint>
      ```
  
    - If the HTTP endpoint is suppose to be public facing endpoint, try an alternative DNS (for example Cloudflare's DNS):
  
      ```
      host -v <your_endpoint> 1.1.1.1
      ```

### Useful resources

1. [HTTP endpoint monitoring with Netdata](/src/go/plugin/go.d/collector/httpcheck/integrations/http_endpoints.md)