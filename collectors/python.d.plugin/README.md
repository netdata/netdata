<!--
---
title: "python.d.plugin"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/README.md
---
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

Every module should be compatible with python2 and python3.
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

Where `[module]` is the directory name under <https://github.com/netdata/netdata/tree/master/collectors/python.d.plugin> 

## How to write a new module

Writing new python module is simple. You just need to remember to include 5 major things:

-   **ORDER** global list
-   **CHART** global dictionary
-   **Service** class
-   **\_get_data** method
-   all code needs to be compatible with Python 2 (**≥ 2.7**) *and* 3 (**≥ 3.1**)

If you plan to submit the module in a PR, make sure and go through the [PR checklist for new modules](#pull-request-checklist-for-python-plugins) beforehand to make sure you have updated all the files you need to. 

For a quick start, you can look at the [example plugin](example/example.chart.py).

### Global variables `ORDER` and `CHART`

`ORDER` list should contain the order of chart ids. Example:

```py
ORDER = ['first_chart', 'second_chart', 'third_chart']
```

`CHART` dictionary is a little bit trickier. It should contain the chart definition in following format:

```py
CHART = {
    id: {
        'options': [name, title, units, family, context, charttype],
        'lines': [
            [unique_dimension_name, name, algorithm, multiplier, divisor]
        ]}
```

All names are better explained in the [External Plugins](../) section.
Parameters like `priority` and `update_every` are handled by `python.d.plugin`.

### `Service` class

Every module needs to implement its own `Service` class. This class should inherit from one of the framework classes:

-   `SimpleService`
-   `UrlService`
-   `SocketService`
-   `LogService`
-   `ExecutableService`

Also it needs to invoke the parent class constructor in a specific way as well as assign global variables to class variables. 

Simple example:

```py
from base import UrlService
class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
```

### `_get_data` collector/parser

This method should grab raw data from `_get_raw_data`, parse it, and return a dictionary where keys are unique dimension names or `None` if no data is collected.

Example:

```py
def _get_data(self):
    try:
        raw = self._get_raw_data().split(" ")
        return {'active': int(raw[2])}
    except (ValueError, AttributeError):
        return None
```

# More about framework classes

Every framework class has some user-configurable variables which are specific to this particular class. Those variables should have default values initialized in the child class constructor.

If module needs some additional user-configurable variable, it can be accessed from the `self.configuration` list and assigned in constructor or custom `check` method. Example:

```py
def __init__(self, configuration=None, name=None):
    UrlService.__init__(self, configuration=configuration, name=name)
    try:
        self.baseurl = str(self.configuration['baseurl'])
    except (KeyError, TypeError):
        self.baseurl = "http://localhost:5001"
```

Classes implement `_get_raw_data` which should be used to grab raw data. This method usually returns a list of strings.

### `SimpleService`

_This is last resort class, if a new module cannot be written by using other framework class this one can be used._

_Example: `ceph`, `sensors`_

It is the lowest-level class which implements most of module logic, like:

-   threading
-   handling run times
-   chart formatting
-   logging
-   chart creation and updating

### `LogService`

_Examples: `apache_cache`, `nginx_log`_

_Variable from config file_: `log_path`.

Object created from this class reads new lines from file specified in `log_path` variable. It will check if file exists and is readable. Also `_get_raw_data` returns list of strings where each string is one line from file specified in `log_path`.

### `ExecutableService`

_Examples: `exim`, `postfix`_

_Variable from config file_: `command`.

This allows to execute a shell command in a secure way. It will check for invalid characters in `command` variable and won't proceed if there is one of:

-   '&'
-   '|'
-   ';'
-   '>'
-   '\<'

For additional security it uses python `subprocess.Popen` (without `shell=True` option) to execute command. Command can be specified with absolute or relative name. When using relative name, it will try to find `command` in `PATH` environment variable as well as in `/sbin` and `/usr/sbin`.

`_get_raw_data` returns list of decoded lines returned by `command`.

### UrlService

_Examples: `apache`, `nginx`, `tomcat`_

_Multiple Endpoints (urls) Examples: [`rabbitmq`](rabbitmq/README.md) (simpler) , [`elasticsearch`](elasticsearch/README.md) (threaded)_


_Variables from config file_: `url`, `user`, `pass`.

If data is grabbed by accessing service via HTTP protocol, this class can be used. It can handle HTTP Basic Auth when specified with `user` and `pass` credentials.

Please note that the config file can use different variables according to the specification of each module.

`_get_raw_data` returns list of utf-8 decoded strings (lines).

### SocketService

_Examples: `dovecot`, `redis`_

_Variables from config file_: `unix_socket`, `host`, `port`, `request`.

Object will try execute `request` using either `unix_socket` or TCP/IP socket with combination of `host` and `port`. This can access unix sockets with SOCK_STREAM or SOCK_DGRAM protocols and TCP/IP sockets in version 4 and 6 with SOCK_STREAM setting.

Sockets are accessed in non-blocking mode with 15 second timeout.

After every execution of `_get_raw_data` socket is closed, to prevent this module needs to set `_keep_alive` variable to `True` and implement custom `_check_raw_data` method.

`_check_raw_data` should take raw data and return `True` if all data is received otherwise it should return `False`. Also it should do it in fast and efficient way.

## Pull Request Checklist for Python Plugins

This is a generic checklist for submitting a new Python plugin for Netdata.  It is by no means comprehensive.

At minimum, to be buildable and testable, the PR needs to include:

-   The module itself, following proper naming conventions: `collectors/python.d.plugin/<module_dir>/<module_name>.chart.py`
-   A README.md file for the plugin under `collectors/python.d.plugin/<module_dir>`. 
-   The configuration file for the module: `collectors/python.d.plugin/<module_dir>/<module_name>.conf`. Python config files are in YAML format, and should include comments describing what options are present. The instructions are also needed in the configuration section of the README.md 
-   A basic configuration for the plugin in the appropriate global config file: `collectors/python.d.plugin/python.d.conf`, which is also in YAML format.  Either add a line that reads `# <module_name>: yes` if the module is to be enabled by default, or one that reads `<module_name>: no` if it is to be disabled by default.
-   A makefile for the plugin at `collectors/python.d.plugin/<module_dir>/Makefile.inc`.  Check an existing plugin for what this should look like.
-   A line in `collectors/python.d.plugin/Makefile.am` including the above-mentioned makefile. Place it with the other plugin includes (please keep the includes sorted alphabetically).
-   Optionally, chart information in `web/gui/dashboard_info.js`.  This generally involves specifying a name and icon for the section, and may include descriptions for the section or individual charts.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
