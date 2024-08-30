### Understand the alert

In a webserver, all activity should be monitored. By default, most of the webservers log activity in an `access.log` file. The access log is a list of all requests for individual files that people or bots have requested from a website. Log File strings include notes about their requests for the HTML files and their embedded graphic images, along with any other associated files that are transmitted.

The Netdata Agent calculates the percentage of unparsed log lines over the last minute. These are entries in the log file that didn't match in any of the common pattern operations (1XX, 2XX, etc) of the webserver. This can indicate an abnormal activity on your web server, or that your server is performing operations that you cannot monitor with the Agent.

Web servers like NGINX and Apache2 give you the ability to modify the log patterns for each request. If you have done that, you also need to adjust the Netdata Agent to parse those patterns.

### Troubleshoot the alert

- Create a custom log format job

You must create a new job in the `web_log` collector for your Agent.

1. See how you can [configure this collector](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/weblog#configuration)
