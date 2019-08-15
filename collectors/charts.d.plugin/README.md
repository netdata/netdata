# charts.d.plugin

`charts.d.plugin` is a Netdata external plugin. It is an **orchestrator** for data collection modules written in `BASH` v4+.

1.  It runs as an independent process `ps fax` shows it
2.  It is started and stopped automatically by Netdata
3.  It communicates with Netdata via a unidirectional pipe (sending data to the `netdata` daemon)
4.  Supports any number of data collection **modules**

`charts.d.plugin` has been designed so that the actual script that will do data collection will be permanently in
memory, collecting data with as little overheads as possible
(i.e. initialize once, repeatedly collect values with minimal overhead).

`charts.d.plugin` looks for scripts in `/usr/lib/netdata/charts.d`.
The scripts should have the filename suffix: `.chart.sh`.

## Configuration

`charts.d.plugin` itself can be configured using the configuration file `/etc/netdata/charts.d.conf`
(to edit it on your system run `/etc/netdata/edit-config charts.d.conf`). This file is also a BASH script.

In this file, you can place statements like this:

```
enable_all_charts="yes"
X="yes"
Y="no"
```

where `X` and `Y` are the names of individual charts.d collector scripts.
When set to `yes`, charts.d will evaluate the collector script (see below).
When set to `no`, charts.d will ignore the collector script.

The variable `enable_all_charts` sets the default enable/disable state for all charts.

## A charts.d module

A `charts.d.plugin` module is a BASH script defining a few functions.

For a module called `X`, the following criteria must be met:

1.  The module script must be called `X.chart.sh` and placed in `/usr/libexec/netdata/charts.d`.

2.  If the module needs a configuration, it should be called `X.conf` and placed in `/etc/netdata/charts.d`.
    The configuration file `X.conf` is also a BASH script itself.
    To edit the default files supplied by Netdata, run `/etc/netdata/edit-config charts.d/X.conf`,
    where `X` is the name of the module.

3.  All functions and global variables defined in the script and its configuration, must begin with `X_`.

4.  The following functions must be defined:

    -   `X_check()` - returns 0 or 1 depending on whether the module is able to run or not
         (following the standard Linux command line return codes: 0 = OK, the collector can operate and 1 = FAILED,
         the collector cannot be used).

    -   `X_create()` - creates the Netdata charts, following the standard Netdata plugin guides as described in
         **[External Plugins](../plugins.d/)** (commands `CHART` and `DIMENSION`).
         The return value does matter: 0 = OK, 1 = FAILED.

    -   `X_update()` - collects the values for the defined charts, following the standard Netdata plugin guides
         as described in **[External Plugins](../plugins.d/)** (commands `BEGIN`, `SET`, `END`).
         The return value also matters: 0 = OK, 1 = FAILED.

5.  The following global variables are available to be set:
    -   `X_update_every` - is the data collection frequency for the module script, in seconds.

The module script may use more functions or variables. But all of them must begin with `X_`.

The standard Netdata plugin variables are also available (check **[External Plugins](../plugins.d/)**). 

### X_check()

The purpose of the BASH function `X_check()` is to check if the module can collect data (or check its config).

For example, if the module is about monitoring a local mysql database, the `X_check()` function may attempt to
connect to a local mysql database to find out if it can read the values it needs.

`X_check()` is run only once for the lifetime of the module.

### X_create()

The purpose of the BASH function `X_create()` is to create the charts and dimensions using the standard Netdata
plugin guides (**[External Plugins](../plugins.d/)**).

`X_create()` will be called just once and only after `X_check()` was successful.
You can however call it yourself when there is need for it (for example to add a new dimension to an existing chart).

A non-zero return value will disable the collector.

### X_update()

`X_update()` will be called repeatedly every `X_update_every` seconds, to collect new values and send them to Netdata,
following the Netdata plugin guides (**[External Plugins](../plugins.d/)**).

The function will be called with one parameter: microseconds since the last time it was run. This value should be
appended to the `BEGIN` statement of every chart updated by the collector script.

A non-zero return value will disable the collector.

### Useful functions charts.d provides

Module scripts can use the following charts.d functions:

#### require_cmd command

`require_cmd()` will check if a command is available in the running system.

For example, your `X_check()` function may use it like this:

```sh
mysql_check() {
    require_cmd mysql || return 1
    return 0
}
```

Using the above, if the command `mysql` is not available in the system, the `mysql` module will be disabled.

#### fixid "string"

`fixid()` will get a string and return a properly formatted id for a chart or dimension.

This is an expensive function that should not be used in `X_update()`.
You can keep the generated id in a BASH associative array to have the values availables in `X_update()`, like this:

```sh
declare -A X_ids=()
X_create() {
   local name="a very bad name for id"

   X_ids[$name]="$(fixid "$name")"
}

X_update() {
   local microseconds="$1"

   ...
   local name="a very bad name for id"
   ...

   echo "BEGIN ${X_ids[$name]} $microseconds"
   ...
}
```

### Debugging your collectors

You can run `charts.d.plugin` by hand with something like this:

```sh
# become user netdata
sudo su -s /bin/sh netdata

# run the plugin in debug mode
/usr/libexec/netdata/plugins.d/charts.d.plugin debug 1 X Y Z
```

Charts.d will run in `debug` mode, with an update frequency of `1`, evaluating only the collector scripts
`X`, `Y` and `Z`. You can define zero or more module scripts. If none is defined, charts.d will evaluate all
module scripts available.

Keep in mind that if your configs are not in `/etc/netdata`, you should do the following before running
`charts.d.plugin`:

```sh
export NETDATA_USER_CONFIG_DIR="/path/to/etc/netdata"
```

Also, remember that Netdata runs `chart.d.plugin` as user `netdata` (or any other user the `netdata` process is configured to run as).

## Running multiple instances of charts.d.plugin

`charts.d.plugin` will call the `X_update()` function one after another. This means that a delay in collector `X`
will also delay the collection of `Y` and `Z`.

You can have multiple `charts.d.plugin` running to overcome this problem.

This is what you need to do:

1.  Decide a new name for the new charts.d instance: example `charts2.d`.

2.  Create/edit the files `/etc/netdata/charts.d.conf` and `/etc/netdata/charts2.d.conf` and enable / disable the
    module you want each to run. Remember to set `enable_all_charts="no"` to both of them, and enable the individual
    modules for each.

3.  link `/usr/libexec/netdata/plugins.d/charts.d.plugin` to `/usr/libexec/netdata/plugins.d/charts2.d.plugin`.
    Netdata will spawn a new charts.d process.

Execute the above in this order, since Netdata will (by default) attempt to start new plugins soon after they are
created in `/usr/libexec/netdata/plugins.d/`.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fcharts.d.plugin%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
