<!--
title: "Configure the Agent"
sidebar_label: "Configure the Agent"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/tasks/general-configuration/configure-the-agent.md"
learn_status: "Published"
sidebar_position: 1
learn_topic_type: "Tasks"
learn_rel_path: "general-configuration"
learn_docs_purpose: "demonstrate an edit-config, reference to the ref-doc of netdata.conf, plus add an admonition in case user want to change metric retention to follow the corresponding doc"
-->

In this task you will learn how to edit the configuration files of the Agent, and also how to use **Host labels**.  
All of our configuration Tasks point to this one for editing the configurtion files correctly.

## Prerequisites

- A node with the Agent installed
- Terminal access to that node

## Edit configuration files using the `edit-config` script

:::tip
The most reliable method of finding your Netdata config directory is loading your `netdata.conf` on your browser. Open a
tab and navigate to `http://HOST:19999/netdata.conf`, replacing `HOST` with your node's IP address, or if you want to
access the node you are locally using, you can also use `localhost`.

Look for the line that begins with `# config directory =` . The text after that will be the path to your Netdata config
directory.
:::

Inside your Netdata config directory there is a helper script called `edit-config`. We made that script to help you in
properly editing configuration files. The script takes as an argument a configuration file, and it will check for that
in the stock configuration directory and in the user directory. If you haven't edited the configuration file before,
upon saving, the script will copy the file and place it in the user configuration directory, which will in turn override
the stock configuration.

:::tip
you can run:

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory if different.
sudo ./edit-config
```

to get a list of all the available stock configuration files that you can edit.
:::

This is the preferred method for editing Netdata's configuration, and making sure the changes are valid.

So, in short, to edit configuration files, for example `netdata.conf`, you can run:

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory if different.
sudo ./edit-config netdata.conf
```

You should now see `netdata.conf` in your editor!

:::caution
After making changes to the Netdata configuration files you need
to [restart the Agent](https://github.com/netdata/netdata/blob/master/docs/tasks/general-configuration/start-stop-and-restart-agent.md#restarting-the-agent)
for these changes to take effect! This action though isn't necessary for health configuration files, in which you just
need to
[reload the health configuration](https://github.com/netdata/netdata/blob/master/docs/tasks/general-configuration/start-stop-and-restart-agent.md#reload-health-configuration)
between changes.
:::

## Use host labels to organize systems, metrics, and alarms

When you use Netdata to monitor and troubleshoot an entire infrastructure, whether that's dozens or hundreds of systems,
you need sophisticated ways of keeping everything organized. You need alarms that adapt to the system's purpose, or
whether the parent or child are in a streaming setup. You need properly-labeled metrics archiving so you can sort,
correlate, and mash-up your data to your heart's content. You need to keep tabs on ephemeral Docker containers in a
Kubernetes cluster.

You need **host labels**: a powerful way of organizing your Netdata-monitored systems.

Let's take a peek into how to create host labels and apply them across a few of Netdata's features to give you more
organization power over your infrastructure.

### Create unique host labels

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

```conf
[host labels]
    type = webserver
    location = us-seattle
    installed = 20200218
```

Once you've written a few host labels, you need to enable them. Instead of restarting the entire Netdata service, you
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
host labels. You can use these to logically organize your systems via health entities, exporting metrics,
parent-child status, and more.

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

### Host labels in streaming

You may have noticed the `_is_parent` and `_is_child` automatic labels from above. Host labels are also now
streamed from a child to its parent node, which concentrates an entire infrastructure's OS, hardware, container,
and virtualization information in one place: the parent.

Now, if you'd like to remind yourself of how much RAM a certain child node has, you can access
`http://localhost:19999/host/CHILD_HOSTNAME/api/v1/info` and reference the automatically-generated host labels from the
child system. It's a vastly simplified way of accessing critical information about your infrastructure.

:::caution
Because automatic labels for child nodes are accessible via API calls, and contain sensitive information like
kernel and operating system versions, you should secure streaming connections with SSL. See
the [Configure streaming](https://github.com/netdata/netdata/blob/master/docs/tasks/manage-retained-metrics/configure-streaming.md#enable-tlsssl-on-streaming-optional)
Task for details. You may also want to use
[access lists](MISSING LINK) or [expose the API only to LAN/localhost
connections](MISSING LINK).
:::

You can also use `_is_parent`, `_is_child`, and any other host labels in both health entities and metrics
exporting. Speaking of which...

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

Of course, there are many more possibilities for intuitively organizing your systems with host labels. See the [health
Reference](MISSING LINK/ no file in the repo yet) documentation for more details, and then get creative!

### Host labels in metrics exporting

If you have enabled any metrics exporting via our
experimental [exporters](https://github.com/netdata/netdata/blob/master/exporting/README.md), any new host
labels you created manually are sent to the destination database alongside metrics. You can change this behavior by
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