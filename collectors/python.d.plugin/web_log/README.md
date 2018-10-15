# web_log

Tails the apache/nginx/lighttpd/gunicorn log files to collect real-time web-server statistics.

It produces following charts:

1. **Response by type** requests/s
 * success (1xx, 2xx, 304)
 * error (5xx)
 * redirect (3xx except 304)
 * bad (4xx)
 * other (all other responses)

2. **Response by code family** requests/s
 * 1xx (informational)
 * 2xx (successful)
 * 3xx (redirect)
 * 4xx (bad)
 * 5xx (internal server errors)
 * other (non-standart responses)
 * unmatched (the lines in the log file that are not matched)

3. **Detailed Response Codes** requests/s (number of responses for each response code family individually)

4. **Bandwidth** KB/s
 * received (bandwidth of requests)
 * send (bandwidth of responses)

5. **Timings** ms (request processing time)
 * min (bandwidth of requests)
 * max (bandwidth of responses)
 * average (bandwidth of responses)

6. **Request per url** requests/s (configured by user)

7. **Http Methods** requests/s (requests per http method)

8. **Http Versions** requests/s (requests per http version)

9. **IP protocols** requests/s (requests per ip protocol version)

10. **Current Poll Unique Client IPs** unique ips/s (unique client IPs per data collection iteration)

11. **All Time Unique Client IPs** unique ips/s (unique client IPs since the last restart of netdata)


### configuration

```yaml
nginx_log:
  name  : 'nginx_log'
  path  : '/var/log/nginx/access.log'

apache_log:
  name  : 'apache_log'
  path  : '/var/log/apache/other_vhosts_access.log'
  categories:
      cacti : 'cacti.*'
      observium : 'observium'
```

Module has preconfigured jobs for nginx, apache and gunicorn on various distros.

---
