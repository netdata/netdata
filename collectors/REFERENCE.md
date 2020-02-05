# Collector configuration reference

## Enabling and disabling plugins

You can enable or disable individual plugins by opening `netdata.conf` and scrolling down to the `[plugins]` section.
This section features a list of Netdata's plugins, with a boolean setting to enable or disable them. The exception is
`statsd.plugin`, which has its own `[statsd]` section. Your `[plugins]` section should look similar to this:

```conf
[plugins]
	# PATH environment variable = /usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/var/lib/snapd/snap/bin:/sbin:/usr/sbin:/usr/local/bin:/usr/local/sbin
	# PYTHONPATH environment variable = 
	# proc = yes
	# diskspace = yes
	# cgroups = yes
	# tc = yes
	# idlejitter = yes
	# enable running new plugins = yes
	# check for new plugins every = 60
	# slabinfo = no
	# fping = yes
	# ioping = yes
	# node.d = yes
	# python.d = yes
	# go.d = yes
	# apps = yes
	# perf = yes
	# charts.d = yes
```

By default, most plugins are enabled, so you don't need to explicity enable them to use their collectors. To enable or
disable any specific plugin, remove the comment (`#`) and change the boolean setting to `yes` or `no`.

All **external plugins** are managed by [plugins.d](plugins.d/), which provides additional management options.

## Enabling, configuring, and disabling modules

Most **modules** come with **auto-detection**, configured to work out-of-the-box on popular operating systems with the
default settings.

However, there are cases that auto-detection fails. Usually the reason is that the applications to be monitored do not
allow Netdata to connect. In most of the cases, allowing the user `netdata` from `localhost` to connect and collect
metrics, will automatically enable data collection for the application in question (it will require a Netdata restart).

You can verify Netdata **external plugins and their modules** are able to collect metrics, following this procedure:

```sh
# become user netdata
sudo su -s /bin/bash netdata

# execute the plugin in debug mode, for a specific module.
# example for the python plugin, mysql module:
/usr/libexec/netdata/plugins.d/python.d.plugin 1 debug trace mysql
```

Similarly, you can use `charts.d.plugin` for BASH plugins and `node.d.plugin` for node.js plugins. Other plugins (like
`apps.plugin`, `freeipmi.plugin`, `fping.plugin`, `ioping.plugin`, `nfacct.plugin`, `xenstat.plugin`, `perf.plugin`,
`slabinfo.plugin`) use the native Netdata plugin API and can be run directly.

If you need to configure a Netdata plugin or module, all user supplied configuration is kept at `/etc/netdata` while the
stock versions of all files is at `/usr/lib/netdata/conf.d`. To copy a stock file and edit it, run
`/etc/netdata/edit-config`. Running this command without an argument, will list the available stock files.

Each file should provide plenty of examples and documentation about each module and plugin.

## Internal plugins

Each of the internal plugins runs as a thread inside the `netdata` daemon.
Once this thread has started, the plugin may spawn additional threads according to its design.

### Internal plugins API

The internal data collection API consists of the following calls:

```c
collect_data() {
    // collect data here (one iteration)

    collected_number collected_value = collect_a_value();

    // give the metrics to Netdata

    static RRDSET *st = NULL; // the chart
    static RRDDIM *rd = NULL; // a dimension attached to this chart

    if(unlikely(!st)) {
        // we haven't created this chart before
        // create it now
        st = rrdset_create_localhost(
                "type"
                , "id"
                , "name"
                , "family"
                , "context"
                , "Chart Title"
                , "units"
                , "plugin-name"
                , "module-name"
                , priority
                , update_every
                , chart_type
        );

        // attach a metric to it
        rd = rrddim_add(st, "id", "name", multiplier, divider, algorithm);
    }
    else {
        // this chart is already created
        // let Netdata know we start a new iteration on it
        rrdset_next(st);
    }

    // give the collected value(s) to the chart
    rrddim_set_by_pointer(st, rd, collected_value);

    // signal Netdata we are done with this iteration
    rrdset_done(st);
}
```

Of course, Netdata has a lot of libraries to help you also in collecting the metrics. The best way to find your way
through this, is to examine what other similar plugins do.

## External Plugins

**External plugins** use the API and are managed by [plugins.d](plugins.d/).

## Write a custom collector

You can add custom collectors by following the [external plugins guide](../collectors/plugins.d/).
