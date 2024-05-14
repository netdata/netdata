<!--
title: "python.d.plugin"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/src/collectors/python.d.plugin/README.md"
sidebar_label: "python.d.plugin"
learn_status: "Published"
learn_topic_type: "Tasks"
learn_rel_path: "Developers/External plugins/python.d.plugin"
-->

# python.d.plugin

`python.d.plugin` is a Netdata external plugin. It is an **orchestrator** for data collection modules written in `python`.

1.  It runs as an independent process `ps fax` shows it
2.  It is started and stopped automatically by Netdata
3.  It communicates with Netdata via a unidirectional pipe (sending data to the `netdata` daemon)
4.  Supports any number of data collection **modules**
5.  Allows each **module** to have one or more data collection **jobs**
6.  Each **job** is collecting one or more metrics from a single data source

## Disclaimer

All third party libraries should be installed system-wide or in `python_modules` directory.
Module configurations are written in YAML and **pyYAML is required**.

Every configuration file must have one of two formats:

-   Configuration for only one job:

```yaml
update_every : 2 # update frequency
priority     : 20000 # where it is shown on dashboard

other_var1   : bla  # variables passed to module
other_var2   : alb
```

-   Configuration for many jobs (ex. mysql):

```yaml
# module defaults:
update_every : 2
priority     : 20000

local:  # job name
  update_every : 5 # job update frequency
  other_var1   : some_val # module specific variable

other_job:
  priority     : 5 # job position on dashboard
  other_var2   : val # module specific variable
```

`update_every` and `priority` are always optional.

## How to debug a python module

```
# become user netdata
sudo su -s /bin/bash netdata
```

Depending on where Netdata was installed, execute one of the following commands to trace the execution of a python module:

```
# execute the plugin in debug mode, for a specific module
/opt/netdata/usr/libexec/netdata/plugins.d/python.d.plugin <module> debug trace
/usr/libexec/netdata/plugins.d/python.d.plugin <module> debug trace
```

Where `[module]` is the directory name under <https://github.com/netdata/netdata/tree/master/src/collectors/python.d.plugin> 

**Note**: If you would like execute a collector in debug mode while it is still running by Netdata, you can pass the `nolock` CLI option to the above commands.

## How to write a new module

See [develop a custom collector in Python](https://github.com/netdata/netdata/edit/master/docs/developer-and-contributor-corner/python-collector.md).
