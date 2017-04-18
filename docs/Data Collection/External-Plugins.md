netdata plugins
===============

Any program that can print a few values to its standard output can become
a netdata plugin.

There are 7 lines netdata parses. lines starting with:

- `CHART` - create a chart
- `DIMENSION` - add a dimension to the chart just created
- `BEGIN` - initialize data collection for a chart
- `SET` - set the value of a dimension for the initialized chart
- `END` - complete data collection for the initialized chart
- `FLUSH` - ignore the last collected values
- `DISABLE` - disable this plugin

a single program can produce any number of charts with any number of dimensions
each.

charts can also be added any time (not just the beginning).

### command line parameters

The plugin should accept just **one** parameter: **the number of seconds it is
expected to update the values for its charts**. The value passed by netdata
to the plugin is controlled via its configuration file (so there is not need
for the plugin to handle this configuration option). Check **[External Plugins Configuration](https://github.com/firehol/netdata/wiki/Configuration#external-plugins)**

The script can overwrite the update frequency. For example, the server may
request per second updates, but the script may overwrite this to one update
every 5 seconds.

### environment variables

There are a few environment variables that are set by `netdata` and are
available for the plugin to use.

variable|description
:------:|:----------
`NETDATA_CONFIG_DIR`|The directory where all netdata related configuration should be stored. If the plugin requires custom configuration, this is the place to save it.
`NETDATA_PLUGINS_DIR`|The directory where all netdata plugins are stored.
`NETDATA_WEB_DIR`|The directory where the web files of netdata are saved.
`NETDATA_CACHE_DIR`|The directory where the cache files of netdata are stored. Use this directory if the plugin requires a place to store data. A new directory should be created for the plugin for this purpose, inside this directory.
`NETDATA_LOG_DIR`|The directory where the log files are stored. By default the `stderr` output of the plugin will be saved in the `error.log` file of netdata.
`NETDATA_HOST_PREFIX`|This is used in environments where system directories like `/sys` and `/proc` have to be accessed at a different path.
`NETDATA_DEBUG_FLAGS`|This is a number (probably in hex starting with `0x`), that enables certain netdata debugging features. Check **[[Tracing Options]]** for more information.
`NETDATA_UPDATE_EVERY`|The minimum number of seconds between chart refreshes. This is like the **internal clock** of netdata (it is user configurable, defaulting to `1`). There is no meaning for a plugin to update its values more frequently than this number of seconds.


# the output of the plugin

The plugin should output instructions for netdata to its output (`stdout`).

## DISABLE

`DISABLE` will disable this plugin. This will prevent netdata from restarting the plugin. You can also exit with the value `1` to have the same effect.

## CHART

`CHART` defines a new chart.

the template is:

> CHART type.id name title units [family [category [charttype [priority [update_every]]]]]

 where:
  - `type.id`

    uniquely identifies the chart,
    this is what will be needed to add values to the chart

  - `name`

    is the name that will be presented to the user instead of `id` in `type.id`. This means that only the `id` part of `type.id` is changed. When a name has been given, the chart is index (and can be referred) as both `type.id` and `type.name`. You can set name to `''`, or `null`, or `(null)` to disable it.

  - `title`

    the text above the chart

  - `units`

    the label of the vertical axis of the chart,
    all dimensions added to a chart should have the same units
    of measurement

  - `family`

    is used to group charts together
    (for example all eth0 charts should say: eth0),
    if empty or missing, the `id` part of `type.id` will be used

  - `category`

    the section under which the chart will appear
    (for example mem.ram should appear in the 'system' section),
    the special word 'none' means: do not show this chart on the home page,
    if empty or missing, the `type` part of `type.id` will be used

  - `charttype`

    one of `line`, `area` or `stacked`,
    if empty or missing, the `line` will be used

  - `priority`

    is the relative priority of the charts as rendered on the web page,
    lower numbers make the charts appear before the ones with higher numbers,
    if empty or missing, `1000` will be used

  - `update_every`

    overwrite the update frequency set by the server,
    if empty or missing, the user configured value will be used


## DIMENSION

`DIMENSION` defines a new dimension for the chart

the template is:

> DIMENSION id [name [algorithm [multiplier [divisor [hidden]]]]]

 where:

  - `id`

    the `id` of this dimension (it is a text value, not numeric),
    this will be needed later to add values to the dimension

  - `name`

    the name of the dimension as it will appear at the legend of the chart,
    if empty or missing the `id` will be used

  - `algorithm`

    one of:

    * `absolute`

      the value is to drawn as-is (interpolated to second boundary),
      if `algorithm` is empty, invalid or missing, `absolute` is used

    * `incremental`

      the value increases over time,
      the difference from the last value is presented in the chart,
      the server interpolates the value and calculates a per second figure

    * `percentage-of-absolute-row`

      the % of this value compared to the total of all dimensions

    * `percentage-of-incremental-row`

      the % of this value compared to the incremental total of
      all dimensions

  - `multiplier`

    an integer value to multiply the collected value,
    if empty or missing, `1` is used

  - `divisor`

    an integer value to divide the collected value,
    if empty or missing, `1` is used

  - `hidden`

    giving the keyword `hidden` will make this dimension hidden,
    it will take part in the calculations but will not be presented in the chart


## data collection

data collection is defined as a series of `BEGIN` -> `SET` -> `END` lines

> BEGIN type.id [microseconds]

  - `type.id`

    is the unique identification of the chart (as given in `CHART`)

  - `microseconds`

    is the number of microseconds since the last update of the chart. It is optional.

    Under heavy system load, the system may have some latency transferring
    data from the plugins to netdata via the pipe. This number improves
    accuracy significantly, since the plugin is able to calculate the
    duration between its iterations better than netdata.

    The first time the plugin is started, no microseconds should be given
    to netdata.

> SET id = value

   - `id`

     is the unique identification of the dimension (of the chart just began)

   - `value`

     is the collected value

> END

  END does not take any parameters, it commits the collected values for all dimensions to the chart. If a dimensions was not `SET`, its value will be empty for this commit.

More `SET` lines may appear to update all the dimensions of the chart.
All of them in one `BEGIN` -> `END` block.

All `SET` lines within a single `BEGIN` -> `END` block have to refer to the
same chart.

If more charts need to be updated, each chart should have its own
`BEGIN` -> `SET` -> `END` block.

If, for any reason, a plugin has issued a `BEGIN` but wants to cancel it,
it can issue a `FLUSH`. The `FLUSH` command will instruct netdata to ignore
all the values collected since the last `BEGIN` command.

If a plugin does not behave properly (outputs invalid lines, or does not
follow these guidelines), will be disabled by netdata.

### collected values

netdata will collect any **signed** value in the 64bit range:
`-9.223.372.036.854.775.808` to `+9.223.372.036.854.775.807`

Internally, all calculations are made using 128 bit double precision and are
stored in 30 bits as floating point.

If a value is not collected, leave it empty, like this:

`SET id = `

or do not output the line at all.

# Skeleton Plugins

1. **node.js**, use `node.d.plugin`, there are a few examples in the [node.d directory](https://github.com/firehol/netdata/tree/master/node.d)
2. **BASH**, use `charts.d.plugin`, there are many examples in the [charts.d directory](https://github.com/firehol/netdata/tree/master/charts.d)
3. **C**, use [apps.plugin](https://github.com/firehol/netdata/blob/master/src/apps_plugin.c#L2420)
4. **Python**, check this example [in issue 206](https://github.com/firehol/netdata/issues/206#issuecomment-208038333)

# Writing Plugins Properly

There are a few rules for writing plugins properly:

1. Respect system resources

   Pay special attention to efficiency:

      - Initialize everything once, at the beginning. Initialization is not an expensive operation. Your plugin will most probably be started once and run forever. So, do whatever heavy operation is needed at the beginning, just once.
      - Do the absolutely minimum while iterating to collect values repeatedly.
      - If you need to connect to another server to collect values, avoid re-connects if possible. Connect just once, with keep-alive enabled and collect values using the same connection.
      - Avoid any CPU or memory heavy operation while collecting data.
      - Avoid running external commands when possible.

2. The best way to iterate at a constant pace is this pseudo code:

   ```js
   update_every = argv[1] * 1000 // seconds * 1000 = milliseconds

   readConfiguration()
   
   if(!verifyWeCanCollectValues()) {
      print "DISABLE"
      exit(1)
   }

   createCharts()

   count = 0
   last_run = 0
   next_run = currentTimeStampInMilliseconds()

   FOREVER {
       now = currentTimeStampInMilliseconds()
   
       if ( next_run <= now ) {
           count++

           while ( next_run <= now )
              next_run += update_every

           dt_since_last_run = (now - last_run) * 1000 // in microseconds
           last_run = now

           collectValues()
           printValues() // output dt_since_last_run in BEGIN, only if count > 1
       }

       sleepMilliseconds(update_every / 10)
   }
   ```

    Using the above procedure, your plugin will be synchronized to start data collection on steps of `update_every`. There will be no need to keep track of latencies in data collection.

    Netdata interpolates values to second boundaries, so even if your plugin is not perfectly aligned it does not matter. Netdata will find out. When your plugin works in increments of `update_every`, there will be no gaps in the charts due to the possible cumulative micro-delays in data collection. Gaps will only appear if the data collection is really delayed.

3. If you are not sure of memory leaks, exit every one hour. Netdata will re-start your process.

4. If possible, try to autodetect if your plugin should be enabled, without any configuration.

