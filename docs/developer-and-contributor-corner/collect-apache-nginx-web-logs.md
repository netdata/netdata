# Monitor Nginx or Apache web server log files

Parsing web server log files with Netdata, revealing the volume of redirects, requests and other metrics, can give you a better overview of your infrastructure.

Too many bad requests? Maybe a recent deploy missed a few small SVG icons. Too many requests? Time to batten down the hatches—it's a DDoS.

You can use the [LTSV log format](http://ltsv.org/), track TLS and cipher usage, and the whole parser is faster than
ever. In one test on a system with SSD storage, the collector consistently parsed the logs for 200,000 requests in
200ms, using ~30% of a single core.

The [web_log](https://github.com/netdata/netdata/blob/master/src/go/collectors/go.d.plugin/modules/weblog/README.md) collector is currently compatible
with [Nginx](https://nginx.org/en/) and [Apache](https://httpd.apache.org/).

This guide will walk you through using the new Go-based web log collector to turn the logs these web servers
constantly write to into real-time insights into your infrastructure.

## Set up your web servers

As with all data sources, Netdata can auto-detect Nginx or Apache servers if you installed them using their standard
installation procedures.

Almost all web server installations will need _no_ configuration to start collecting metrics. As long as your web server
has readable access log file, you can configure the web log plugin to access and parse it.

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
`web_log.conf` file.

```bash
./edit-config go.d/web_log.conf
```

To create a new custom configuration, you need to set the `path` parameter to point to your web server's access log
file. You can give it a `name` as well, and set the `log_type` to `auto`.

```yaml
jobs:
  - name: example
    path: /path/to/file.log
    log_type: auto
```

Restart Netdata with `sudo systemctl restart netdata`, or the [appropriate
method](https://github.com/netdata/netdata/blob/master/packaging/installer/README.md#maintaining-a-netdata-agent-installation) for your system. Netdata should pick up your web server's access log and
begin showing real-time charts!

### Custom log formats and fields

The web log collector is capable of parsing custom Nginx and Apache log formats and presenting them as charts, but we'll
leave that topic for a separate guide.

We do have [extensive
documentation](https://github.com/netdata/netdata/blob/master/src/go/collectors/go.d.plugin/modules/weblog/README.md#custom-log-format) on how
to build custom parsing for Nginx and Apache logs.

## Tweak web log collector alerts

Over time, we've created some default alerts for web log monitoring. These alerts are designed to work only when your
web server is receiving more than 120 requests per minute. Otherwise, there's simply not enough data to make conclusions
about what is "too few" or "too many."

-   [web log alerts](https://raw.githubusercontent.com/netdata/netdata/master/src/health/health.d/web_log.conf).

You can also edit this file directly with `edit-config`:

```bash
./edit-config health.d/weblog.conf
```

For more information about editing the defaults or writing new alert entities, see our 
[health monitoring documentation](https://github.com/netdata/netdata/blob/master/src/health/README.md).
