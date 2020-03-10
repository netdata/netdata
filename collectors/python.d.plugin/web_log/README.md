<!--
---
title: "Apache/NGINX/Squid monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/web_log/README.md
---
-->

# Apache/NGINX/Squid monitoring with Netdata

Tails access log file and Collects web server/caching proxy metrics.

## Motivation

Web server log files exist for more than 20 years. All web servers of all kinds, from all vendors, [since the time NCSA httpd was powering the web](https://en.wikipedia.org/wiki/NCSA_HTTPd), produce log files, saving in real-time all accesses to web sites and APIs.

Yet, after the appearance of google analytics and similar services, and the recent rise of APM (Application Performance Monitoring) with sophisticated time-series databases that collect and analyze metrics at the application level, all these web server log files are mostly just filling our disks, rotated every night without any use whatsoever.

Netdata turns this "useless" log file, into a powerful performance and health monitoring tool, capable of detecting, **in real-time**, most common web server problems, such as:

-   too many redirects (i.e. **oops!** *this should not redirect clients to itself*)
-   too many bad requests (i.e. **oops!** *a few files were not uploaded*)
-   too many internal server errors (i.e. **oops!** *this release crashes too much*)
-   unreasonably too many requests (i.e. **oops!** *we are under attack*)
-   unreasonably few requests (i.e. **oops!** *call the network guys*)
-   unreasonably slow responses (i.e. **oops!** *the database is slow again*)
-   too few successful responses (i.e. **oops!** *help us God!*)

## Usage

If Netdata is installed on a system running a web server, it will detect it and it will automatically present a series of charts, with information obtained from the web server API, like these (*these do not come from the web server log file*):

![image](https://cloud.githubusercontent.com/assets/2662304/22900686/e283f636-f237-11e6-93d2-cbdf63de150c.png)
*[**netdata**](https://my-netdata.io/) charts based on metrics collected by querying the `nginx` API (i.e. `/stub_status`).*

> [**netdata**](https://my-netdata.io/) supports `apache`, `nginx`, `lighttpd` and `tomcat`. To obtain real-time information from a web server API, the web server needs to expose it. For directions on configuring your web server, check the config files for each web server. There is a directory with a config file for each web server under [`/etc/netdata/python.d/`](../). 

## Configuration

Edit the `python.d/web_log.conf` configuration file using `edit-config` from the your agent's [config
directory](../../../docs/step-by-step/step-04.md#find-your-netdataconf-file), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/web_log.conf
```

[**netdata**](https://my-netdata.io/) has a powerful `web_log` plugin, capable of incrementally parsing any number of web server log files. This plugin is automatically started with [**netdata**](https://my-netdata.io/) and comes, pre-configured, for finding web server log files on popular distributions. Its configuration is at [`/etc/netdata/python.d/web_log.conf`](web_log.conf), like this:

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

The module has preconfigured jobs for nginx, apache and gunicorn on various distros.
You can add one such section for each of your web server log files.

> **Important**<br/>Keep in mind [**netdata**](https://my-netdata.io/) runs as user `netdata`. So, make sure user `netdata` has access to the logs directory and can read the log file.

## Charts

Once you have all log files configured and [**netdata**](https://my-netdata.io/) restarted, **for each log file** you will get a section at the [**netdata**](https://my-netdata.io/) dashboard, with the following charts.

### Responses by status

In this chart we tried to provide a meaningful status for all responses. So:

-   `success` counts all the valid responses (i.e. `1xx` informational, `2xx` successful and `304` not modified).
-   `error` are `5xx` internal server errors. These are very bad, they mean your web site or API is facing difficulties.
-   `redirect` are `3xx` responses, except `304`. All `3xx` are redirects, but `304` means "not modified" - it tells the browsers the content they already have is still valid and can be used as-is. So, we decided to account it as a successful response.
-   `bad` are bad requests that cannot be served.
-   `other` as all the other, non-standard, types of responses.

![image](https://cloud.githubusercontent.com/assets/2662304/22902194/ea0affc6-f23c-11e6-85f1-a4951dd4bb40.png)

### Responses by type

Then, we group all responses by code family, without interpreting their meaning.
**Response by type** requests/s

-   success (1xx, 2xx, 304)
-   error (5xx)
-   redirect (3xx except 304)
-   bad (4xx)
-   other (all other responses)

![image](https://cloud.githubusercontent.com/assets/2662304/22901883/dea7d33a-f23b-11e6-960d-00a913b58936.png)

### Responses by code family

Here we show all the response codes in detail.

**Response by code family** requests/s

-   1xx (informational)
-   2xx (successful)
-   3xx (redirect)
-   4xx (bad)
-   5xx (internal server errors)
-   other (non-standart responses)
-   unmatched (the lines in the log file that are not matched)

![image](https://cloud.githubusercontent.com/assets/2662304/22901965/1a5d84ba-f23c-11e6-9d38-3deebcc8b879.png)

> **Important**<br/>If your application is using hundreds of non-standard response codes, your browser may become slow while viewing this chart, so we have added a configuration [option to disable this chart](https://github.com/netdata/netdata/blob/419cd0a237275e5eeef3f92dcded84e735ee6c58/conf.d/python.d/web_log.conf#L63).

### Detailed Response Codes

Number of responses for each response code family individually (requests/s)

### Bandwidth

This is a nice view of the traffic the web server is receiving and is sending.

What is important to know for this chart, is that the bandwidth used for each request and response is accounted at the time the log is written. Since [**netdata**](https://my-netdata.io/) refreshes this chart every single second, you may have unrealistic spikes is the size of the requests or responses is too big. The reason is simple: a response may have needed 1 minute to be completed, but all the bandwidth used during that minute for the specific response will be accounted at the second the log line is written.

As the legend on the chart suggests, you can use FireQoS to setup QoS on the web server ports and IPs to accurately measure the bandwidth the web server is using. Actually, [there may be a few more reasons to install QoS on your servers](../../tc.plugin/#tcplugin)...

**Bandwidth** KB/s

-   received (bandwidth of requests)
-   send (bandwidth of responses)

![image](https://cloud.githubusercontent.com/assets/2662304/22902266/245141d6-f23d-11e6-90f9-98729733e0da.png)

> **Important**<br/>Most web servers do not log the request size by default.<br/>So, [unless you have configured your web server to log the size of requests](https://github.com/netdata/netdata/blob/419cd0a237275e5eeef3f92dcded84e735ee6c58/conf.d/python.d/web_log.conf#L76-L89), the `received` dimension will be always zero.

### Timings

[**netdata**](https://my-netdata.io/) will also render the `minimum`, `average` and `maximum` time the web server needed to respond to requests.

Keep in mind most web servers timings start at the reception of the full request, until the dispatch of the last byte of the response. So, they include network latencies of responses, but they do not include network latencies of requests.

**Timings** ms (request processing time)

-   min (bandwidth of requests)
-   max (bandwidth of responses)
-   average (bandwidth of responses)

![image](https://cloud.githubusercontent.com/assets/2662304/22902283/369e3f92-f23d-11e6-9359-53e5d4ecb18e.png)

> **Important**<br/>Most web servers do not log timing information by default.<br/>So, [unless you have configured your web server to also log timings](https://github.com/netdata/netdata/blob/419cd0a237275e5eeef3f92dcded84e735ee6c58/conf.d/python.d/web_log.conf#L76-L89), this chart will not exist.

### URL patterns

This is a very interesting chart. It is configured entirely by you.

[**netdata**](https://my-netdata.io/) can map the URLs found in the log file into categories. You can define these categories, by providing names and regular expressions in `web_log.conf`.

So, this configuration:

```yaml
nginx_netdata:                        # name the charts
  path: '/var/log/nginx/access.log'   # web server log file
  categories:
    badges      : '^/api/v1/badge\.svg'
    charts      : '^/api/v1/(data|chart|charts)'
    registry    : '^/api/v1/registry'
    alarms      : '^/api/v1/alarm'
    allmetrics  : '^/api/v1/allmetrics'
    api_other   : '^/api/'
    netdata_conf: '^/netdata.conf'
    api_old     : '^/(data|datasource|graph|list|all\.json)'
```

Produces the following chart. The `categories` section is matched in the order given. So, pay attention to the order you give your patterns.

![image](https://cloud.githubusercontent.com/assets/2662304/22902302/4d25bf06-f23d-11e6-844d-18c0876bdc3d.png)

### HTTP methods

This chart breaks down requests by HTTP method used.

![image](https://cloud.githubusercontent.com/assets/2662304/22902323/5ee376d4-f23d-11e6-8457-157d3f438843.png)

### IP versions

This one provides requests per IP version used by the clients (`IPv4`, `IPv6`).

![image](https://cloud.githubusercontent.com/assets/2662304/22902370/7091a770-f23d-11e6-8cd2-74e9a67b1397.png)

### Unique clients

The last charts are about the unique IPs accessing your web server.

**Current Poll Unique Client IPs** unique ips/s. This one counts the unique IPs for each data collection iteration (i.e. **unique clients per second**).

![image](https://cloud.githubusercontent.com/assets/2662304/22902384/835aa168-f23d-11e6-914f-cfc3f06eaff8.png)

**All Time Unique Client IPs** unique ips/s. Counts the unique IPs, since the last [**netdata**](https://my-netdata.io/) restart.

![image](https://cloud.githubusercontent.com/assets/2662304/22902407/92dd27e6-f23d-11e6-900d-eede7bc08e64.png)

> **Important**<br/>To provide this information `web_log` plugin keeps in memory all the IPs seen by the web server. Although this does not require so much memory, if you have a web server with several million unique client IPs, we suggest to [disable this chart](https://github.com/netdata/netdata/blob/419cd0a237275e5eeef3f92dcded84e735ee6c58/conf.d/python.d/web_log.conf#L64).

## Alarms

The magic of [**netdata**](https://my-netdata.io/) is that all metrics are collected per second, and all metrics can be used or correlated to provide real-time alarms. Out of the box, [**netdata**](https://my-netdata.io/) automatically attaches the [following alarms](../../../health/health.d/web_log.conf) to all `web_log` charts (i.e. to all log files configured, individually):

| alarm|description|minimum<br/>requests|warning|critical|
|:----|-----------|:------------------:|:-----:|:------:|
| `1m_redirects`|The ratio of HTTP redirects (3xx except 304) over all the requests, during the last minute.<br/> <br/>*Detects if the site or the web API is suffering from too many or circular redirects.*<br/> <br/>(i.e. **oops!** *this should not redirect clients to itself*)|120/min|> 20%|> 30%|
| `1m_bad_requests`|The ratio of HTTP bad requests (4xx) over all the requests, during the last minute.<br/> <br/>*Detects if the site or the web API is receiving too many bad requests, including `404`, not found.*<br/> <br/>(i.e. **oops!** *a few files were not uploaded*)|120/min|> 30%|> 50%|
| `1m_internal_errors`|The ratio of HTTP internal server errors (5xx), over all the requests, during the last minute.<br/> <br/>*Detects if the site is facing difficulties to serve requests.*<br/> <br/>(i.e. **oops!** *this release crashes too much*)|120/min|> 2%|> 5%|
| `5m_requests_ratio`|The percentage of successful web requests of the last 5 minutes, compared with the previous 5 minutes.<br/> <br/>*Detects if the site or the web API is suddenly getting too many or too few requests.*<br/> <br/>(i.e. too many = **oops!** *we are under attack*)<br/>(i.e. too few = **oops!** *call the network guys*)|120/5min|> double or \< half|> 4x or \< 1/4x|
| `web_slow`|The average time to respond to requests, over the last 1 minute, compared to the average of last 10 minutes.<br/> <br/>*Detects if the site or the web API is suddenly a lot slower.*<br/> <br/>(i.e. **oops!** *the database is slow again*)|120/min|> 2x|> 4x|
| `1m_successful`|The ratio of successful HTTP responses (1xx, 2xx, 304) over all the requests, during the last minute.<br/> <br/>*Detects if the site or the web API is performing within limits.*<br/> <br/>(i.e. **oops!** *help us God!*)|120/min|\< 85%|\< 75%|

The column `minimum requests` state the minimum number of requests required for the alarm to be evaluated. We found that when the site is receiving requests above this rate, these alarms are pretty accurate (i.e. no false-positives).

[**netdata**](https://my-netdata.io/) alarms are user configurable. Sample config files can be found under directory `health/health.d` of the [Netdata GitHub repository](https://github.com/netdata/netdata/). So, even [`web_log` alarms can be adapted to your needs](../../../health/health.d/web_log.conf).

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fweb_log%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
