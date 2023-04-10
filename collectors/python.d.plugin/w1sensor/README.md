<!--
title: "1-Wire Sensors monitoring with Netdata"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/w1sensor/README.md"
sidebar_label: "1-Wire sensors"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/Remotes/Devices"
-->

# 1-Wire Sensors collector

Monitors sensor temperature.

On Linux these are supported by the wire, w1_gpio, and w1_therm modules.
Currently temperature sensors are supported and automatically detected.

Charts are created dynamically based on the number of detected sensors.

## Configuration

Edit the `python.d/w1sensor.conf` configuration file using `edit-config` from the Netdata [config
directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/w1sensor.conf
```

An example of a working configuration is:

```yaml
# netdata python.d.plugin configuration for w1sensor

# ----------------------------------------------------------------------
# Global Variables
# These variables set the defaults for all JOBs, however each JOB
# may define its own, overriding the defaults.

# update_every sets the default data collection frequency.
# If unset, the python.d.plugin default is used.
update_every: 5

# priority controls the order of charts at the netdata dashboard.
# Lower numbers move the charts towards the top of the page.
# If unset, the default for python.d.plugin is used.
priority: 60000

# penalty indicates whether to apply penalty to update_every in case of failures.
# Penalty will increase every 5 failed updates in a row. Maximum penalty is 10 minutes.
penalty: yes

# autodetection_retry sets the job re-check interval in seconds.
# The job is not deleted if check fails.
# Attempts to start the job are made once every autodetection_retry.
# This feature is disabled by default.
autodetection_retry: 0

# ----------------------------------------------------------------------
# JOBS (data collection sources)
#
# The default JOBS share the same *name*. JOBS with the same name
# are mutually exclusive. Only one of them will be allowed running at
# any time. This allows autodetection to try several alternatives and
# pick the one that works.
#
# Any number of jobs is supported.
#
# All python.d.plugin JOBS (for all its modules) support a set of
# predefined parameters. These are:
#
job_name:
    name: myname            # the JOB's name as it will appear at the
                            # dashboard (by default is the job_name)
                            # JOBs sharing a name are mutually exclusive
    update_every: 5         # the JOB's data collection frequency
    priority: 60000         # the JOB's order on the dashboard
    penalty: yes            # the JOB's penalty
    autodetection_retry: 0  # the JOB's re-check interval in seconds
#
# Additionally to the above, example also supports the following:
#
# name_<1-Wire id>: '<human readable name>'
# This allows associating a human readable name with a sensor's 1-Wire
# identifier. Example:
# name_00000022276e: 'Machine room'
# name_00000022298f: 'Rack 12'
```

### Troubleshooting

To troubleshoot issues with the `w1sensor` module, run the `python.d.plugin` with the debug option enabled. The 
output will give you the output of the data collection job or error messages on why the collector isn't working.

First, navigate to your plugins directory, usually they are located under `/usr/libexec/netdata/plugins.d/`. If that's 
not the case on your system, open `netdata.conf` and look for the setting `plugins directory`. Once you're in the 
plugin's directory, switch to the `netdata` user.

```bash
cd /usr/libexec/netdata/plugins.d/
sudo su -s /bin/bash netdata
```

Now you can manually run the `w1sensor` module in debug mode:

```bash
./python.d.plugin w1sensor debug trace
```

