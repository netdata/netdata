# Bind9 collector

[`Bind9`](https://www.isc.org/bind/) (or named) is a very flexible, full-featured DNS system.

This module will monitor one or more `Bind9` servers, depending on your configuration.

## Requirements

- `bind` version 9.9+ with configured `statistics-channels`

For detail information on how to get your bind installation ready, please refer to the following articles:

- [bind statistics channel developer comments](http://jpmens.net/2013/03/18/json-in-bind-9-s-statistics-server/)
- [bind documentation](https://ftp.isc.org/isc/bind/9.10.3/doc/arm/Bv9ARM.ch06.html#statistics)
- [bind Knowledge Base article AA-01123](https://kb.isc.org/article/AA-01123/0).

Normally, you will need something like this in your `named.conf.options`:

```
statistics-channels {
        inet 127.0.0.1 port 8653 allow { 127.0.0.1; };
        inet ::1 port 8653 allow { ::1; };
};
```

## Charts

It produces the following charts:

- Global Received Requests by IP version (IPv4, IPv6) in `requests/s`
- Global Successful Queries in `queries/s`
- Global Recursive Clients in `clients`
- Global Queries by IP Protocol (TCP, UDP) in `queries/s`
- Global Queries Analysis in `queries/s`
- Global Received Updates in `updates/s`
- Global Query Failures in `failures/s`
- Global Query Failures Analysis in `failures/s`
- Global Server Statistics in `operations/s`
- Global Incoming Requests by OpCode in `requests/s`
- Global Incoming Requests by Query Type in `requests/s`

Per View Statistics (the following set will be added for each bind view):

- Resolver Active Queries in `queries`
- Resolver Statistics in `operations/s`
- Resolver Round Trip Time in `queries/s`
- Resolver Requests by Query Type in `requests/s`
- Resolver Cache Hits in `operations/s`

## Configuration

Edit the `go.d/bind.conf` configuration file using `edit-config` from the
Netdata [config directory](/docs/netdata-agent/configuration/README.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata # Replace this path with your Netdata config directory
sudo ./edit-config go.d/bind.conf
```

Needs only `url`. Here is an example for several servers:

```yaml
jobs:
  - name: local
    url: http://127.0.0.1:8653/json/v1

  - name: local
    url: http://127.0.0.1:8653/xml/v3

  - name: remote
    url: http://203.0.113.10:8653/xml/v3

  - name: local_with_views
    url: http://127.0.0.1:8653/json/v1
    permit_view: '!_* *'
```

View filter syntax: [simple patterns](https://docs.netdata.cloud/libnetdata/simple_pattern/).

For all available options please see
module [configuration file](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/config/go.d/bind.conf).

## Troubleshooting

To troubleshoot issues with the `bind` collector, run the `go.d.plugin` with the debug option enabled. The output should
give you clues as to why the collector isn't working.

- Navigate to the `plugins.d` directory, usually at `/usr/libexec/netdata/plugins.d/`. If that's not the case on
  your system, open `netdata.conf` and look for the `plugins` setting under `[directories]`.

  ```bash
  cd /usr/libexec/netdata/plugins.d/
  ```

- Switch to the `netdata` user.

  ```bash
  sudo -u netdata -s
  ```

- Run the `go.d.plugin` to debug the collector:

  ```bash
  ./go.d.plugin -d -m bind
  ```


