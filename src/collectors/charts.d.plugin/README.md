# charts.d.plugin

`charts.d.plugin` is a Netdata external plugin. It is an **orchestrator** for data collection modules written in `BASH` v4+.

1. It runs as an independent process `ps fax` shows it
2. It is started and stopped automatically by Netdata
3. It communicates with Netdata via a unidirectional pipe (sending data to the `netdata` daemon)
4. Supports any number of data collection **modules**

To better understand the guidelines and the API behind our External plugins, please have a look at the [Introduction to External plugins](/src/plugins.d/README.md) prior to reading this page.

`charts.d.plugin` has been designed so that the actual script that will do data collection will be permanently in
memory, collecting data with as little overheads as possible
(i.e. initialize once, repeatedly collect values with minimal overhead).

`charts.d.plugin` looks for scripts in `/usr/lib/netdata/charts.d`.
The scripts should have the filename suffix: `.chart.sh`.

By default, `charts.d.plugin` is not included as part of the install when using [our official native DEB/RPM packages](/packaging/installer/methods/packages.md). You can install it by installing the `netdata-plugin-chartsd` package.

## Configuration

`charts.d.plugin` itself can be [configured](/docs/netdata-agent/configuration/README.md#edit-configuration-files) using the configuration file `/etc/netdata/charts.d.conf`. This file is also a BASH script.

In this file, you can place statements like this:

```text
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

1. The module script must be called `X.chart.sh` and placed in `/usr/libexec/netdata/charts.d`.

2. If the module needs a configuration, it should be called `X.conf` and placed in `/etc/netdata/charts.d`.  
   The configuration file `X.conf` is also a BASH script itself.  
   You can edit the default files supplied by Netdata, by editing `/etc/netdata/edit-config charts.d/X.conf`, where `X` is the name of the module.

3. All functions and global variables defined in the script and its configuration, must begin with `X_`.

4. The following functions must be defined:
   - `X_check()` - returns 0 or 1 depending on whether the module is able to run or not
     (following the standard Linux command line return codes: 0 = OK, the collector can operate and 1 = FAILED,
     the collector cannot be used).

   - `X_create()` - creates the Netdata charts (commands `CHART` and `DIMENSION`).
     The return value does matter: 0 = OK, 1 = FAILED.

   - `X_update()` - collects the values for the defined charts (commands `BEGIN`, `SET`, `END`).
     The return value also matters: 0 = OK, 1 = FAILED.

5. The following global variables are available to be set:
   - `X_update_every` - is the data collection frequency for the module script, in seconds.

The module script may use more functions or variables. But all of them must begin with `X_`.

### X_check()

The purpose of the BASH function `X_check()` is to check if the module can collect data (or check its config).

For example, if the module is about monitoring a local mysql database, the `X_check()` function may attempt to
connect to a local mysql database to find out if it can read the values it needs.

`X_check()` is run only once for the lifetime of the module.

### X_create()

The purpose of the BASH function `X_create()` is to create the charts and dimensions using the standard Netdata
plugin guidelines.

`X_create()` will be called just once and only after `X_check()` was successful.
You can however call it yourself when there is need for it (for example to add a new dimension to an existing chart).

A non-zero return value will disable the collector.

### X_update()

`X_update()` will be called repeatedly every `X_update_every` seconds, to collect new values and send them to Netdata,
following the Netdata plugin guidelines.

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
You can keep the generated id in a BASH associative array to have the values available in `X_update()`, like this:

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

1. Decide a new name for the new charts.d instance: example `charts2.d`.

2. Create/edit the files `/etc/netdata/charts.d.conf` and `/etc/netdata/charts2.d.conf` and enable / disable the
   module you want each to run. Remember to set `enable_all_charts="no"` to both of them, and enable the individual
   modules for each.

3. link `/usr/libexec/netdata/plugins.d/charts.d.plugin` to `/usr/libexec/netdata/plugins.d/charts2.d.plugin`.
   Netdata will spawn a new charts.d process.

Execute the above in this order, since Netdata will (by default) attempt to start new plugins soon after they are
created in `/usr/libexec/netdata/plugins.d/`.

## Complete Working Example

The following is a complete, working example of a charts.d plugin module. This example demonstrates all the required functions and shows how to create multiple charts.

### The Module Script: `example.chart.sh`

This file should be placed at `/usr/libexec/netdata/charts.d/example.chart.sh`:

```sh
# no need for shebang - this file is loaded from charts.d.plugin
# SPDX-License-Identifier: GPL-3.0-or-later
# shellcheck shell=bash

# if this chart is called X.chart.sh, then all functions and global variables
# must start with X_

# _update_every is a special variable - it holds the number of seconds
# between the calls of the _update() function
example_update_every=

# the priority is used to sort the charts on the dashboard
# 1 = the first chart
example_priority=150000

# to enable this chart, you have to set this to 12345
# (just a demonstration for something that needs to be checked)
example_magic_number=

# global variables to store our collected data
# remember: they need to start with the module name example_
example_value1=
example_value2=
example_value3=
example_value4=
example_last=0
example_count=0

example_get() {
  # do all the work to collect / calculate the values
  # for each dimension
  #
  # Remember:
  # 1. KEEP IT SIMPLE AND SHORT
  # 2. AVOID FORKS (avoid piping commands)
  # 3. AVOID CALLING TOO MANY EXTERNAL PROGRAMS
  # 4. USE LOCAL VARIABLES (global variables may overlap with other modules)

  example_value1=$RANDOM
  example_value2=$RANDOM
  example_value3=$RANDOM
  example_value4=$((8192 + (RANDOM * 16383 / 32767)))

  if [ $example_count -gt 0 ]; then
    example_count=$((example_count - 1))

    [ $example_last -gt 16383 ] && example_value4=$((example_last + (RANDOM * ((32767 - example_last) / 2) / 32767)))
    [ $example_last -le 16383 ] && example_value4=$((example_last - (RANDOM * (example_last / 2) / 32767)))
  else
    example_count=$((1 + (RANDOM * 5 / 32767)))

    if [ $example_last -gt 16383 ] && [ $example_value4 -gt 16383 ]; then
      example_value4=$((example_value4 - 16383))
    fi
    if [ $example_last -le 16383 ] && [ $example_value4 -lt 16383 ]; then
      example_value4=$((example_value4 + 16383))
    fi
  fi
  example_last=$example_value4

  # this should return:
  #  - 0 to send the data to netdata
  #  - 1 to report a failure to collect the data

  return 0
}

# _check is called once, to find out if this chart should be enabled or not
example_check() {
  # this should return:
  #  - 0 to enable the chart
  #  - 1 to disable the chart

  # check something
  [ "${example_magic_number}" != "12345" ] && error "manual configuration required: you have to set example_magic_number=$example_magic_number in example.conf to start example chart." && return 1

  # check that we can collect data
  example_get || return 1

  return 0
}

# _create is called once, to create the charts
example_create() {
  # create the chart with 3 dimensions
  cat << EOF
CHART example.random '' "Random Numbers Stacked Chart" "% of random numbers" random random stacked $((example_priority)) $example_update_every '' '' 'example'
DIMENSION random1 '' percentage-of-absolute-row 1 1
DIMENSION random2 '' percentage-of-absolute-row 1 1
DIMENSION random3 '' percentage-of-absolute-row 1 1
CHART example.random2 '' "A random number" "random number" random random area $((example_priority + 1)) $example_update_every '' '' 'example'
DIMENSION random '' absolute 1 1
EOF

  return 0
}

# _update is called continuously, to collect the values
example_update() {
  # the first argument to this function is the microseconds since last update
  # pass this parameter to the BEGIN statement (see below).

  example_get || return 1

  # write the result of the work.
  cat << VALUESEOF
BEGIN example.random $1
SET random1 = $example_value1
SET random2 = $example_value2
SET random3 = $example_value3
END
BEGIN example.random2 $1
SET random = $example_value4
END
VALUESEOF

  return 0
}
```

### The Configuration File: `example.conf`

This file should be placed at `/etc/netdata/charts.d/example.conf`:

```sh
# example_magic_number is required to enable the example chart
# set it to 12345 to activate this collector
example_magic_number=12345

# update every N seconds
example_update_every=1
```

To edit the configuration file, run:

```sh
sudo ./edit-config charts.d/example.conf
```

### How to Test the Example

To test the example collector in debug mode, run:

```sh
# become user netdata
sudo su -s /bin/sh netdata

# run the plugin in debug mode
/usr/libexec/netdata/plugins.d/charts.d.plugin debug 1 example
```

This will:

1. Run `charts.d.plugin` in debug mode
2. Set the update frequency to 1 second
3. Load only the `example` module

You should see output similar to:

```
CHART example.random '' "Random Numbers Stacked Chart" ...
DIMENSION random1 '' percentage-of-absolute-row 1 1
DIMENSION random2 '' percentage-of-absolute-row 1 1
DIMENSION random3 '' percentage-of-absolute-row 1 1
CHART example.random2 '' "A random number" ...
DIMENSION random '' absolute 1 1
BEGIN example.random 1000000
SET random1 = 12345
SET random2 = 23456
SET random3 = 34567
END
BEGIN example.random2 1000000
SET random = 28192
END
```

### Key Points from the Example

1. **Naming Convention**: All functions and variables start with `example_` (matching the module name `example.chart.sh`)

2. **`example_check()`**: Verifies configuration (`example_magic_number` must be set to `12345`) and tests data collection

3. **`example_create()`**: Defines two charts using `CHART` and `DIMENSION` commands
   - A stacked chart showing random numbers as percentages
   - An area chart showing a single random number

4. **`example_update()`**: Collects data via `example_get()` and sends it to Netdata using `BEGIN`, `SET`, and `END` commands

5. **Helper Function**: `example_get()` does the actual data collection, following best practices (no forks, minimal external calls, local variables)
