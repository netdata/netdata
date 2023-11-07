# 1m_unmatched

**Web Server | Web log**

In a webserver, all activity should be monitored. By default, most of the webservers log 
activity in an `access.log` file. The access log is a list of all requests for
individual files that people or bots have requested from a website. Log File strings include notes
about their requests for the HTML files and their embedded graphic images, along with any other
associated files that are transmitted.

The Netdata Agent calculates the percentage of unparsed log lines over the last minute. These are
entries in the log file that didn't match in any of the common pattern operations of
the webserver (1XX, 2XX, etc). This can indicate an abnormal activity on your web server, or that your server is
performing operations that you cannot monitor with the Agent.

Web servers like NGINX and Apache2 gives you the ability to modify the log patterns for each request. 
If you have done that, you will also need to adjust the Netdata Agent to parse those patterns.

### Troubleshooting section:

<details>
<summary>Create a custom log format job</summary>

This alert is triggered by the `python.d.plugin`. You must create a new job in the `web_log`
collector for your Agent.

1. See how you can [configure this collector](https://learn.netdata.cloud/docs/agent/collectors/python.d.plugin/web_log#configuration)


2. Follow the job template specified in
the [default web_log.conf file](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/web_log/web_log.conf#L53-L86)
, focus on the
lines [83:85](https://github.com/netdata/netdata/blob/e6d9fbc4a53f1d35363e9b342231bb11627bafbd/collectors/python.d.plugin/web_log/web_log.conf#L83-L85)
where you can see how you define a `custom_log_format`.


3. Restart the Netdata Agent
   ```
   root@netdata # systemctl restart netdata
   ```
</details>


