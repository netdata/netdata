<!--
title: "Anomaly detection with ML and the Netdata Agent"
description: "Train a "
image: /img/seo/guides/monitor/anomalies-ml.png
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/guides/monitor/anomalies-ml.md
-->

# Anomaly detection with ML and the Netdata Agent

**T/K**

**anomaly detection** with machine learning (ML) and the Netdata Agent.

If you choose, you'll use an Nginx web server to practice configuring the anomalies collector and making sense of the
its real-time visualizations.

## Prerequisites

- A node running the Netdata Agent. If you don't yet have that, [get Netdata](/docs/get/README.md).
- A Netdata Cloud account. [Sign up](https://app.netdata.cloud) if you don't have one already.
- Familiarity with configuring the Netdata Agent with [`edit-config`](/docs/configure/nodes.md).
- _Optional_: An Nginx web server running on the same node to follow the example configuration steps.

## Install required Python packages

The anomalies collector uses a few Python packages, available with `pip`, to run ML training. It requires `numba`,
`scikit-learn`, `pyod`, in addition to `netdata-pandas`, which is a package built by the Netdata team to pull data from
a Netdata Agent's API into a Pandas DataFrame. Read more about `netdata-pandas` on its [package
repo](https://github.com/netdata/netdata-pandas) or in Netdata's [community
repo](https://github.com/netdata/community/tree/main/netdata-agent-api/netdata-pandas).

```bash
# Become the netdata user
sudo su -s /bin/bash netdata

# Install required packages for the netdata user
pip3 install --user netdata-pandas==0.0.32 numba==0.50.1 scikit-learn==0.23.2 pyod==0.8.3
```

Use `exit` to become your normal user again.

## Enable the anomalies collector

Navigate to your [Netdata config directory](/docs/configure/nodes.md#the-netdata-config-directory) and use `edit-config`
to open the `python.d.conf` file.

```bash
sudo ./edit-config python.d.conf
```

In `python.d.conf` file, search for the `anomalies` line. If the line exists, set the value to `yes`. If the line
doesn't exist, add it into the file anywhere you like. Either way, the final result should look like:

```conf
anomalies: yes
```

Restart the Agent with `service netdata restart` to start up the anomalies collector. By default, the model training
process runs every 30 minutes, and uses the previous 4 hours of metrics to establish a baseline for health and
performance across the default charts to include. 

## Configure the anomalies collector

Open `python.d/anomalies.conf` with `edit-conf`.

```bash
sudo ./edit-config python.d/anomalies.conf
```

The file contains many user-configurable settings with sane defaults. Here are some important settings that don't
involve tweaking the behavior of the ML training itself.

- `charts_regex`: Which charts to train against and include in anomaly detection.
- `charts_to_exclude`: Specific charts, selected by the regex in `charts_regex`, to exclude.
- `train_every_n`: How often to train the ML models.
- `train_n_secs`: The amount of historical metrics to train ML models on. The default is 4 hours, but if your node
  doesn't have historical metrics going back that far, consider [changing the metrics retention
  policy](/docs/store/change-metrics-storage.md).
- `custom_models`: A way to define custom models that you want anomaly probabilities for, including multi-node or
  streaming setups.

### Run anomaly detection on Nginx and log file metrics

As mentioned above, this guide uses an Nginx web server to demonstrate how the anomalies collector works. You must
configure the collector to monitor charts from the
[Nginx](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/nginx) and [web
log](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/weblog) collectors.

`charts_regex` allows for some basic regex, such as wildcards (`*`) to match all contexts with a certain pattern. For
example, `system\..*` matches with any chart wit ha context that begins with `system.`, and ends in any number of other
characters (`.*`). Note the escape character (`\`) around the first period to capture a period character exactly, and
not any character.

Change `charts_regex` in `anomalies.conf` to the following:

```conf
    charts_regex: 'system\..*|nginx_local\..*|web_log_nginx\..*|apps.cpu|apps.mem'
```

This value tells the anomaly collector to train against every `system.` chart, every `nginx_local` chart, every
`web_log_nginx` chart, and specifically the `apps.cpu` and `apps.mem` charts.

![The anomalies collector chart with many
dimensions](https://user-images.githubusercontent.com/1153921/102813877-db5e4880-4386-11eb-8040-d7a1d7a476bb.png)

### Remove some metrics from anomaly detection

As you can see in the above screenshot, this node is now looking for anomalies in a lot of places. The result is a
single `anomalies_local.probability` chart with more than twenty dimensions, some of which are hidden at the bottom of a
scroll-able area. In addition, training and analyzing the anomaly collector on this many charts might require more CPU
utilization that you're willing to give.

First, explicitly declare which `system.` charts to monitor rather than of all of them using regex (`system\..*`).

```conf
    charts_regex: 'system\.cpu|system\.load|system\.io|system\.net|system\.ram|nginx_local\..*|web_log_nginx\..*|apps.cpu|apps.mem'
```

Next, remove some charts with the `charts_to_exclude` setting. For this example, using an Nginx web server, focus on the
volume of requests/responses, not, for example, which type of 4xx response a user might receive.

```conf
    charts_to_exclude: 'web_log_nginx.excluded_requests,web_log_nginx.responses_by_status_code_class,web_log_nginx.status_code_class_2xx_responses,web_log_nginx.status_code_class_4xx_responses,web_log_nginx.current_poll_uniq_clients,web_log_nginx.requests_by_http_method,web_log_nginx.requests_by_http_version,web_log_nginx.requests_by_ip_proto'
```

![The anomalies collector with less
dimensions](https://user-images.githubusercontent.com/1153921/102820642-d69f9180-4392-11eb-91c5-d3d166d40105.png)

Apply the ideas behind the collector's regex and excluding settings to any other
[system](/docs/collect/system-metrics.md), [container](/docs/collect/container-metrics.md), or
[application](/docs/collect/application-metrics.md) metrics you want to detect anomalies for.

### Add a custom anomaly detection model for Nginx

While you now have basic anomaly detection for your Nginx web server, the collector is only training and visualizing
anomalies based on individual charts, not groups of connected charts.

You may want to know if a group of charts is acting anomalously, not just a single chart related to a particular
application or service. For example, a sudden increase in volume of requests on an Nginx web server may be anomalous, in
that the website being served is seeing more traffic than usual, but the web server may still be operating normally. On
the other hand, high requests _plus_ high CPU utilization or `4xx` responses most likely indicates an incident worth
further investigation.

```conf
custom_models:
    - name: 'nginx'
      dimensions: 'apps.cpu|httpd,apps.mem|httpd,nginx.connections,web_log.requests,web_log.type_requests'
```

Let's break down this line.

## See anomaly detection in action

### Alarms

### Build an anomaly detection dashboard

### Multi-node anomaly detection

## What's next?

TK

### Related reference documentation

- [Netdata Agent · Anomalies collector](/collectors/python.d/anomalies/README.md)

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fguides%2Fmonitor%2Fanomaly-detectionl&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
