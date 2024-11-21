### Understand the alert

This alert monitors the average number of successful HTTP requests per second, over the last 5 minutes (`web_log.type_requests`). If you receive this alert, it means that there has been a significant change in the number of successful HTTP requests to your web server.

### What does successful HTTP request mean?

A successful HTTP request is one that receives a response with an HTTP status code in the range of `200-299`. In other words, these requests have been processed correctly by the web server and returned the expected results to the client.

### Troubleshoot the alert

1. Check your web server logs

   Inspect your web server logs for any abnormal activity or issues that might have led to increased or decreased successful HTTP requests. Depending on your web server (e.g., Apache, Nginx), the location of the logs will vary.

2. Analyze the type of requests

   Check the logs for request types (e.g., GET, POST, PUT, DELETE) and their corresponding distribution during the time of the alert. This might help you identify a pattern or source of the issue.

3. Monitor web server resources

   Use monitoring tools like `top`, `htop`, or `glances` to check the resource usage of your web server during the alert period. High resource usage may indicate that your server is struggling to handle the load, causing an abnormal number of successful HTTP requests.

4. Verify client connections

   Investigate the IP addresses and user Agents that are making a significant number of requests during the alert period. If there's a spike in requests from a single or a few IPs, it could be a sign of a coordinated attack, excessive crawling, or other unexpected behavior.

5. Check your web application

   Make sure that your web application is functioning well and generating the expected response for clients, which can impact successful HTTP requests.

### Useful resources

1. [Apache Log Files](https://httpd.apache.org/docs/current/logs.html)
2. [Nginx Log Files](https://nginx.org/en/docs/ngx_core_module.html#error_log)