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
System Requirements |  |
External Dependencies |  nginx with configured 'ngx_http_stub_status_module' |

## Introduction

Retrieves connection and request information from the nging servers configured in python.d/nginx.conf, using the [nginx http stub_status module](http://nginx.org/en/docs/http/ngx_http_stub_status_module.html)

## Charts

_For explanation of the columns in the table below, see the [CHART output documentation](../../plugins.d/#CHART)_

title | units | family | context |
:-----|:------|:-------|:--------|
nginx Active Connections | connections | active connections | nginx.connections |
nginx Requests | requests/s | requests | nginx.requests | 
nginx Active Connections by Status | connections | status | nginx.connection_status |
nginx Connections Rate | connections/s | connections rate | nginx.connect_rate |


## Alarms

The only preconfigured alarm makes sure nginx is running.

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
