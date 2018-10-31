# [Collector Module Name]

Parameter | Value |
:---------|:------|
Short Description | This module will monitor one or more nginx servers depending on configuration. Servers can be either local or remote. |
Category | Web |
Sub-Category | nginx | 
Modular Plugin | python.d.plugin |
Module  | nginx/nginx.chart.py |
Prog. Language | Python | 
Config file | python.d/nginx.conf |
Alarms config | health.d/nginx.conf |
Dependencies |  nginx with configured 'ngx_http_stub_status_module' |
Live Demo | [See it live](https://singapore.my-netdata.io/#menu_nginx_local) |

_The README always starts with the table above after the main header. Any other content in this section will not be present in the documentation.
- Category is one of the following: Web, Cloud, Data Store, Messaging (Queues), Monitoring Tools, Operating System, Application Instrumentation, Other
- Sub-Category could be Application, Metric type (e.g.) 
- Modular Plugin is usually one of charts.d.plugin, node.d.plugin, python.d.plugin
- Live Demo: If a full URL appears here, we will show it as is and also parse the path after the hash tag. If it's not a link format, we'll assume that we are given the path after the hash tag.
_

## Introduction

_This can contain subsections and will appear as is. It should give a high level explanation of what is monitored and how, without going into extreme details._

Retrieves connection and request information from the nging servers configured in python.d/nginx.conf, using the [nginx http stub_status module](http://nginx.org/en/docs/http/ngx_http_stub_status_module.html)

## Charts

_Detailed list of charts generated._

type.id | name | title | units | family | context | charttype | options |
:-------|:-----|:------|:------|:-------|:--------|:----------|:--------|
nginx_local.connections | None | nginx Active Connections | connections | active connections | nginx.connections | line | lines: active |
nginx_local.requests | None | nginx Requests | requests/s | requests | nginx.requests | line | requests ((incremental)) | |
nginx_local.connection_status | None | nginx Active Connections by Status | connections | status | nginx.connection_status | line | lines: reading, writing, waiting/idle |
nginx_local.connect_rate | None | nginx Connections Rate | connections/s | connections rate','nginx.connect_rate | line | lines: accepts, accepted ((incremental)), handled ((incremental)) |


## Alarms

_Preconfigured alarms, or suggestions for alarms_

The only preconfigured alarm makes sure nginx is running:

```yaml
template: nginx_last_collected_secs
      on: nginx.requests
    calc: $now - $last_collected_t
   units: seconds ago
   every: 10s
    warn: $this > (($status >= $WARNING)  ? ($update_every) : ( 5 * $update_every))
    crit: $this > (($status == $CRITICAL) ? ($update_every) : (60 * $update_every))
   delay: down 5m multiplier 1.5 max 1h
    info: number of seconds since the last successful data collection
      to: webmaster
```

## Installation

_e.g. do I need to install an external application, do I need to configure my application so it can be monitored by netdata, do I need to restart netdata etc._

nginx should have the [ngx_http_stub_status_module](http://nginx.org/en/docs/http/ngx_http_stub_status_module.html) configured. 

## Configuration

_Here we explain what we put in the default config and what other config options the plugin supports. We can also add suggestions for custom charts for the specific plugin_

Needs only `url` to server's `stub_status`

Here is an example for local server:

```yaml
update_every : 10
priority     : 90100

local:
  url     : 'http://localhost/stub_status'
  retries : 10
```

Without configuration, the plugin attempts to connect to `http://localhost/stub_status`

## Usage

_Anything specific that we want to say about what we see in the charts, disclaimers, use cases in which the data may be misinterpreted etc. May include screenshots of charts._

## Notes

_Optional section, any additional info that does not fit in the categories above_
