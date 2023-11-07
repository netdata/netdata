# web_log_5m_requests_ratio

**Web Server | Web log**

HTTP response status codes indicate whether a specific HTTP request has been successfully completed
or not.

The Netdata Agent calculates the ratio of successful HTTP requests over the last 5 minutes, compared
with the previous 5 minutes. These requests consist of 1xx, 2xx, 304, 401 response codes.

There is a warning alert when the percentage of requests is increased more than 25% in the
last 5 minutes compared with the previous 5 minutes and in critical when it increased more than 50%.
A clear notification for this alert **will not be sent** when the ratio becomes normal again.

### Troubleshooting section:

This alert is not always a bad thing. It means that there is a slight increase in the
requests towards to your Web server. You should just keep an eye on this metrics. If you receive
this alert regularly you should consider take action in advance to avoid server overload.

You can find an interesting article on actions you can take
to [manage sudden traffic](https://www.nginx.com/blog/how-to-manage-sudden-traffic-surges-server-overload/)
on a web server (this article is produced by the NGINX associates, but nearly same principles
applied in any case)
