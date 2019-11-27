---
title: Collect metrics from more services and apps
description: Set up your apps and services to have their metrics auto-collected by Netdata's intelligent plugin system.
---

When Netdata _starts_, it auto-detects dozens of **data sources**, such as database servers, web servers, and more.

To auto-detect and collect metrics from a source you just installed, you need to [restart
Netdata](/docs/getting-started/#start-stop-and-restart-netdata).

However, auto-detection only works if you installed the source using its standard installation
procedure. If Netdata isn't collecting metrics after a restart, your source probably isn't configured
correctly.

Check out the [available data collection modulues](/docs/Add-more-charts-to-netdata/#available-data-collection-modules)
to find the module for the source you want to monitor.

## What you'll learn in this part

We'll begin with an overview on Netdata's plugin architecture, and then dive into the following:

-   [Netdata's plugin architecture](#netdatas-plugin-architecture)
-   [Enable and disable plugins](#enable-and-disable-plugins)
-   [Enable the Nginx module as an example](#example-enabling-the-nginx-module)

## Netdata's plugin architecture

Many Netdata users never have to configure plugins or worry about which plugin orchestrator they want to use.

But, if you want to configure plugins or write a collector module for your custom source, it's important to understand
the underlying plugin architecture.

By default, Netdata collects a lot of metrics every second. It uses **internal** plugins to collect system metrics,
**external** plugins to collect non-system metrics, and **orchestrator** plugins to support data collection modules,
such as the Nginx example mentioned earlier.

Plugin orchestrators are external plugins that do not collect any data by themselves. Instead, they support data
collection modules written in the language of the orchestrator. They communicate with the netdata daemon via pipes
(stdout communication).

Orchestrators also simplify the plugin development process and minimize the number of threads and processes running.

To develop a collector module, you'll choose an orchestrator based on your preferred programming language. Netdata
provides plugin orchestrators for [Bash](/docs/collectors/charts.d.plugin/) (`charts.d`),
[Python](/docs/collectors/python.d.plugin/) (`python.d`), [Go](/docs/collectors/go.d.plugin/) (`go.d`), and
[Node.js](/docs/collectors/node.d.plugin/) (`node.d`).

## Enable and disable plugins

You don't need to explicitly enable plugins to auto-detect properly configured sources, but it's useful to know how to
enable or disable them.

One reason you might want to _disable_ plugins is to improve Netdata's performance on low-resource systems, like
ephemeral nodes or edge devices.

You can enable or disable plugins in the [plugin] section of `netdata.conf`. This section features a list of all the
plugins with a boolean setting (`yes` or `no`) to enable or disable them. Be sure to uncomment the line by removing the
hash (`#`)!

Enabled:

```conf
[plugins]
  node.d = no
```

Disabled:

```conf
[plugins]
  node.d = no
```

When you explicitly disable a plugin this way, it won't auto-collect metrics using its modules.

## Example: Enabling the Nginx module

To help explain how the auto-dectection process works, let's use an Nginx web server as an example. 

Even if you don't have Nginx installed on your system, we recommend you read through the following section so you can
apply the process to other data sources, such as Apache, Redis, Memcached, and more.

The Nginx module, which helps Netdata collect metrics from a running Nginx web server, is part of the `python.d.plugin`
external plugin _orchestrator_. We'll cover what external plugins and orchestrators are in the next section on
[Netdata's plugin architecture](#netdatas-plugin-architecture).

In order for Netdata to auto-detect an Nginx web server, you need to enable `ngx_http_stub_status_module` and pass the
`stub_status` directive in the `location` block of your Nginx configuration file.

You can confirm if the module is already enabled or not by using following command:

```sh
nginx -V 2>&1 | grep -o with-http_stub_status_module
```

And your Nginx configuration file should have a `location` block similar to the following:

```conf
location /stub_status {
    stub_status;
}
```

Restart Netdata using `service netdata restart` or the [correct alternative](/docs/getting-started/#start-stop-and-restart-netdata) for your system, and Netdata will auto-detect metrics from the Nginx web server!

While not necessary for most auto-detection and collection purposes, you can also configure the Nginx collection module
itself by editing its configuration file:

```sh
/etc/netdata/edit-config python.d/nginx.conf
```

After configuring any source, or changing the configration files for their respective modules, always
restart Netdata.

## What's next?

Now that you've learned the fundamentals behind configuring data sources for auto-detection, it's time to move back to
the dashboard to learn more about some of its more advanced features.

<Button><Link to="/tutorials/part-07/">Next: Netdata's dashboard in depth <FaAngleDoubleRight /></Link></Button>
