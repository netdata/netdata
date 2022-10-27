<!--
title: "Agent configuration"
sidebar_label: "Agent configuration"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/tasks/setup/agent-configuration.md"
learn_status: "Published"
sidebar_position: 40
learn_topic_type: "Tasks"
learn_rel_path: "Setup"
learn_docs_purpose: "The most common configuration task for an Agent deployment"
-->

Agent is pre-configured and optimized for general purpose. You can change its settings to tailor made it base to your
deployments and needs. In this document you will guide you through the most common task you can make to the Agent
configuration.

## General tasks

:::info

1. You need to locate your Agent's configuration directory of your Agent. (default: `/etc/netdata`)
2. You need privileged access to perform those tasks.

:::

### See the effective configuration

All the configuration files have the default options (commended in) of Agent's version you installed. Furthermore, you
can see the options you may have changed in the past (commended out). If you have updated the Agent, these default
values may be invalid. To see the effective configuration of a particular Agent, visit the `NODE_IP:19999/netdata.conf`

### Override your Agent's configuration with the effective configuration.

Download the latest version of this file with `wget` or `curl`

```bash
wget -O /etc/netdata/netdata.conf http://localhost:19999/netdata.conf
```

or

```bash
curl -o /etc/netdata/netdata.conf http://localhost:19999/netdata.conf
```

### List all the available configuration files

Under your configuration directory, you will see only some necessary configuration files and the configuration files you
already have touched. To explore all the configuration files execute the following steps.

#### Steps:

1. Navigate under the Agent's configuration directory
    ```bash
    cd /etc/netdata   # Replace this path with your Netdata config directory if different.
    ```


3. Run the `edit-config` helper script
    ```bash
    sudo ./edit-config
    ```

#### Expected result:

You will see all the configuration files available:

```bash

$ sudo ./edit-config  
USAGE:
  ./edit-config FILENAME

  Copy and edit the stock config file named: FILENAME
  if FILENAME is already copied, it will be edited as-is.

  The EDITOR shell variable is used to define the editor to be used.

  Stock config files at: '/usr/lib/netdata/conf.d'
  User  config files at: '/etc/netdata'

  Available files in '/usr/lib/netdata/conf.d' to copy and edit:

./apps_groups.conf         ./go.d/consul.conf              ./go.d/pgbouncer.conf          ./health.d/anomalies.conf           ./health.d/memcached.conf        ./health.d/wmi.conf             ./python.d/ntpd.conf
./charts.d/ap.conf         ./go.d/coredns.conf             ./go.d/phpdae
. . . 
```

#### Relative documents

## Global configuration

### Add labels for your node

You need **host labels**: a powerful way of organizing your Netdata-monitored systems.

Let's take a peek into how to create host labels and apply them across a few of Netdata's features to give you more
organization power over your infrastructure.

#### Create unique host labels

Host labels are defined in `netdata.conf`. To create host labels, open that file
using [`edit-config`](#edit-configuration-files-using-the-edit-config-script).

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config netdata.conf
```

Create a new `[host labels]` section defining a new host label and its value for the system in question. Make sure not
to violate any of the host label naming rules.

The following restrictions apply to host label names:

- Names cannot start with `_`, but it can be present in other parts of the name.
- Names only accept alphabet letters, numbers, dots, and dashes.

The policy for values is more flexible, but you **cannot** use exclamation marks (`!`), whitespaces (` `), single quotes
(`'`), double quotes (`"`), or asterisks (`*`), because they are used to compare label values in health alarms and
templates.

#### Steps:

1. Override your current configuration with the effective

2. `edit-config` the `netdata.conf`

3. Find the section `[host labels]`.

4. Add your labels as the following example
   ```conf
   [host labels]
       # Example of the host label, already there in vanilla installations.
       # name = value
       # Newly added host labels
       location = us-seattle 
       installed = 20200218
   ```

5. Once you've written a few host labels, you need to enable them. Instead of restarting the entire Netdata service, you
   can reload labels using the helpful `netdatacli` tool:

   ```bash
   netdatacli reload-labels
   ```

Your host labels will now be enabled. You can double-check these by using `curl http://HOST-IP:19999/api/v1/info` to
read the status of your agent. For example, from a VPS system running Debian 10:

```json
{
  ...
  "host_labels": {
    "_is_k8s_node": "false",
    "_is_parent": "false",
    "_virt_detection": "systemd-detect-virt",
    "_container_detection": "none",
    "_container": "unknown",
    "_virtualization": "kvm",
    "_architecture": "x86_64",
    "_kernel_version": "4.19.0-6-amd64",
    "_os_version": "10 (buster)",
    "_os_name": "Debian GNU/Linux",
    "type": "webserver",
    "location": "seattle",
    "installed": "20200218"
  },
  ...
}
```

You may have noticed a handful of labels that begin with an underscore (`_`). These are automatic labels.

### Automatic labels

When Netdata starts, it captures relevant information about the system and converts them into automatically-generated
host labels. You can use these to logically organize your systems via health entities, exporting metrics, parent-child
status, and more.

They capture the following:

- Kernel version
- Operating system name and version
- CPU architecture, system cores, CPU frequency, RAM, and disk space
- Whether Netdata is running inside a container, and if so, the OS and hardware details about the container's host
- Whether Netdata is running inside K8s node
- What virtualization layer the system runs on top of, if any
- Whether the system is a streaming parent or child

If you want to organize your systems without manually creating host labels, try the automatic labels in the features
below.

## Database configuration

### Change the Agent's metric retention

The Agent uses a custom-made time-series database (TSDB), named
the [`dbengine`](https://github.com/netdata/netdata/blob/master/database/engine/README.md), to store metrics.

The default settings retain approximately two day's worth of metrics on a system collecting 2,000 metrics every second,
but the Agent is highly configurable if you want your nodes to store days, weeks, or months worth of per-second data.

#### Prerequisites

- A node with the Agent installed, and terminal access to that node

#### Steps

1. Calculate the system resources (RAM, disk space) needed to store metrics

   You can store more or less metrics using the database engine by changing the allocated disk space. Use the calculator
   below to find the appropriate value for the `dbengine` based on how many metrics your node(s) collect, whether you
   are streaming metrics to a parent node, and more.

   You do not need to edit the `dbengine page cache size` setting to store more metrics using the database engine.
   However, if you want to store more metrics _specifically in memory_, you can increase the cache size.

   :::note

   This calculator provides an estimation of disk and RAM usage for **metrics usage**. Real-life usage may vary based on
   the accuracy of the values you enter below, changes in the compression ratio, and the types of metrics stored.

   :::

   Download
   the [calculator](https://docs.google.com/spreadsheets/d/e/2PACX-1vTYMhUU90aOnIQ7qF6iIk6tXps57wmY9lxS6qDXznNJrzCKMDzxU3zkgh8Uv0xj_XqwFl3U6aHDZ6ag/pub?output=xlsx)
   to optimize the data retention to your preferences. Utilize the "Front" spreadsheet. Experiment with the variables
   which are padded with yellow to come up with the best settings for your use case.

2. Edit `netdata.conf` with recommended database engine settings

   Now that you have a recommended setting for your Agent's `dbengine`, edit
   the [Netdata configuration file](https://github.com/netdata/netdata/blob/master/docs/tasks/general-configuration/configure-the-agent.md)
   and look for the `[db]` subsection. Change it to the recommended values you calculated from the calculator. For
   example:

   ```conf
   [db]
      mode = dbengine
      storage tiers = 3
      update every = 1
      dbengine multihost disk space MB = 1024
      dbengine page cache size MB = 32
      dbengine tier 1 update every iterations = 60
      dbengine tier 1 multihost disk space MB = 384
      dbengine tier 1 page cache size MB = 32
      dbengine tier 2 update every iterations = 60
      dbengine tier 2 multihost disk space MB = 16
      dbengine tier 2 page cache size MB = 32
   ```

3. Save the file
   and [restart the Agent](https://github.com/netdata/netdata/blob/master/docs/tasks/general-configuration/start-stop-and-restart-agent.md)
   , to change the database engine's size.

#### Related topics

1. [dbengine Reference](https://github.com/netdata/netdata/blob/master/database/engine/README.md)

## Agent's directories and files

## Daemon's environment variables

## Agent's machine learning component

## Global Health settings

## Change Health settings for a specific alert

### Silence an alert

#### Steps:

All of Netdata's health configuration files are in Netdata's config directory, inside the `health.d/` directory. You can
edit them by following
the [Configure the Agent](https://github.com/netdata/netdata/blob/master/docs/tasks/general-configuration/configure-the-agent.md)
Task.

For example, lets take a look at the `health/cpu.conf` file.

```bash
 template: 10min_cpu_usage
       on: system.cpu
    class: Utilization
     type: System
component: CPU
       os: linux
    hosts: *
   lookup: average -10m unaligned of user,system,softirq,irq,guest
    units: %
    every: 1m
     warn: $this > (($status >= $WARNING)  ? (75) : (85))
     crit: $this > (($status == $CRITICAL) ? (85) : (95))
    delay: down 15m multiplier 1.5 max 1h
     info: average CPU utilization over the last 10 minutes (excluding iowait, nice and steal)
       to: sysadmin
```

1. Navigate under the Agent's configuration directory
    ```bash
    cd /etc/netdata   # Replace this path with your Netdata config directory if different.
    ```


2. `edit-config` it's configuration file
    ```bash
    sudo ./edit-config health.d/cpu.conf
    ```

3. Change it's `to: ...` field.
   ```sh
     to: silent
   ```

4. Reload health settings
    ```bash
    sudo netdatacli reload-health
    ```

## Agent's web server

## Setup registry

## Global settings for plugins

## Global settings (per plugin)

## Streaming engine

### Enable streaming between two Agents (Parent-Child set up)

The simplest streaming configuration is **replication**, in which a child node streams its metrics in real time to a
parent node, and both nodes retain metrics in their own databases. You can read more in
our [Streaming Concept](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/metrics-streaming-replication.md)
documentation.

#### Prerequisites

To configure replication, you need two nodes, each of them running a Netdata instance.

#### Steps

First you'll need to enable streaming/ prepare the environment of the parent Agent to accept streams of metrics.

1. Configuration on the Parent node

    1. First, log onto the node that will act as the parent.

    2. Run `uuidgen` to create a new API key, which is a randomly-generated machine GUID the Netdata Agent uses to
       identify itself while initiating a streaming connection. Copy that into a separate text file for later use.

       :::info Find out how to [install `uuidgen`](https://command-not-found.com/uuidgen) on your node if you don't
       already have it.
       :::

    3. Next, open `stream.conf` using `edit-config` from within the Netdata config directory.  
       To read more about how to configure the Netdata Agent, check
       our [Agent Configuration](https://github.com/netdata/netdata/blob/master/docs/tasks/general-configuration/configure-the-agent.md)
       Task.

    4. Scroll down to the section beginning with `[API_KEY]`. Paste the API key you generated earlier between the
       brackets, so that it looks like the following:

       ```conf
       [11111111-2222-3333-4444-555555555555]
       ```
    5. Set `enabled` to `yes`, and `default memory mode` to `dbengine`. Leave all the other settings as their defaults.
       A simplified version of the configuration, minus the commented lines, looks like the following:

       ```conf
       [11111111-2222-3333-4444-555555555555]
           enabled = yes
           default memory mode = dbengine
       ```

    6. Save the file and close it, then restart Netdata with `sudo systemctl restart netdata`, or the appropriate method
       for your system in
       the [Starting, Stopping and Restarting the Agent](https://github.com/netdata/netdata/blob/master/docs/tasks/general-configuration/start-stop-and-restart-agent.md)
       Task.

2. Configuration on the Child node

In this section you will enable streaming in the Child node and set the target where the stream of metrics will be sent.

1. Connect via terminal to your child node with SSH.
2. Open `stream.conf` again.
3. Scroll down to the `[stream]` section and set `enabled` to `yes`.
4. Paste the IP address of your parent node at the end of the `destination` line
5. Paste the API key generated on the parent node onto the `api key` line.

   Leave all the other settings as their defaults. A simplified version of the configuration, minus the commented lines,
   looks like the following:

   ```conf
   [stream]
       enabled = yes 
       destination = 203.0.113.0
       api key = 11111111-2222-3333-4444-555555555555
   ```

6. Save the file and close it, then restart Netdata with `sudo systemctl restart netdata`, or the appropriate method for
   your system in
   the [Starting, Stopping and Restarting the Agent](https://github.com/netdata/netdata/blob/master/docs/tasks/general-configuration/start-stop-and-restart-agent.md)
   Task.

#### Further actions

##### Enable TLS/SSL on streaming (optional)

While encrypting the connection between your parent and child nodes is recommended for security, it's not required to
get started. If you're not interested in encryption, skip ahead to the [Expected result](#expected-result) section.

In this example, we'll use self-signed certificates.

1. On the **parent** node, use OpenSSL to create the key and certificate, then use `chown` to make the new files
   readable by the `netdata` user.

   ```bash
   sudo openssl req -newkey rsa:2048 -nodes -sha512 -x509 -days 365 -keyout /etc/netdata/ssl/key.pem -out /etc/netdata/ssl/cert.pem
   sudo chown netdata:netdata /etc/netdata/ssl/cert.pem /etc/netdata/ssl/key.pem
   ```

2. Next, enforce TLS/SSL on the web server. Open `netdata.conf`, scroll down to the `[web]` section, and look for
   the `bind to` setting. Add `^SSL=force` to turn on TLS/SSL.

   See the [Netdata configuration Reference](https://github.com/netdata/netdata/blob/master/daemon/README.md) for other
   TLS/SSL options.

    ```conf
    [web]
       bind to = *=dashboard|registry|badges|management|streaming|netdata.conf^SSL=force
    ```

3. Next, connect to the **child** node and open `stream.conf`.   
   Add `:SSL` to the end of the existing `destination` setting to connect to the parent using TLS/SSL.   
   Uncomment the `ssl skip certificate verification` line to allow the use of self-signed certificates.

   ```conf
   [stream]
       enabled = yes
       destination = 203.0.113.0:SSL
       ssl skip certificate verification = yes
       api key = 11111111-2222-3333-4444-555555555555
   ```

4. Restart both the parent and child nodes with `sudo systemctl restart netdata`, or
   the [appropriate method](/docs/configure/start-stop-restart.md) for your system, to stream encrypted metrics using
   TLS/SSL.

#### Expected result

At this point, the child node is streaming its metrics in real time to its parent. Open the local Agent dashboard for
the parent by navigating to `http://PARENT-NODE:19999` in your browser, replacing `PARENT-NODE` with its IP address or
hostname.

This dashboard shows parent metrics. To see child metrics, open the left-hand sidebar with the hamburger icon
![Hamburger icon](https://raw.githubusercontent.com/netdata/netdata-ui/master/src/components/icon/assets/hamburger.svg)
in the top panel. Both nodes appear under the **Replicated Nodes** menu. Click on either of the links to switch between
separate parent and child dashboards.

The child dashboard is also available directly at `http://PARENT-NODE:19999/host/CHILD-HOSTNAME`.

#### Related topics

1. [Streaming Concept](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/metrics-streaming-replication.md)
2. [Streaming Configuration Reference](https://github.com/netdata/netdata/blob/master/streaming/README.md)

## Exporting engine

### Export your metric via an exporting connector

In this task, you will learn how to enable the exporting engine, and the exporting connector, followed by two examples
using the OpenTSDB and Graphite connectors.

:::note When you enable the exporting engine and a connector, the Netdata Agent exports metrics _beginning from the time
you restart its process_, not the entire database of long-term metrics.
:::

Once you understand the process of enabling a connector, you can translate that knowledge to any other connector.

#### Prerequisites

You need to find the right connector for
your [external time-series database](MISSING LINK FOR METRIC EXPORTING REFERENCES), and then you can proceed on with the
task.

#### Enable the exporting engine

1. Edit the `exporting.conf` configuration file,
   by [editing the Netdata configuration](https://github.com/netdata/netdata/blob/master/docs/tasks/general-configuration/configure-the-agent.md)
   .
2. Enable the exporting engine itself by setting `enabled` to `yes`:

    ```conf
    [exporting:global]
        enabled = yes
    ```

3. Save the file but keep it open, as you will edit it again to enable specific connectors.

#### Example: Enable the OpenTSDB connector

Use the following configuration as a starting point. Copy and paste it into `exporting.conf`.

```conf
[opentsdb:http:my_opentsdb_http_instance]
    enabled = yes
    destination = localhost:4242
```

1. Replace `my_opentsdb_http_instance` with an instance name of your choice, and change the `destination` setting to the
   IP address or hostname of your OpenTSDB database.

2. [Restart your Agent](https://github.com/netdata/netdata/blob/master/docs/tasks/general-configuration/start-stop-and-restart-agent.md#restarting-the-agent)
   to begin exporting to your OpenTSDB database. The Netdata Agent exports metrics _beginning from the time the process
   starts_, and because it exports as metrics are collected, you should start seeing data in your external database
   after only a few seconds.

<!--Any further configuration is optional, based on your needs and the configuration of your OpenTSDB database. See the
[OpenTSDB connector doc](/exporting/opentsdb/README.md)-->

#### Example: Enable the Graphite connector

Use the following configuration as a starting point. Copy and paste it into `exporting.conf`.

```conf
[graphite:my_graphite_instance]
    enabled = yes
    destination = 203.0.113.0:2003
```

1. Replace `my_graphite_instance` with an instance name of your choice, and change the `destination` setting to the IP
   address or hostname of your Graphite-supported database.

2. [Restart your Agent](https://github.com/netdata/netdata/blob/master/docs/tasks/general-configuration/start-stop-and-restart-agent.md#restarting-the-agent)
   to begin exporting to your Graphite-supported database. Because the Agent exports metrics as they're collected, you
   should start seeing data in your external database after only a few seconds.

<!--Any further configuration is optional, based on your needs and the configuration of your Graphite-supported database.
See [exporting engine reference](/exporting/README.md#configuration) for details.-->

### Host labels in streaming

You may have noticed the `_is_parent` and `_is_child` automatic labels from above. Host labels are also now streamed
from a child to its parent node, which concentrates an entire infrastructure's OS, hardware, container, and
virtualization information in one place: the parent.

Now, if you'd like to remind yourself of how much RAM a certain child node has, you can access
`http://localhost:19999/host/CHILD_HOSTNAME/api/v1/info` and reference the automatically-generated host labels from the
child system. It's a vastly simplified way of accessing critical information about your infrastructure.

:::caution Because automatic labels for child nodes are accessible via API calls, and contain sensitive information like
kernel and operating system versions, you should secure streaming connections with SSL. See
the [Configure streaming](https://github.com/netdata/netdata/blob/master/docs/tasks/manage-retained-metrics/configure-streaming.md#enable-tlsssl-on-streaming-optional)
Task for details. You may also want to use
[access lists](MISSING LINK) or [expose the API only to LAN/localhost connections](MISSING LINK).
:::

You can also use `_is_parent`, `_is_child`, and any other host labels in both health entities and metrics exporting.
Speaking of which...

### Host labels in health entities

You can use host labels to logically organize your systems by their type, purpose, or location, and then apply specific
alarms to them.

For example, let's use the configuration example from earlier:

```conf
[host labels]
    type = webserver
    location = us-seattle
    installed = 20200218
```

You can now create a new health entity (checking if disk space will run out soon) that applies only to any host
labeled `webserver`:

```yaml
    template: disk_fill_rate
      on: disk.space
      lookup: max -1s at -30m unaligned of avail
        calc: ($this - $avail) / (30 * 60)
        every: 15s
    host labels: type = webserver
```

Or, by using one of the automatic labels, for only webserver systems running a specific OS:

```yaml
 host labels: _os_name = Debian*
```

In a streaming configuration where a parent node is triggering alarms for its child nodes, you could create health
entities that apply only to child nodes:

```yaml
 host labels: _is_child = true
```

Or when ephemeral Docker nodes are involved:

```yaml
 host labels: _container = docker
```

Of course, there are many more possibilities for intuitively organizing your systems with host labels. See
the [health Reference](MISSING LINK/ no file in the repo yet) documentation for more details, and then get creative!

### Host labels in metrics exporting

If you have enabled any metrics exporting via our
experimental [exporters](https://github.com/netdata/netdata/blob/master/exporting/README.md), any new host labels you
created manually are sent to the destination database alongside metrics. You can change this behavior by
editing `exporting.conf`, and you can even send automatically-generated labels on with exported metrics.

```conf
[exporting:global]
enabled = yes
send configured labels = yes
send automatic labels = no
```

You can also change this behavior per exporting connection:

```conf
[opentsdb:my_instance3]
enabled = yes
destination = localhost:4242
data source = sum
update every = 10
send charts matching = system.cpu
send configured labels = no
send automatic labels = yes
```

By applying labels to exported metrics, you can more easily parse historical metrics with the labels applied. To learn
more about exporting, read the [Exporting Reference](https://github.com/netdata/netdata/blob/master/exporting/README.md)
documentation.