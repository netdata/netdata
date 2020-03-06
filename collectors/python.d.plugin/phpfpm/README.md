# PHP-FPM monitoring with Netdata

Monitors one or more PHP-FPM instances depending on configuration.

## Requirements

-   `PHP-FPM` with [enabled `status` page](https://easyengine.io/tutorials/php/fpm-status-page/)
-   access to `status` page via web server

## Charts

It produces following charts:

-   Active Connections in `connections`
-   Requests in `requests/s`
-   Performance in `status`
-   Requests Duration Among All Idle Processes in `milliseconds`
-   Last Request CPU Usage Among All Idle Processes in `percentage`
-   Last Request Memory Usage Among All Idle Processes in `KB`

## Configuration

Edit the `python.d/phpfpm.conf` configuration file using `edit-config` from the your agent's [config
directory](../../../docs/step-by-step/step-04.md#find-your-netdataconf-file), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/phpfpm.conf
```

Needs only `url` to server's `status`. Here is an example for local and remote instances:

```yaml
local:
  url     : 'http://localhost/status?full&json'

remote:
  url     : 'http://203.0.113.10/status?full&json'
```

Without configuration, module attempts to connect to `http://localhost/status`

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fphpfpm%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
