# Monitor web server log files with Netdata

Log files have been a critical resource for developers and system administrators who want to understand the health and
performance of their web servers, and Netdata is taking important steps to make them even more valuable.

By parsing web server log files with Netdata, and seeing the volume of redirects, requests, or server errors over time,
you can better understand what's happening on your infrastructure. Too many bad requests? Maybe a recent deploy missed a
few small SVG icons. Too many requsests? Time to batten down the hatches—it's a DDoS.

Netdata has been capable of monitoring web log files for quite some time, thanks for the [weblog python.d
module](../../collectors/python.d.plugin/web_log/README.md), but we recently refactored this module in Go, and that
effort comes with a ton of improvements.

You can now use the [ltsv log format](http://ltsv.org/), track TLS and cipher usage, and the whole parser is faster than
ever. In one test on a system with SSD storage, the collector consistently parsed the logs for 200,000 requests in
200ms, using ~30% of a single core. To learn more about these improvements, see our [v1.19 release post](link).

The [go.d plugin](https://github.com/netdata/go.d.plugin/tree/master/modules/weblog) is currently compatible with
[Nginx](https://nginx.org/en/), [Apache](https://httpd.apache.org/), and [Gunicorn](https://gunicorn.org/) logs.

This tutorial will walk you through using the new Go-based web log collector to turn the logs these web servers
constantly write to into real-time insights into your infrastructure.

## Set up your web servers

As with all data sources, Netdata can auto-detect Nginx, Apache, or Gunicorn servers if you installed them using their
standard installation procedures.

Almost all web server installations will need _no_ configuration to start collecting metrics.

By default, Netdata looks for the access logs in the default location for that web server and distribution combination.
For example, on a Debian system running Apache, Netdata will look for a file at `/var/log/apache2/access.log`. On an
Arch system, however, it looks at `/var/log/httpd/access_log`.

As long as your web server has readable access log file, you can configure the weblog plugin to access and parse it.

## Configure the web_log collector module



## Tweak web_log collector alarms

Over time, we've created some default alarms for web log monitoring. These alarms are designed to work only when there is enough data 

-   [web log alarms](https://raw.githubusercontent.com/netdata/netdata/master/health/health.d/web_log.conf).

You can also edit this file directly with `edit-config`:

```bash
cd /etc/netdata/ # Replace with your Netdata configuration directory, if not /etc/netdata/
./edit-config health.d/weblog.conf
```

For more information about editing the defaults or writing new alarm entities, see our [health monitoring
documentation](../../health/README.md).

## What's next?



Don't forget to give GitHub user [Wing924](https://github.com/Wing924) a big 👍 for his hard work in starting up the Go
refactoring effort.