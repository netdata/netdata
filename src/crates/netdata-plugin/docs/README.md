# External plugins

`plugins.d` is the Netdata internal plugin that collects metrics
from external processes, thus allowing Netdata to use **external plugins**.

## Provided External Plugins

|                                                 plugin                                                 | language |      O/S       | description                                                                                                                             |
|:------------------------------------------------------------------------------------------------------:|:--------:|:--------------:|:----------------------------------------------------------------------------------------------------------------------------------------|
|     [apps.plugin](/src/collectors/apps.plugin/README.md)     |   `C`    | linux, freebsd | monitors the whole process tree on Linux and FreeBSD and breaks down system resource usage by **process**, **user** and **user group**. |
| [charts.d.plugin](/src/collectors/charts.d.plugin/README.md) |  `BASH`  |      all       | a **plugin orchestrator** for data collection modules written in `BASH` v4+.                                                            |
|     [cups.plugin](/src/collectors/cups.plugin/README.md)     |   `C`    |      all       | monitors **CUPS**                                                                                                                       |
|     [ebpf.plugin](/src/collectors/ebpf.plugin/README.md)     |   `C`    |     linux      | monitors different metrics on environments using kernel internal functions.                                                             |
|              [go.d.plugin](/src/go/plugin/go.d/README.md)               |   `GO`   |      all       | collects metrics from the system, applications, or third-party APIs.                                                                    |
|   [ioping.plugin](/src/collectors/ioping.plugin/README.md)   |   `C`    |      all       | measures disk latency.                                                                                                                  |
| [freeipmi.plugin](/src/collectors/freeipmi.plugin/README.md) |   `C`    |     linux      | collects metrics from enterprise hardware sensors, on Linux servers.                                                                    |
|   [nfacct.plugin](/src/collectors/nfacct.plugin/README.md)   |   `C`    |     linux      | collects netfilter firewall, connection tracker and accounting metrics using `libmnl` and `libnetfilter_acct`.                          |
|  [xenstat.plugin](/src/collectors/xenstat.plugin/README.md)  |   `C`    |     linux      | collects XenServer and XCP-ng metrics using `lxenstat`.                                                                                 |
|     [perf.plugin](/src/collectors/perf.plugin/README.md)     |   `C`    |     linux      | collects CPU performance metrics using performance monitoring units (PMU).                                                              |
| [python.d.plugin](/src/collectors/python.d.plugin/README.md) | `python` |      all       | a **plugin orchestrator** for data collection modules written in `python` v2 or v3 (both are supported).                                |
| [slabinfo.plugin](/src/collectors/slabinfo.plugin/README.md) |   `C`    |     linux      | collects kernel internal cache objects (SLAB) metrics.                                                                                  |

Plugin orchestrators may also be described as **modular plugins**. They are modular since they accept custom made modules to be included. Writing modules for these plugins is easier than accessing the native Netdata API directly. You will find modules already available for each orchestrator under the directory of the particular modular plugin (e.g. under python.d.plugin for the python orchestrator).
Each of these modular plugins has each own methods for defining modules. Please check the examples and their documentation.

## Motivation

This plugin allows Netdata to use **external plugins** for data collection:

1.  external data collection plugins may be written in any computer language.

2.  external data collection plugins may use O/S capabilities or `setuid` to
    run with escalated privileges (compared to the `netdata` daemon).
    The communication between the external plugin and Netdata is unidirectional
    (from the plugin to Netdata), so that Netdata cannot manipulate an external
    plugin running with escalated privileges.

## Operation

Each of the external plugins is expected to run forever.
Netdata will start it when it starts and stop it when it exits.

If the external plugin exits or crashes, Netdata will log an error.
If the external plugin exits or crashes without pushing metrics to Netdata, Netdata will not start it again.

-   Plugins that exit with any value other than zero, will be disabled. Plugins that exit with zero, will be restarted after some time.
-   Plugins may also be disabled by Netdata if they output things that Netdata does not understand.

The `stdout` of external plugins is connected to Netdata to receive metrics,
with the API defined below.

The `stderr` of external plugins is connected to Netdata's `error.log`.

Plugins can create any number of charts with any number of dimensions each. Each chart can have its own characteristics independently of the others generated by the same plugin. For example, one chart may have an update frequency of 1 second, another may have 5 seconds and a third may have 10 seconds.

## Configuration

Netdata will supply the environment variables `NETDATA_USER_CONFIG_DIR` (for user supplied) and `NETDATA_STOCK_CONFIG_DIR` (for Netdata supplied) configuration files to identify the directory where configuration files are stored. It is up to the plugin to read the configuration it needs.

The `netdata.conf` section `[plugins]` section contains a list of all the plugins found at the system where Netdata runs, with a boolean setting to enable them or not.

Example:

```
[plugins]
	# enable running new plugins = yes
	# check for new plugins every = 60

	# charts.d = yes
	# ioping = yes
	# python.d = yes
```

The setting `enable running new plugins` sets the default behavior for all external plugins. It can be 
overridden for distinct plugins by modifying the appropriate plugin value configuration to either `yes` or `no`.

The setting `check for new plugins every` sets the interval between scans of the directory
`/usr/libexec/netdata/plugins.d`. New plugins can be added any time, and Netdata will detect them in a timely manner.

For each of the external plugins enabled, another `netdata.conf` section
is created, in the form of `[plugin:NAME]`, where `NAME` is the name of the external plugin.
This section allows controlling the update frequency of the plugin and provide
additional command line arguments to it.

For example, for `apps.plugin` the following section is available:

```
[plugin:apps]
	# update every = 1
	# command options =
```

-   `update every` controls the granularity of the external plugin.
-   `command options` allows giving additional command line options to the plugin.

Netdata will provide to the external plugins the environment variable `NETDATA_UPDATE_EVERY`, in seconds (the default is 1). This is the **minimum update frequency** for all charts. A plugin that is updating values more frequently than this, is just wasting resources.

Netdata will call the plugin with just one command line parameter: the number of seconds the user requested this plugin to update its data (by default is also 1).

Other than the above, the plugin configuration is up to the plugin.

Keep in mind, that the user may use Netdata configuration to overwrite chart and dimension parameters. This is transparent to the plugin.

### Autoconfiguration

Plugins should attempt to autoconfigure themselves when possible.

For example, if your plugin wants to monitor `squid`, you can search for it on port `3128` or `8080`. If any succeeds, you can proceed. If it fails you can output an error (on stderr) saying that you cannot find `squid` running and giving instructions about the plugin configuration. Then you can stop (exit with non-zero value), so that Netdata will not attempt to start the plugin again.

## External Plugins API

Any program that can print a few values to its standard output can become a Netdata external plugin.

Netdata parses lines starting with:

-    `CHART` - create or update a chart
-    `DIMENSION` - add or update a dimension to the chart just created
-    `VARIABLE` - define a variable (to be used in health calculations)
-    `CLABEL` - add a label to a chart
-    `CLABEL_COMMIT` - commit added labels to the chart
-    `FUNCTION` - define a function that can be called later to execute it
-    `BEGIN` - initialize data collection for a chart
-    `SET` - set the value of a dimension for the initialized chart
-    `END` - complete data collection for the initialized chart
-    `FLUSH` - ignore the last collected values
-    `DISABLE` - disable this plugin
-    `FUNCTION` - define functions
-    `FUNCTION_PROGRESS` - report the progress of a function execution
-    `FUNCTION_RESULT_BEGIN` - to initiate the transmission of function results
-    `FUNCTION_RESULT_END` - to end the transmission of function result
-    `CONFIG` - to define dynamic configuration entities

a single program can produce any number of charts with any number of dimensions each.

Charts can be added any time (not just the beginning).

Netdata may send the following commands to the plugin's `stdin`:

-    `FUNCTION` - to call a specific function, with all parameters inline
-    `FUNCTION_PAYLOAD` - to call a specific function, with a payload of parameters
-    `FUNCTION_PAYLOAD_END` - to end the payload of parameters
-    `FUNCTION_CANCEL` - to cancel a running function transaction - no response is required
-    `FUNCTION_PROGRESS` - to report that a user asked the progress of running function call - no response is required

### Command line parameters

The plugin **MUST** accept just **one** parameter: **the number of seconds it is
expected to update the values for its charts**. The value passed by Netdata
to the plugin is controlled via its configuration file (so there is no need
for the plugin to handle this configuration option).

The external plugin can overwrite the update frequency. For example, the server may
request per second updates, but the plugin may ignore it and update its charts
every 5 seconds.

### Environment variables

There are a few environment variables that are set by `netdata` and are
available for the plugin to use.

|             variable             | description                                                                                                                                                                                                                                            |
|:--------------------------------:|:-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
|    `NETDATA_USER_CONFIG_DIR`     | The directory where all Netdata-related user configuration should be stored. If the plugin requires custom user configuration, this is the place the user has saved it (normally under `/etc/netdata`).                                                |
|    `NETDATA_STOCK_CONFIG_DIR`    | The directory where all Netdata -related stock configuration should be stored. If the plugin is shipped with configuration files, this is the place they can be found (normally under `/usr/lib/netdata/conf.d`).                                      |
|      `NETDATA_PLUGINS_DIR`       | The directory where all Netdata plugins are stored.                                                                                                                                                                                                    |
|   `NETDATA_USER_PLUGINS_DIRS`    | The list of directories where custom plugins are stored.                                                                                                                                                                                               |
|        `NETDATA_WEB_DIR`         | The directory where the web files of Netdata are saved.                                                                                                                                                                                                |
|       `NETDATA_CACHE_DIR`        | The directory where the cache files of Netdata are stored. Use this directory if the plugin requires a place to store data. A new directory should be created for the plugin for this purpose, inside this directory.                                  |
|        `NETDATA_LOG_DIR`         | The directory where the log files are stored. By default the `stderr` output of the plugin will be saved in the `error.log` file of Netdata.                                                                                                           |
|      `NETDATA_HOST_PREFIX`       | This is used in environments where system directories like `/sys` and `/proc` have to be accessed at a different path.                                                                                                                                 |
|      `NETDATA_DEBUG_FLAGS`       | This is a number (probably in hex starting with `0x`), that enables certain Netdata debugging features. Check **\[[Tracing Options]]** for more information.                                                                                           |
|      `NETDATA_UPDATE_EVERY`      | The minimum number of seconds between chart refreshes. This is like the **internal clock** of Netdata (it is user configurable, defaulting to `1`). There is no meaning for a plugin to update its values more frequently than this number of seconds. |
|     `NETDATA_INVOCATION_ID`      | A random UUID in compact form, representing the unique invocation identifier of Netdata. When running under systemd, Netdata uses the `INVOCATION_ID` set by systemd.                                                                                  |
|       `NETDATA_LOG_METHOD`       | One of `syslog`, `journal`, `stderr` or `none`, indicating the preferred log method of external plugins.                                                                                                                                               |
|       `NETDATA_LOG_FORMAT`       | One of `journal`, `logfmt` or `json`, indicating the format of the logs. Plugins can use the Netdata `systemd-cat-native` command to log always in `journal` format, and have it automatically converted to the format expected by netdata.            |
|       `NETDATA_LOG_LEVEL`        | One of `emergency`, `alert`, `critical`, `error`, `warning`, `notice`, `info`, `debug`. Plugins are expected to log events with the given priority and the more important ones.                                                                        |
|    `NETDATA_SYSLOG_FACILITY`     | Set only when the `NETDATA_LOG_METHOD` is `syslog`. Possible values are `auth`, `authpriv`, `cron`, `daemon`, `ftp`, `kern`, `lpr`, `mail`, `news`, `syslog`, `user`, `uucp` and `local0` to `local7`                                                  |  
| `NETDATA_ERRORS_THROTTLE_PERIOD` | The log throttling period in seconds.                                                                                                                                                                                                                  |
|   `NETDATA_ERRORS_PER_PERIOD`    | The allowed number of log events per period.                                                                                                                                                                                                           | 
| `NETDATA_SYSTEMD_JOURNAL_PATH`   | When `NETDATA_LOG_METHOD` is set to `journal`, this is the systemd-journald socket path to use.                                                                                                                                                        |

### The output of the plugin

The plugin should output instructions for Netdata to its output (`stdout`). Since this uses pipes, please make sure you flush stdout after every iteration.

#### DISABLE

`DISABLE` will disable this plugin. This will prevent Netdata from restarting the plugin. You can also exit with the value `1` to have the same effect.

#### HOST_DEFINE

`HOST_DEFINE` defines a new (or updates an existing) virtual host.

The template is:

> HOST_DEFINE machine_guid hostname

where:

-   `machine_guid`

    uniquely identifies the host, this is what will be needed to add charts to the host.

-   `hostname`

    is the hostname of the virtual host

#### HOST_LABEL

`HOST_LABEL` adds a key-value pair to the virtual host labels. It has to be given between `HOST_DEFINE` and `HOST_DEFINE_END`.

The template is:

> HOST_LABEL key value

where:

-   `key`

    uniquely identifies the key of the label

-   `value`

    is the value associated with this key

There are a few special keys that are used to define the system information of the monitored system:

- `_cloud_provider_type`
- `_cloud_instance_type`
- `_cloud_instance_region`
- `_os_name`
- `_os_version`
- `_kernel_version`
- `_system_cores`
- `_system_cpu_freq`
- `_system_ram_total`
- `_system_disk_space`
- `_architecture`
- `_virtualization`
- `_container`
- `_container_detection`
- `_virt_detection`
- `_is_k8s_node`
- `_install_type`
- `_prebuilt_arch`
- `_prebuilt_dist`

#### HOST_DEFINE_END

`HOST_DEFINE_END` commits the host information, creating a new host entity, or updating an existing one with the same `machine_guid`.

#### HOST 

`HOST` switches data collection between hosts.

The template is:

> HOST machine_guid

where:

-   `machine_guid`

    is the UUID of the host to switch to. After this command, every other command following it is assumed to be associated with this host.
    Setting machine_guid to `localhost` switches data collection to the local host.

#### CHART

`CHART` defines a new chart.

the template is:

> CHART type.id name title units \[family \[context \[charttype \[priority \[update_every \[options \[plugin [module]]]]]]]]

 where:

-   `type.id`

    uniquely identifies the chart,
    this is what will be needed to add values to the chart

    the `type` part controls the menu the charts will appear in

-   `name`

    is the name that will be presented to the user instead of `id` in `type.id`. This means that only the `id` part of
    `type.id` is changed. When a name has been given, the chart is indexed (and can be referred) as both `type.id` and
    `type.name`. You can set name to `''`, or `null`, or `(null)` to disable it. If a chart with the same name already
    exists, a serial number is automatically attached to the name to avoid naming collisions.

-   `title`

    the text above the chart

-   `units`

    the label of the vertical axis of the chart,
    all dimensions added to a chart should have the same units
    of measurement

-   `family`

    is used to group charts together
    (for example all eth0 charts should say: eth0),
    if empty or missing, the `id` part of `type.id` will be used

    this controls the sub-menu on the dashboard

-   `context`

    the context is giving the template of the chart. For example, if multiple charts present the same information for a different family, they should have the same `context`

    this is used for looking up rendering information for the chart (colors, sizes, informational texts) and also apply alerts to it

-   `charttype`

    one of `line`, `area`, `stacked` or `heatmap`,
    if empty or missing, the `line` will be used

-   `priority`

    is the relative priority of the charts as rendered on the web page,
    lower numbers make the charts appear before the ones with higher numbers,
    if empty or missing, `1000` will be used

-   `update_every`

    overwrite the update frequency set by the server,
    if empty or missing, the user configured value will be used

-   `options`

    a space separated list of options, enclosed in quotes. The following options are currently supported: `obsolete` to mark a chart as obsolete (Netdata will hide it and delete it after some time), `store_first` to make Netdata store the first collected value, assuming there was an invisible previous value set to zero (this is used by statsd charts - if the first data collected value of incremental dimensions is not zero based, unrealistic spikes will appear with this option set) and `hidden` to perform all operations on a chart, but do not offer it on dashboards (the chart will be send to external databases). `CHART` options have been added in Netdata v1.7 and the `hidden` option was added in 1.10.

-   `plugin` and `module`

    both are just names that are used to let the user identify the plugin and the module that generated the chart. If `plugin` is unset or empty, Netdata will automatically set the filename of the plugin that generated the chart. `module` has not default.

#### DIMENSION

`DIMENSION` defines a new dimension for the chart

the template is:

> DIMENSION id \[name \[algorithm \[multiplier \[divisor [options]]]]]

 where:

-   `id`

    the `id` of this dimension (it is a text value, not numeric),
    this will be needed later to add values to the dimension

    We suggest to avoid using `.` in dimension ids. External databases expect metrics to be `.` separated and people will get confused if a dimension id contains a dot.

-   `name`

    the name of the dimension as it will appear at the legend of the chart,
    if empty or missing the `id` will be used

-   `algorithm`

    one of:

    -   `absolute`

        the value is to drawn as-is (interpolated to second boundary),
        if `algorithm` is empty, invalid or missing, `absolute` is used

    -   `incremental`

        the value increases over time,
        the difference from the last value is presented in the chart,
        the server interpolates the value and calculates a per second figure

    -   `percentage-of-absolute-row`

        the % of this value compared to the total of all dimensions

    -   `percentage-of-incremental-row`

        the % of this value compared to the incremental total of
        all dimensions

-   `multiplier`

    an integer value to multiply the collected value,
    if empty or missing, `1` is used

-   `divisor`

    an integer value to divide the collected value,
    if empty or missing, `1` is used

-   `options`

    a space separated list of options, enclosed in quotes. Options supported: `obsolete` to mark a dimension as obsolete (Netdata will delete it after some time) and `hidden` to make this dimension hidden, it will take part in the calculations but will not be presented in the chart.

#### VARIABLE

> VARIABLE [SCOPE] name = value

`VARIABLE` defines a variable that can be used in alerts. This is to used for setting constants (like the max connections a server may accept).

Variables support 2 scopes:

-   `GLOBAL` or `HOST` to define the variable at the host level.
-   `LOCAL` or `CHART` to define the variable at the chart level. Use chart-local variables when the same variable may exist for different charts (i.e. Netdata monitors 2 mysql servers, and you need to set the `max_connections` each server accepts). Using chart-local variables is the ideal to build alert templates.

The position of the `VARIABLE` line, sets its default scope (in case you do not specify a scope). So, defining a `VARIABLE` before any `CHART`, or between `END` and `BEGIN` (outside any chart), sets `GLOBAL` scope, while defining a `VARIABLE` just after a `CHART` or a `DIMENSION`, or within the `BEGIN` - `END` block of a chart, sets `LOCAL` scope.

These variables can be set and updated at any point.

Variable names should use alphanumeric characters, the `.` and the `_`.

The `value` is floating point (Netdata used `long double`).

Variables are transferred to upstream Netdata servers (streaming and database replication).

#### CLABEL

> CLABEL name value source

`CLABEL` defines a label used to organize and identify a chart.

Name and value accept characters according to the following table:

| Character           | Symbol | Label Name | Label Value |
|---------------------|:------:|:----------:|:-----------:|
| UTF-8 character     | UTF-8  |     _      |    keep     |
| Lower case letter   | [a-z]  |    keep    |    keep     |
| Upper case letter   | [A-Z]  |    keep    |    [a-z]    |
| Digit               | [0-9]  |    keep    |    keep     |
| Underscore          |   _    |    keep    |    keep     |
| Minus               |   -    |    keep    |    keep     |
| Plus                |   +    |     _      |    keep     |
| Colon               |   :    |     _      |    keep     |
| Semicolon           |   ;    |     _      |      :      |
| Equal               |   =    |     _      |      :      |
| Period              |   .    |    keep    |    keep     |
| Comma               |   ,    |     .      |      .      |
| Slash               |   /    |    keep    |    keep     |
| Backslash           |   \    |     /      |      /      |
| At                  |   @    |     _      |    keep     |
| Space               |  ' '   |     _      |    keep     |
| Opening parenthesis |   (    |     _      |    keep     |
| Closing parenthesis |   )    |     _      |    keep     |
| Anything else       |        |     _      |      _      |

The `source` is an integer field that can have the following values:
- `1`: The value was set automatically.
- `2`: The value was set manually.
- `4`: This is a K8 label.
- `8`: This is a label defined using `netdata` Agent-Cloud link.

#### CLABEL_COMMIT

`CLABEL_COMMIT` indicates that all labels were defined and the chart can be updated.

#### FUNCTION

The plugin can register functions to Netdata, like this:

> FUNCTION [GLOBAL] "name and parameters of the function" timeout "help string for users" "tags" "access" priority version

- Tags currently recognized are either `top` or `logs` (or both, space separated).
- Access is one of `any`, `member`, or `admin`:
  - `any` to offer the function to all users of Netdata, even if they are not authenticated.
  - `member` to offer the function to all authenticated members of Netdata.
  - `admin` to offer the function only to authenticated administrators.
- Priority defines the position of the function relative to the other functions (default is 100).
- Version defines the version of the function (default is 0).

Users can use a function to ask for more information from the collector. Netdata maintains a registry of functions in 2 levels:

- per node
- per chart

Both node and chart functions are exactly the same, but chart functions allow Netdata to relate functions with charts and therefore present a context-sensitive menu of functions related to the chart the user is using.

Users can get a list of all the registered functions using the `/api/v1/functions` endpoint of Netdata and call functions using the `/api/v1/function` API call of Netdata.

Once a function is called, the plugin will receive at its standard input a command that looks like this:

```
FUNCTION transaction_id timeout "name and parameters of the function as one quoted parameter" "user permissions value" "source of request"
```

When the function to be called is to receive a payload of parameters, the call looks like this:

```
FUNCTION_PAYLOAD transaction_id timeout "name and parameters of the function as one quoted parameter" "user permissions value" "source of request" "content/type"
body of the payload, formatted according to content/type
FUNCTION PAYLOAD END
```

In this case, Netdata will send:

- A line starting with `FUNCTION_PAYLOAD` together with the required metadata for the function, like the transaction id, the function name and its parameters, the timeout and the content type. This line ends with a newline.
- Then, the payload itself (which may or may not have newlines in it). The payload should be parsed according to the content type parameter.
- Finally, a line starting with `FUNCTION_PAYLOAD_END`, so it is expected like `\nFUNCTION_PAYLOAD_END\n`.

Note 1: The plugins.d protocol allows parameters without single or double quotes if they don't contain spaces. However, the plugin should be able to parse parameters even if they are enclosed in single or double quotes. If the first character of a parameter is a single quote, its last character should also be a single quote too, and similarly for double quotes.

Note 2: Netdata always sends the function and its parameters enclosed in double quotes. If the function command and its parameters contain quotes, they are converted to single quotes.

The plugin is expected to parse and validate `name and parameters of the function as  one quotes parameter`. Netdata allows the user interface to manipulate this string by appending more parameters.

If the plugin rejects the request, it should respond with this:

```
FUNCTION_RESULT_BEGIN transaction_id 400 application/json
{
   "status": 400,
   "error_message": "description of the rejection reasons"
}
FUNCTION_RESULT_END
```

If the plugin prepares a response, it should send (via its standard output, together with the collected data, but not interleaved with them):

```
FUNCTION_RESULT_BEGIN transaction_id http_response_code content_type expiration
```

Where:

  - `transaction_id` is the transaction id that Netdata sent for this function execution
  - `http_response_code` is the http error code Netdata should respond with, 200 is the "ok" response
  - `content_type` is the content type of the response
  - `expiration` is the absolute timestamp (number, unix epoch) this response expires

Immediately after this, all text is assumed to be the response content.
The content is text and line oriented. The maximum line length accepted is 15kb. Longer lines will be truncated.
The type of the context itself depends on the plugin and the UI.

To terminate the message, Netdata seeks a line with just this:

```
FUNCTION_RESULT_END
```

This defines the end of the message. `FUNCTION_RESULT_END` should appear in a line alone, without any other text, so it is wise to add `\n` before and after it.

After this line, Netdata resumes processing collected metrics from the plugin.

The maximum uncompressed payload size Netdata will accept is 100MB.

##### Functions cancellation

Netdata is able to detect when a user made an API request, but abandoned it before it was completed. If this happens to an API called for a function served by the plugin, Netdata will generate a `FUNCTION_CANCEL` request to let the plugin know that it can stop processing the query.

After receiving such a command, the plugin **must still send a response for the original function request**, to wake up any waiting threads before they timeout. The http response code is not important, since the response will be discarded, however for auditing reasons we suggest to send back a 499 http response code. This is not a standard response code according to the HTTP protocol, but web servers like `nginx` are using it to indicate that a request was abandoned by a user.

##### Functions progress

When a request takes too long to be processed, Netdata allows the plugin to report progress to Netdata, which in turn will report progress to the caller.

The plugin can send `FUNCTION_PROGRESS` like this:

```
FUNCTION_PROGRESS transaction_id done all
```

Where:

- `transaction_id` is the transaction id of the function request
- `done` is an integer value indicating the amount of work done
- `all` is an integer value indicating the total amount of work to be done

Netdata supports two kinds of progress:
- progress as a percentage, which is calculated as `done * 100 / all`
- progress without knowing the total amount of work to be done, which is enabled when the plugin reports `all` as zero.

##### Functions timeout

All functions calls specify a timeout, at which all the intermediate routing nodes (parents, web server threads) will time out and abort the call.

However, all intermediate routing nodes are configured to extend the timeout when the caller asks for progress. This works like this:

When a progress request is received, if the expected timeout of the request is less than or equal to 10 seconds, the expected timeout is extended by 10 seconds.

Usually, the user interface asks for a progress every second. So, during the last 10 seconds of the timeout, every progress request made shifts the timeout 10 seconds to the future.

To accomplish this, when Netdata receives a progress request by a user, it generates progress requests to the plugin, updating all the intermediate nodes to extend their timeout if necessary.

The plugin will receive progress requests like this:

```
FUNCTION_PROGRESS transaction_id
```

There is no need to respond to this command. It is only there to let the plugin know that a user is still waiting for the query to finish. 

#### CONFIG

`CONFIG` commands sent from the plugin to Netdata define dynamic configuration entities. These configurable entities are exposed to the user interface, allowing users to change configuration at runtime.

Dynamically configurations made this way are saved to disk by Netdata and are replayed automatically when Netdata or the plugin restarts.

`CONFIG` commands look like this:

```
CONFIG id action ...
```

Where:

- `id` is a unique identifier for the configurable entity. This should by design be unique across Netdata. It should be something like `plugin:module:jobs`, e.g. `go.d:postgresql:jobs:masterdb`. This is assumed to be colon-separated with the last part (`masterdb` in our example), being the one displayed to users when there ano conflicts under the same configuration path.
- `action` can be:
  - `create`, to declare the dynamic configuration entity
  - `delete`, to delete the dynamic configuration entity - this does not delete user configuration, we if an entity with the same id is created in the future, the saved configuration will be given to it.
  - `status`, to update the dynamic configuration entity status

> IMPORTANT:<br/>
> The plugin should blindly create, delete and update the status of its dynamic configuration entities, without any special logic applied to it. Netdata needs to be updated of what is actually happening at the plugin. Keep in mind that creating dynamic configuration entities triggers responses from Netdata, depending on its type and status. Re-creating a job, triggers the same responses every time, so make sure you create jobs only when you add jobs. 

When the `action` is `create`, the following additional parameters are expected:

```
CONFIG id action status type "path" source_type "source" "supported commands" "view permissions" "edit permissions"
```

Where:

- `action` should be `create`
- `status` can be:
  - `accepted`, the plugin accepted the configuration, but it is not running yet.
  - `running`, the plugin accepted and runs the configuration.
  - `failed`, the plugin tries to run the configuration but it fails.
  - `incomplete`, the plugin needs additional settings to run this configuration. This is usually used for the cases the plugin discovered a job, but important information is missing for it to work.
  - `disabled`, the configuration has been disabled by a user.
  - `orphan`, the configuration is not claimed by any plugin. This is used internally by Netdata to mark the configuration nodes available, for which there is no plugin related to them. Do not use in plugins directly.
- `type` can be `single`, `template` or `job`:
  - `single` is used when the configurable entity is fixed and users should never be able to add or delete it.
  - `template` is used to define a template based on which users can add multiple configurations, like adding data collection jobs. So, the plugin defines the template of the jobs and users are presented with a `[+]` button to add such configuration jobs. The plugin can define multiple templates by giving different `id`s to them.
  - `job` is used to define a job of a template. The plugin should always add all its jobs, independently of the way they have been discovered. It is important to note the relation between `template` and `job` when it comes it the `id`: The `id` of the template should be the prefix of the `job`'s `id`. For example, if the template is `go.d:postgresql:jobs`, then all its jobs be like `go.d:postgresql:jobs:jobname`. 
- `path` is the absolute path of the configurable entity inside the tree of Netdata configurations. Usually, this is should be `/collectors`.
- `source` can be `internal`, `stock`, `user`, `discovered` or `dyncfg`:
  - `internal` is used for configurations that are based on internal code settings
  - `stock` is used for default configurations
  - `discovered` is used for dynamic configurations the plugin discovers by its own
  - `user` is used for user configurations, usually via a configuration file
  - `dyncfg` is used for configuration received via this dynamic configuration mechanism
- `source` should provide more details about the exact source of the configuration, like `line@file`, or `user@ip`, etc.
- `supported_commands` is a space separated list of the following keywords, enclosed in single or double quotes. These commands are used by the user interface to determine the actions the users can take:
  - `schema`, to expose the JSON schema for the user interface. This is mandatory for all configurable entities. When `schema` requests are received, Netdata will first attempt to load the schema from `/etc/netdata/schema.d/` and `/var/lib/netdata/conf.d/schema.d`. For jobs, it will serve the schema of their template. If no schema is found for the required `id`, the `schema` request will be forwarded to the plugin, which is expected to send back the relevant schema.
  - `get`, to expose the current configuration values, according the schema defined. `templates` cannot support `get`, since they don't maintain any data.
  - `update`, to receive configuration updates for this entity. `templates` cannot support `update`, since they don't maintain any data.
  - `test`, like `update` but only test the configuration and report success or failure.
  - `add`, to receive job creation commands for templates. Only `templates` should support this command.
  - `remove`, to remove a configuration. Only `jobs` should support this command.
  - `enable` and `disable`, to receive user requests to enable and disable this entity. Adding only one of `enable` or `disable` to the supported commands, Netdata will add both of them. The plugin should expose these commands on `templates` only when it wants to receive `enable` and `disable` commands for all the `jobs` of this `template`.
  - `restart`, to restart a job.
- `view permissions` and `edit permissions` are bitmaps of the Netdata permission system to control access to the configuration. If set to zero, Netdata will require a signed in user with view and edit permissions to the Netdata's configuration system.

The plugin receives commands as if it had exposed a `FUNCTION` named `config`. Netdata formats all these calls like this:

```
config id command
```

Where `id` is the unique id of the configurable entity and `command` is one of the supported commands the plugin sent to Netdata.

The plugin will receive (for commands: `schema`, `get`, `remove`, `enable`, `disable` and `restart`):

```
FUNCTION transaction_id timeout "config id command" "user permissions value" "source string"
```

or (for commands: `update`, `add` and `test`):

```
FUNCTION_PAYLOAD transaction_id timeout "config id command" "user permissions value" "source string" "content/type"
body of the payload formatted according to content/type
FUNCTION_PAYLOAD_END
```

Once received, the plugin should process it and respond accordingly. 

Immediately after the plugin adds a configuration entity, if the commands `enable` and `disable` are supported by it, Netdata will send either `enable` or `disable` for it, based on the last user action, which has been persisted to disk.

Plugin responses follow the same format `FUNCTIONS` do:

```
FUNCTION_RESULT_BEGIN transaction_id http_response_code content/type expiration
body of the response formatted according to content/type
FUNCTION_RESULT_END
```

Successful responses (HTTP response code 200) to `schema` and `get` should send back the relevant JSON object.
All other responses should have the following response body:

```json
{
  "status" : 404,
  "message" : "some text"
}
```

The user interface presents the message to users, even when the response is successful (HTTP code 200).

When responding to additions and updates, Netdata uses the following success response codes to derive additional information:

- `200`, responding with 200, means the configuration has been accepted and it is running.
- `202`, responding with 202, means the configuration has been accepted but it is not yet running. A subsequent `status` action will update it.
- `298`, responding with 298, means the configuration has been accepted but it is disabled for some reason (probably because it matches nothing or the contents are not useful - use the `message` to provide additional information).
- `299`, responding with 299, means the configuration has been accepted but a restart is required to apply it.

## Data collection

data collection is defined as a series of `BEGIN` -> `SET` -> `END` lines

> BEGIN type.id [microseconds]

-   `type.id`

    is the unique identification of the chart (as given in `CHART`)

-   `microseconds`

    is the number of microseconds since the last update of the chart. It is optional.

    Under heavy system load, the system may have some latency transferring
    data from the plugins to Netdata via the pipe. This number improves
    accuracy significantly, since the plugin is able to calculate the
    duration between its iterations better than Netdata.

    The first time the plugin is started, no microseconds should be given
    to Netdata.

> SET id = value

-   `id`

    is the unique identification of the dimension (of the chart just began)

-   `value`

    is the collected value, only integer values are collected. If you want to push fractional values, multiply this value by 100 or 1000 and set the `DIMENSION` divider to 1000.

> END

  END does not take any parameters, it commits the collected values for all dimensions to the chart. If a dimensions was not `SET`, its value will be empty for this commit.

More `SET` lines may appear to update all the dimensions of the chart.
All of them in one `BEGIN` -> `END` block.

All `SET` lines within a single `BEGIN` -> `END` block have to refer to the
same chart.

If more charts need to be updated, each chart should have its own
`BEGIN` -> `SET` -> `END` block.

If, for any reason, a plugin has issued a `BEGIN` but wants to cancel it,
it can issue a `FLUSH`. The `FLUSH` command will instruct Netdata to ignore
all the values collected since the last `BEGIN` command.

If a plugin does not behave properly (outputs invalid lines, or does not
follow these guidelines), will be disabled by Netdata.

### collected values

Netdata will collect any **signed** value in the 64bit range:
`-9.223.372.036.854.775.808` to `+9.223.372.036.854.775.807`

If a value is not collected, leave it empty, like this:

`SET id =`

or do not output the line at all.

## Modular Plugins

1.  **python**, use `python.d.plugin`, there are many examples in the [python.d
    directory](/src/collectors/python.d.plugin/README.md)

    python is ideal for Netdata plugins. It is a simple, yet powerful way to collect data, it has a very small memory footprint, although it is not the most CPU efficient way to do it.

2.  **BASH**, use `charts.d.plugin`, there are many examples in the [charts.d
    directory](/src/collectors/charts.d.plugin/README.md)

    BASH is the simplest scripting language for collecting values. It is the less efficient though in terms of CPU resources. You can use it to collect data quickly, but extensive use of it might use a lot of system resources.

3.  **C**

    Of course, C is the most efficient way of collecting data. This is why Netdata itself is written in C.

## Writing Plugins Properly

There are a few rules for writing plugins properly:

1.  Respect system resources

    Pay special attention to efficiency:

    -   Initialize everything once, at the beginning. Initialization is not an expensive operation. Your plugin will most probably be started once and run forever. So, do whatever heavy operation is needed at the beginning, just once.
    -   Do the absolutely minimum while iterating to collect values repeatedly.
    -   If you need to connect to another server to collect values, avoid re-connects if possible. Connect just once, with keep-alive (for HTTP) enabled and collect values using the same connection.
    -   Avoid any CPU or memory heavy operation while collecting data. If you control memory allocation, avoid any memory allocation while iterating to collect values.
    -   Avoid running external commands when possible. If you are writing shell scripts avoid especially pipes (each pipe is another fork, a very expensive operation).

2.  The best way to iterate at a constant pace is this pseudo code:

```js
   var update_every = argv[1] * 1000; /* seconds * 1000 = milliseconds */

   readConfiguration();

   if(!verifyWeCanCollectValues()) {
      print("DISABLE");
      exit(1);
   }

   createCharts(); /* print CHART and DIMENSION statements */

   var loops = 0;
   var last_run = 0;
   var next_run = 0;
   var dt_since_last_run = 0;
   var now = 0;

   while(true) {
       /* find the current time in milliseconds */
       now = currentTimeStampInMilliseconds();

       /*
        * find the time of the next loop
        * this makes sure we are always aligned
        * with the Netdata daemon
        */
       next_run = now - (now % update_every) + update_every;

       /*
        * wait until it is time
        * it is important to do it in a loop
        * since many wait functions can be interrupted
        */
       while( now < next_run ) {
           sleepMilliseconds(next_run - now);
           now = currentTimeStampInMilliseconds();
       }

       /* calculate the time passed since the last run */
       if ( loops > 0 )
           dt_since_last_run = (now - last_run) * 1000; /* in microseconds */

       /* prepare for the next loop */
       last_run = now;
       loops++;

       /* do your magic here to collect values */
       collectValues();

       /* send the collected data to Netdata */
       printValues(dt_since_last_run); /* print BEGIN, SET, END statements */
   }
```

   Using the above procedure, your plugin will be synchronized to start data collection on steps of `update_every`. There will be no need to keep track of latencies in data collection.

   Netdata interpolates values to second boundaries, so even if your plugin is not perfectly aligned it does not matter. Netdata will find out. When your plugin works in increments of `update_every`, there will be no gaps in the charts due to the possible cumulative micro-delays in data collection. Gaps will only appear if the data collection is really delayed.

3.  If you are not sure of memory leaks, exit every one hour. Netdata will re-start your process.

4.  If possible, try to autodetect if your plugin should be enabled, without any configuration.


