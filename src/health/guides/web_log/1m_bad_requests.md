### Understand the alert

This alert is triggered when the ratio of client error HTTP requests (4xx class status codes, excluding 401) within the last minute is higher than normal. Client errors indicate that the issue is on the client's side, such as incorrect requests or invalid URLs.

### Troubleshoot the alert

1. **Analyze response codes**: Identify the specific HTTP response codes your web server is sending to clients. Use the Netdata dashboard and inspect the `detailed_response_codes` chart for your web server to track the error codes being sent.

2. **Check server logs**: Review the web server logs (e.g., access.log and error.log) to identify any issues, patterns, or errors causing the increase in client errors. These logs can typically be found under `/var/log/{nginx, apache2}/{access.log, error.log}`.

3. **Verify application behavior**: Check the behavior of applications running on your web server to ensure they are not generating incorrect URLs or causing issues with client requests.

4. **Identify broken links**: If there is a high number of 404 errors, use a broken link checker tool to identify and fix any dead links on your website or other websites that redirect to your website.

5. **Monitor server performance**: Keep an eye on the web server's performance metrics to ensure that changes in client errors do not negatively impact server performance or resource usage.

### Useful resources

1. [RFC 2616 - HTTP/1.1 Status Code Definitions](https://datatracker.ietf.org/doc/html/rfc2616#section-10.4)
2. [Mozilla - HTTP Status Codes - Client Error Responses](https://developer.mozilla.org/en-US/docs/Web/HTTP/Status#client_error_responses)
3. [Broken Link Checker Tools](https://www.google.com/search?q=broken+link+checker)
