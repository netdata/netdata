# nginx module

Parameter | Value |
:---------|:------|
Short Description | This module will monitor one or more nginx servers depending on configuration. Servers can be either local or remote. |
Category | Web |
Sub-Category | nginx | 
Orchestrator | python.d.plugin |
Module  | nginx/nginx.chart.py |
Prog. Language | Python | 
Config file | python.d/nginx.conf |
Alarms config | health.d/nginx.conf |
Dependencies |  nginx with configured 'ngx_http_stub_status_module' |
Live Demo | [See it live](https://singapore.my-netdata.io/#menu_nginx_local) |


## Introduction

Retrieves connection and request information from the nging servers configured in python.d/nginx.conf, using the [nginx http stub_status module](http://nginx.org/en/docs/http/ngx_http_stub_status_module.html)

## Charts

type.id | name | title | units | family | context | charttype | options |
:-------|:-----|:------|:------|:-------|:--------|:----------|:--------|
nginx_local.connections | None | nginx Active Connections | connections | active connections | nginx.connections | line | lines: active |
nginx_local.requests | None | nginx Requests | requests/s | requests | nginx.requests | line | requests ((incremental)) | |
nginx_local.connection_status | None | nginx Active Connections by Status | connections | status | nginx.connection_status | line | lines: reading, writing, waiting/idle |
nginx_local.connect_rate | None | nginx Connections Rate | connections/s | connections rate','nginx.connect_rate | line | lines: accepts, accepted ((incremental)), handled ((incremental)) |


## Alarms

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

nginx should have the [ngx_http_stub_status_module](http://nginx.org/en/docs/http/ngx_http_stub_status_module.html) configured. 

## Configuration

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

## Notes
