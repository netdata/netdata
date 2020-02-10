# Collectors quickstart

In this quickstart guide, you'll learn how to enable collectors so you can get metrics from your favorite applications
and services.

This guide will not cover advanced collector features, such as enabling/disabling entire plugins, 

## What's in this quickstart guide

-   [Find the collector for your application or service](#find-the-collector-for-your-application-or-service)
-   [Configure your application or service for monitoring](#configure-your-application-or-service-for-monitoring)
-   [Edit the configuration file for your collector](#edit-the-configuration-file-for-your-collector)
-   [Enable the collector](#enable-the-collector)
-   [See new charts on your dashboard]()
-   [Troubleshooting a collector]()

## Find the collector for your application or service

Netdata has _pre-installed_ collectors for hundreds of popular applications and services. You don't need to install
anything to collect metrics from many popular services, like Nginx web servers, MySQL/MariaDB databases, and much more.

To find whether Netdata has a pre-installed collector for your favorite app/service, check out our [collector support
list](COLLECTORS.md). The only exception is the [third-party collectors](COLLECTORS.md#third-party-plugins), which
you do need to install yourself. However, this quickstart guide will focus on pre-installed collectors.

From here on out, this quickstart guide will use the [Nginx collector](go.d.plugin/modules/nginx/README.md) as an
example to showcase the process of configuring and enabling one of Netdata's pre-installed collectors.

## Configure your application or service for monitoring

Every collector's documentation comes with instructions on how to configure your app/service to make it available to
Netdata's collector.

For example, the [Nginx collector documentation](go.d.plugin/modules/nginx/README.md) informs you that it requires that
Nginx have the `stub_status` module configured correctly, in addition to an active `stub_status/` page. You can confirm
whether you have the module enabled with the following command:

```bash
nginx -V 2>&1 | grep -o with-http_stub_status_module
```

If this command returns nothing, you'll need to [enable the `stub_status`
module](https://www.nginx.com/blog/monitoring-nginx/).

Next, edit your `/etc/nginx/sites-enabled/default` file to include a `location` block with the following, which enables
the `stub_status` page:

```conf
server {
    ...

    location /nginx_status {
        stub_status;
    }
}
```

At this point, your Nginx installation is fully configured and ready for Netdata to monitor it. Next, you'll configure
your collector.

## Edit the configuration file for your collector

This step may not be required based on how you configured your app/service, as each collector comes with a few
pre-configured jobs that look for the app/service in common and expected locations. For example, the Nginx collector
looks for a `stub_status` page at `http://localhost/stub_status` and `http://127.0.0.1/stub_status`, which allows it to
auto-detect almost all local Nginx web servers.

Despite Netdata's auto-detection capabilities, it's important to know how to edit collector configuration files.

You should always edit configuration files with the `edit-config` script that comes with every installation of Netdata.
To edit a collector configuration file, navigate to your [Netdata configuration
directory](../docs/step-by-step/step-04.md#find-your-netdataconf-file). Launch `edit-config` with the path to the
collector's configuration file.

For example, the Nginx collector's file is at `go.d/nginx.conf`.

```bash
cd /etc/netdata
sudo ./edit-config go.d/nginx.conf
```

This file contains all of the possible job parameters to help you monitor Nginx in all sorts of complex deployments. At
the bottom of the file is a `[JOB]` section, which contains the two default jobs. Configure these as needed, using those
parameters as a reference, to configure the collector.

### How do I edit the right configuration file?

The easiest way to know which path to use with `edit-config` is by referencing the [collector support
list](COLLECTORS.md). Each row contains a _configuration file_ column containing the configuration file path.

## Enable the collector

Most collectors are enabled and will auto-detect their app/service without manual configuration. However, you need to
restart Netdata to trigger the auto-detection process.

To restart Netdata on most systems, use `service netdata restart`. For other systems, see the [other restart
methods](../docs/getting-started.md#start-stop-and-restart-netdata).

Open Netdata's dashboard in your browser, or refresh the page if you already have it open. You should now see a new
entry in the menu and new interactive charts!

## What's next?

Collector not working? Learn about collector troubleshooting in our [collector
reference](REFERENCE.md#troubleshoot-a-collector).

View our [collectors tutorials](README.md#tutorials) to get specific instructions on enabling new and popular
collectors.

Finally, learn more advanced collector features, such as disabling plugins or developing a custom collector, in our
[internal plugin API](REFERENCE.md#internal-plugins-api) or our [external plugin docs](plugins.d/README.md).
