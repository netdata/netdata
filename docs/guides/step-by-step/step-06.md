<!--
title: "Step 6. Collect metrics from more services and apps"
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/guides/step-by-step/step-06.md
-->

# Step 6. Collect metrics from more services and apps

When Netdata _starts_, it auto-detects dozens of **data sources**, such as database servers, web servers, and more.

To auto-detect and collect metrics from a source you just installed, you need to restart Netdata using `sudo systemctl
restart netdata`, or the [appropriate method](https://github.com/netdata/netdata/blob/master/docs/configure/start-stop-restart.md) for your system.

However, auto-detection only works if you installed the source using its standard installation
procedure. If Netdata isn't collecting metrics after a restart, your source probably isn't configured
correctly.

Check out the [collectors that come pre-installed with Netdata](https://github.com/netdata/netdata/blob/master/collectors/COLLECTORS.md) to find the module for the
source you want to monitor.

## What you'll learn in this step

We'll begin with an overview on Netdata's collector architecture, and then dive into the following:

-   [Netdata's collector architecture](#netdatas-collector-architecture)
-   [Enable and disable plugins](#enable-and-disable-plugins)
-   [Enable the Nginx collector as an example](#example-enable-the-nginx-collector)

## Netdata's collector architecture

Many Netdata users never have to configure collector or worry about which plugin orchestrator they want to use.

But, if you want to configure collector or write a collector for your custom source, it's important to understand the
underlying architecture.

By default, Netdata collects a lot of metrics every second using any number of discrete collector. Collectors, in turn,
are organized and manged by plugins. **Internal** plugins collect system metrics, **external** plugins collect
non-system metrics, and **orchestrator** plugins group individual collectors together based on the programming language
they were built in.

These modules are primarily written in [Go](https://github.com/netdata/go.d.plugin/blob/master/README.md) (`go.d`) and
[Python](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/README.md), although some use [Bash](https://github.com/netdata/netdata/blob/master/collectors/charts.d.plugin/README.md)
(`charts.d`).

## Enable and disable plugins

You don't need to explicitly enable plugins to auto-detect properly configured sources, but it's useful to know how to
enable or disable them.

One reason you might want to _disable_ plugins is to improve Netdata's performance on low-resource systems, like
ephemeral nodes or edge devices. Disabling orchestrator plugins like `python.d` can save significant resources if you're
not using any of its data collector modules.

You can enable or disable plugins in the `[plugin]` section of `netdata.conf`. This section features a list of all the
plugins with a boolean setting (`yes` or `no`) to enable or disable them. Be sure to uncomment the line by removing the
hash (`#`)!

Enabled:

```conf
[plugins]
  # python.d = yes
```

Disabled:

```conf
[plugins]
  python.d = no
```

When you explicitly disable a plugin this way, it won't auto-collect metrics using its collectors.

## Example: Enable the Nginx collector

To help explain how the auto-detection process works, let's use an Nginx web server as an example. 

Even if you don't have Nginx installed on your system, we recommend you read through the following section so you can
apply the process to other data sources, such as Apache, Redis, Memcached, and more.

The Nginx collector, which helps Netdata collect metrics from a running Nginx web server, is part of the
`python.d.plugin` external plugin _orchestrator_.

In order for Netdata to auto-detect an Nginx web server, you need to enable `ngx_http_stub_status_module` and pass the
`stub_status` directive in the `location` block of your Nginx configuration file.

You can confirm if the `stub_status` Nginx module is already enabled or not by using following command:

```sh
nginx -V 2>&1 | grep -o with-http_stub_status_module
```

If this command returns nothing, you'll need to [enable this module](https://www.nginx.com/blog/monitoring-nginx/).

Next, edit your `/etc/nginx/sites-enabled/default` file to include a `location` block with the following:

```conf
    location /stub_status {
        stub_status;
    }
```

Restart Netdata using `sudo systemctl restart netdata`, or the [appropriate
method](https://github.com/netdata/netdata/blob/master/docs/configure/start-stop-restart.md) for your system, and Netdata will auto-detect metrics from your Nginx web
server!

While not necessary for most auto-detection and collection purposes, you can also configure the Nginx collector itself
by editing its configuration file:

```sh
./edit-config python.d/nginx.conf
```

After configuring any source, or changing the configuration files for their respective modules, always restart Netdata.

## What's next?

Now that you've learned the fundamentals behind configuring data sources for auto-detection, it's time to move back to
the dashboard to learn more about some of its more advanced features.

[Next: Netdata's dashboard in depth &rarr;](step-07.md)


