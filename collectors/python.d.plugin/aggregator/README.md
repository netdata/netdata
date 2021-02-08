<!--
title: "aggregator"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/aggregator/README.md
-->

# Aggregator

This collector 'aggregates' charts from multiple children that are streaming to a parent node. 

## Charts

You should see charts similar to those you have configured to aggregate. For example in the below chart underneath the "Aggregator devml" context we see the "system cpu" chart with is just the aggregation of the system.cpu chart over the specified children nodes (in this case two children called 'devml' and 'devml2' as defined in the configuration example below). 

![netdata-aggregator-collector](https://user-images.githubusercontent.com/2178292/107240701-1d6c4800-6a22-11eb-8b89-0a9cfff853dd.jpg)

## Configuration

Enable the collector, define the configuration and restart Netdata.

Note: given the nature of this collector you will have to edit the `aggregator.conf` file to define your own specific configuration. Obviously this collector can not know in advance what specific children and charts you want to aggregate over and so will fail if you dont configure `aggregator.conf`.

### Enable

```bash
cd /etc/netdata/
sudo ./edit-config python.d.conf
# Set `aggregator: no` to `aggregator: yes`
```

### Configure

Edit the `python.d/aggregator.conf` configuration file using `edit-config` from the your agent's [config directory](/docs/configure/nodes.md), which is usually at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/aggregator.conf
```

Below is an example of a configuration where we are aggregating across two child nodes with the string "devml" in their name and we want to exclude the parent which is called "devml-master" and so also happens to have "devml" in its name, hence we explicitly exclude it by setting `child_not_contains='devml-master'`. Be then manually list most of the main "System Overview" charts as its those we want to aggregate in this example. 

```bash

# give the aggregation job a meaningful name.
devml:
    name: 'devml'
    # the parent we want to pull the data to aggregate from.
    parent: '127.0.0.1:19999'
    # a string "," separated list of terms we want to use to select which children to aggregate.
    # for example 'term1,term2' would aggregate all children on the parent that have 'term1' or 'term2' in their name
    child_contains: 'devml'
    # a string "," separated list of terms we want to exclude from any match.
    # for example 'term2,term3' would filter from the children matched by the terms 
    # in child_contains and remove any that have 'term3' or 'term4 in their name.
    child_not_contains: 'devml-master'
    # a list of charts to aggregate.
    charts_to_agg:
      # chart name.
      - name: 'system.cpu'
        # optional aggregation function can be 'mean', 'min', 'max', 'sum' ('mean' is default).
        agg_func: 'mean'
        # a "," separated string list of specific dimensions to exclude.
        exclude_dims: 'idle'
      - name: 'system.cpu_pressure'
      - name: 'system.load'
      - name: 'system.io'
      - name: 'system.pgpgio'
      - name: 'system.io_some_pressure'
      - name: 'system.io_full_pressure'
      - name: 'system.ram'
      - name: 'system.memory_some_pressure'
      - name: 'system.memory_full_pressure'
      - name: 'system.net'
      - name: 'system.ip'
      - name: 'system.ipv6'
      - name: 'system.processes'
      - name: 'system.forks'
      - name: 'system.active_processes'
      - name: 'system.ctxt'
      - name: 'system.idlejitter'
      - name: 'system.intr'
      - name: 'system.interrupts'
      - name: 'system.softirqs'
      - name: 'system.softnet_stat'
      - name: 'system.entropy'
      - name: 'system.uptime'
      - name: 'system.ipc_semaphores'
      - name: 'system.ipc_semaphore_arrays'
      - name: 'system.shared_memory_segments'
      - name: 'system.shared_memory_bytes'
```

### Restart Netdata

Now restart netdata for the changes to take effect. 

```bash
sudo systemctl restart netdata
```

## Troubleshooting

To see any relevant log messages you can use a command like below.

```bash
grep 'aggregator' /var/log/netdata/error.log
```

If you would like to log in as `netdata` user and run the collector in debug mode to see more detail.

```bash
# become netdata user
sudo su -s /bin/bash netdata
# run collector in debug using `nolock` option if netdata is already running the collector itself.
/usr/libexec/netdata/plugins.d/python.d.plugin aggregator debug trace nolock
```
