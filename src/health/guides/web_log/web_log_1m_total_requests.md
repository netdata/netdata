### Understand the alert

This alert calculates the total number of HTTP requests received by the web server in the last minute. If you receive this alert, it means that your web server is experiencing an increase in workload, which might affect its performance or availability.

### What does an increase in workload mean?

An increase in workload means that your web server is handling more traffic than usual, or there might be an unexpected spike in the number of HTTP requests received. This might be because of a variety of reasons, like marketing campaigns, product promotions, or even a sudden surge in user demand.

### Troubleshoot the alert

1. Analyze web traffic logs

   To understand the reason behind the increased workload, the first step is to analyze the web server traffic logs. Look for any patterns, specific time intervals, or specific user Agents that are contributing to the high number of requests.

2. Check the web server performance

   Monitoring web server performance metrics like CPU usage, memory usage, and disk space can provide insight into the resource utilization. Use tools like `top`, `vmstat`, `iostat`, and `free` for this assessment.

3. Monitor response times

   Checking the response time statistics, like average response time and peak response time, can help to understand if the server is struggling to serve the high number of requests. Tools like `apachetop` or `logstash` can be used to track this information.

4. Evaluate server scaling options

   If none of the previous steps help to identify or resolve the issue, it might be time to consider scaling options. If the server is unable to handle the increased workload, vertically or horizontally scaling the system can help.

5. Investigate application-level issues

   Application-level issues might also be the reason for high web server traffic. Profiling the web application, checking for slow database queries, or inefficient scripts can help to identify and resolve performance issues.

### Useful resources

1. [Vertically or Horizontally Scaling Your Web Server](https://www.digitalocean.com/community/tutorials/5-common-server-setups-for-your-web-application)