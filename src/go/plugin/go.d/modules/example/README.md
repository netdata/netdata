<!--
title: "Example module"
description: "Use this example data collection module, which produces example charts with random values, to better understand how to build your own collector in Go."
custom_edit_url: "https://github.com/netdata/go.d.plugin/edit/master/modules/example/README.md"
sidebar_label: "Example module in Go"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/Mock Collectors"
-->

# Example module

An example data collection module. Use it as an example writing a new module.

## Charts

This module produces example charts with random values. Number of charts, dimensions and chart type is configurable.

## Configuration

Edit the `go.d/example.conf` configuration file using `edit-config` from the
Netdata [config directory](/docs/netdata-agent/configuration/README.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata # Replace this path with your Netdata config directory
sudo ./edit-config go.d/example.conf
```

Disabled by default. Should be explicitly enabled
in [go.d.conf](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/config/go.d.conf).

```yaml
# go.d.conf
modules:
  example: yes
```

Here is an example configuration with several jobs:

```yaml
jobs:
  - name: example
    charts:
      num: 3
      dimensions: 5

  - name: hidden_example
    hidden_charts:
      num: 3
      dimensions: 5
```

---

For all available options, see the Example
collector's [configuration file](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/config/go.d/example.conf).

## Troubleshooting

To troubleshoot issues with the `example` collector, run the `go.d.plugin` with the debug option enabled. The output
should give you clues as to why the collector isn't working.

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
  ./go.d.plugin -d -m example
  ```
