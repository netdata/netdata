# Monitor Nginx or Apache web server log files with Netdata

Log files have been a critical resource for developers and system administrators who want to understand the health and
performance of their web servers, and Netdata is taking important steps to make them even more valuable.

By parsing web server log files with Netdata, and seeing the volume of redirects, requests, or server errors over time,
you can better understand what's happening on your infrastructure. Too many bad requests? Maybe a recent deploy missed a
few small SVG icons. Too many requsests? Time to batten down the hatches—it's a DDoS.

Netdata has been capable of monitoring web log files for quite some time, thanks for the [weblog python.d
module](../../collectors/python.d.plugin/web_log/README.md), but we recently refactored this module in Go, and that
effort comes with a ton of improvements.

You can now use the [LTSV log format](http://ltsv.org/), track TLS and cipher usage, and the whole parser is faster than
ever. In one test on a system with SSD storage, the collector consistently parsed the logs for 200,000 requests in
200ms, using ~30% of a single core. To learn more about these improvements, see our [v1.19 release post](https://blog.netdata.cloud/posts/release-1.19/).

The [go.d plugin](https://github.com/netdata/go.d.plugin/tree/master/modules/weblog) is currently compatible with
[Nginx](https://nginx.org/en/) and [Apache](https://httpd.apache.org/).

This tutorial will walk you through using the new Go-based web log collector to turn the logs these web servers
constantly write to into real-time insights into your infrastructure.

## Set up your web servers

As with all data sources, Netdata can auto-detect Nginx or Apache servers if you installed them using their standard
installation procedures.

Almost all web server installations will need _no_ configuration to start collecting metrics. As long as your web server
has readable access log file, you can configure the web log plugin to access and parse it.

## Configure the web log collector

To use the Go version of this plugin, you need to explicitly enable it, and disable the deprecated Python version.
First, open `python.d.conf`:

```bash
cd /etc/netdata/ # Replace with your Netdata configuration directory, if not /etc/netdata/
./edit-config python.d.conf
```

Find the `web_log` line, uncomment it, and set it to `web_log: no`. Next, open the `go.d.conf` file for editing.

```bash
./edit-config go.d.conf
```

Find the `web_log` line again, uncomment it, and set it to `web_log: yes`.

Finally, restart Netdata with `service netdata restart`, or the appropriate method for your system. You should see
metrics in your Netdata dashboard!

![Example of real-time web server log metrics in Netdata's
dashboard](https://user-images.githubusercontent.com/1153921/69448130-2980c280-0d15-11ea-9fa5-6dcff25a92c3.png)

If you don't see web log charts, or **web log nginx**/**web log apache** menus on the right-hand side of your dashboard,
continue reading for other configuration options.

## Custom configuration of the web log collector

The web log collector's default configuration comes with a few example jobs that should cover most Linux distributions
and their default locations for log files:

```yaml
# [ JOBS ]
jobs:
# NGINX
# debian, arch
  - name: nginx
    path: /var/log/nginx/access.log

# gentoo
  - name: nginx
    path: /var/log/nginx/localhost.access_log

# APACHE
# debian
  - name: apache
    path: /var/log/apache2/access.log

# gentoo
  - name: apache
    path: /var/log/apache2/access_log

# arch
  - name: apache
    path: /var/log/httpd/access_log

# debian
  - name: apache_vhosts
    path: /var/log/apache2/other_vhosts_access.log

# GUNICORN
  - name: gunicorn
    path: /var/log/gunicorn/access.log

  - name: gunicorn
    path: /var/log/gunicorn/gunicorn-access.log
```

However, if your log files were not auto-detected, it might be because they are in a different location. Try the default
`weblog.conf` file.

```bash
./edit-config go.d/weblog.conf
```

To create a new custom configuration, you need to set the `path` parameter to point to your web server's access log
file. You can give it a `name` as well, and set the `log_type` to `auto`.

```yaml
jobs:
  - name: example
    path: /path/to/file.log
    log_type: auto
```

Restart Netdata with `service netdata restart` or the appropriate method for your system. Netdata should pick up your
web server's access log and begin showing real-time charts!

### Custom log formats and fields

The web log collector is capable of parsing custom Nginx and Apache log formats and presenting them as charts, but we'll
leave that topic for a separate tutorial.

We do have [extensive documentation](../../collectors/go.d.plugin/modules/weblog/#custom-log-format) on how to build
custom parsing for Nginx and Apache logs.

## Tweak web log collector alarms

Over time, we've created some default alarms for web log monitoring. These alarms are designed to work only when your
web server is receiving more than 120 requests per minute. Otherwise, there's simply not enough data to make conclusions
about what is "too few" or "too many."

-   [web log alarms](https://raw.githubusercontent.com/netdata/netdata/master/health/health.d/web_log.conf).

You can also edit this file directly with `edit-config`:

```bash
./edit-config health.d/weblog.conf
```

For more information about editing the defaults or writing new alarm entities, see our [health monitoring
documentation](../../health/README.md).

## What's next?

Now that you have web log collection up and running, we recommend you take a look at the documentation for our
[python.d](../../collectors/python.d.plugin/web_log/README.md) for some ideas of how you can turn these rather "boring"
logs into powerful real-time tools for keeping your servers happy.

Don't forget to give GitHub user [Wing924](https://github.com/Wing924) a big 👍 for his hard work in starting up the Go
refactoring effort.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Ftutorials%2Fweb-log-collector&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
