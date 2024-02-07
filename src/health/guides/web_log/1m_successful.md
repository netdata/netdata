### Understand the alert

This alert is triggered when the percentage of successful HTTP requests (1xx, 2xx, 304, 401 response codes) within the last minute falls below a certain threshold. A warning state occurs when the success rate is below 85%, and a critical state occurs when it falls below 75%. This alert can indicate a malfunction in your web server's services, malicious activity towards your website, or broken links.

### Troubleshoot the alert

1. **Analyze response codes**: Identify the specific HTTP response codes your web server is sending to clients. Use the Netdata dashboard and inspect the `detailed_response_codes` chart for your web server to track the error codes being sent.

2. **Check server logs**: Review the web server logs to identify any issues, patterns, or errors causing the decrease in successful requests. Investigate any unusual or unexpected response codes.

3. **Inspect application logs**: Check the logs of applications running on your web server for any errors or issues that might be affecting the success rate of HTTP requests.

4. **Verify server resources**: Ensure your server has adequate resources (CPU, RAM, disk space) to handle the workload, as resource limitations can impact the success rate of HTTP requests.

5. **Review server configuration**: Check your web server's configuration for any misconfigurations, incorrect permissions, or improper settings that may be causing the issue.

6. **Monitor security**: Look for signs of malicious activity, such as a high number of requests from a specific IP address or a sudden spike in requests. Implement security measures, such as rate limiting, IP blocking, or Web Application Firewalls (WAF), if necessary.

### Useful resources

1. [HTTP status codes on Mozilla](https://developer.mozilla.org/en-US/docs/Web/HTTP/Status)
2. [Apache HTTP Server Documentation](https://httpd.apache.org/docs/)
3. [Nginx Documentation](https://nginx.org/en/docs/)
