### Understand the alert

HTTP response status codes indicate whether a specific HTTP request has been successfully completed or not.

The Netdata Agent calculates the ratio of successful HTTP requests over the last minute. These requests consist of 1xx, 2xx, 304, 401 response codes. You receive this alert in warning when the percentage of successful requests is less than 85% and in critical when it is below 75%. This alert can indicate:

- A malfunction in the services of your web server
- Malicious activity towards your website
- Broken links towards your servers.

In most cases, Netdata will send you another alert indicating high incidences of "abnormal" HTTP requests code, for example you could also receive the `web_log_1m_bad_requests` alert.

### Troubleshoot the alert

There are a number of reasons triggering this alert. All of them could eventually cause bad user experience with your web services.

Identify exactly what HTTP response code your web server sent back to your clients. 

Open the Netdata dashboard and inspect the `detailed_response_codes` chart for your web server. This chart keeps track of exactly what error codes your web server sends out.

### Useful resources

1. [HTTP status codes on Mozilla](https://developer.mozilla.org/en-US/docs/Web/HTTP/Status)