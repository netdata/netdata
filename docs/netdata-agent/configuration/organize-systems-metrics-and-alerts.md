# Organize systems, metrics, and alerts

When you use Netdata to monitor and troubleshoot an entire infrastructure, you need sophisticated ways of keeping everything organized.
Netdata allows to organize your observability infrastructure with spaces, war rooms, virtual nodes, host labels, and metric labels.

## Spaces and war rooms

[Spaces](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/organize-your-infrastrucutre-invite-your-team.md#netdata-cloud-spaces) are used for organization-level or infrastructure-level 
grouping of nodes and people. A node can only appear in a single space, while people can have access to multiple spaces.

The [war rooms](https://github.com/netdata/netdata/edit/master/docs/cloud/war-rooms.md) in a space bring together nodes and people in 
collaboration areas. War rooms can also be used for fine-tuned 
[role based access control](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/role-based-access.md). 

## Virtual nodes

Netdata’s virtual nodes functionality allows you to define nodes in configuration files and have them be treated as regular nodes 
in all of the UI, dashboards, tabs, filters etc. For example, you can create a virtual node each for all your Windows machines 
and monitor them as discrete entities. Virtual nodes can help you simplify your infrastructure monitoring and focus on the 
individual node that matters.

To define your windows server as a virtual node you need to:

  * Define virtual nodes in `/etc/netdata/vnodes/vnodes.conf`

    ```yaml
    - hostname: win_server1
      guid: <value>
    ```
    Just remember to use a valid guid (On Linux you can use `uuidgen` command to generate one, on Windows just use the `[guid]::NewGuid()` command in PowerShell)
    
  * Add the vnode config to the data collection job. e.g. in `go.d/windows.conf`:
    ```yaml
      jobs:
        - name: win_server1
          vnode: win_server1
          url: http://203.0.113.10:9182/metrics
    ```
    
## Host labels

Host labels can be extremely useful when:

- You need alerts that adapt to the system's purpose
- You need properly-labeled metrics archiving so you can sort, correlate, and mash-up your data to your heart's content.
- You need to keep tabs on ephemeral Docker containers in a Kubernetes cluster.

Let's take a peek into how to create host labels and apply them across a few of Netdata's features to give you more
organization power over your infrastructure.

### Default labels

When Netdata starts, it captures relevant information about the system and converts them into automatically generated
host labels. You can use these to logically organize your systems via health entities, exporting metrics,
parent-child status, and more.

They capture the following:

-   Kernel version
-   Operating system name and version
-   CPU architecture, system cores, CPU frequency, RAM, and disk space
-   Whether Netdata is running inside of a container, and if so, the OS and hardware details about the container's host
-   Whether Netdata is running inside K8s node 
-   What virtualization layer the system runs on top of, if any
-   Whether the system is a streaming parent or child

If you want to organize your systems without manually creating host labels, try the automatic labels in some of the
features below. You can see them under `http://HOST-IP:19999/api/v1/info`, beginning with an underscore `_`.
```json
{
  ...
  "host_labels": {
    "_is_k8s_node": "false",
    "_is_parent": "false",
    ...
```

### Custom labels

Host labels are defined in `netdata.conf`. To create host labels, open that file using `edit-config`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config netdata.conf
```

Create a new `[host labels]` section defining a new host label and its value for the system in question. Make sure not
to violate any of the [host label naming rules](https://github.com/netdata/netdata/blob/master/docs/configure/common-changes.md#organize-nodes-with-host-labels).

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


### Host labels in streaming

You may have noticed the `_is_parent` and `_is_child` automatic labels from above. Host labels are also now
streamed from a child to its parent node, which concentrates an entire infrastructure's OS, hardware, container,
and virtualization information in one place: the parent.

Now, if you'd like to remind yourself of how much RAM a certain child node has, you can access
`http://localhost:19999/host/CHILD_HOSTNAME/api/v1/info` and reference the automatically-generated host labels from the
child system. It's a vastly simplified way of accessing critical information about your infrastructure.

> ⚠️ Because automatic labels for child nodes are accessible via API calls, and contain sensitive information like
> kernel and operating system versions, you should secure streaming connections with SSL. See the [streaming
> documentation](https://github.com/netdata/netdata/blob/master/src/streaming/README.md#securing-streaming-communications) for details. You may also want to use
> [access lists](https://github.com/netdata/netdata/blob/master/src/web/server/README.md#access-lists) or [expose the API only to LAN/localhost
> connections](https://github.com/netdata/netdata/blob/master/docs/category-overview-pages/secure-nodes.md#expose-netdata-only-in-a-private-lan).

You can also use `_is_parent`, `_is_child`, and any other host labels in both health entities and metrics
exporting. Speaking of which...

### Host labels in alerts

You can use host labels to logically organize your systems by their type, purpose, or location, and then apply specific
alerts to them.

For example, let's use configuration example from earlier:

```conf
[host labels]
    type = webserver
    location = us-seattle
    installed = 20200218
```

You could now create a new health entity (checking if disk space will run out soon) that applies only to any host
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

In a streaming configuration where a parent node is triggering alerts for its child nodes, you could create health
entities that apply only to child nodes:

```yaml
 host labels: _is_child = true
```

Or when ephemeral Docker nodes are involved:

```yaml
 host labels: _container = docker
```

Of course, there are many more possibilities for intuitively organizing your systems with host labels. See the [health
documentation](https://github.com/netdata/netdata/blob/master/src/health/REFERENCE.md#alert-line-host-labels) for more details, and then get creative!

### Host labels in metrics exporting

If you have enabled any metrics exporting via our experimental [exporters](https://github.com/netdata/netdata/blob/master/src/exporting/README.md), any new host
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
more about exporting, read the [documentation](https://github.com/netdata/netdata/blob/master/src/exporting/README.md).

## Metric labels

The Netdata aggregate charts allow you to filter and group metrics based on label name-value pairs.

All go.d plugin collectors support the specification of labels at the "collection job" level. Some collectors come with out of the box 
labels (e.g. generic Prometheus collector, Kubernetes, Docker and more). But you can also add your own custom labels, by configuring 
the data collection jobs. 

For example, suppose we have a single Netdata agent, collecting data from two remote Apache web servers, located in different data centers. 
The web servers are load balanced and provide access to the service "Payments".

You can define the following in `go.d.conf`, to be able to group the web requests by service or location:

```
jobs:
  - name: mywebserver1
    url: http://host1/server-status?auto
    labels:
      service: "Payments"
      location: "Atlanta"
  - name: mywebserver2
    url: http://host2/server-status?auto
    labels:
      service: "Payments"
      location: "New York"
```

Of course you may define as many custom label/value pairs as you like, in as many data collection jobs you need. 
