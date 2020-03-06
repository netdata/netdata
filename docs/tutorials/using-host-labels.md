# Use host labels to organize systems, metrics, and alarms

When you use Netdata to monitor and troubleshoot an entire infrastructure, whether that's dozens or hundreds of systems,
you need sophisticated ways of keeping everything organized. You need alarms that adapt to the system's purpose, or
whether the `master` or `slave` in a streaming setup. You need properly-labeled metrics archiving so you can sort,
correlate, and mash-up your data to your heart's content. You need to keep tabs on ephemeral Docker containers in a
Kubernetes cluster.

You need **host labels**: a powerful new way of organizing your Netdata-monitored systems. We introduced host labels in
[v1.20 of Netdata](https://blog.netdata.cloud/posts/release-1.20/), and they come pre-configured out of the box.

Let's take a peek into how to create host labels and apply them across a few of Netdata's features to give you more
organization power over your infrastructure.

## Create unique host labels

Host labels are defined in `netdata.conf`. To create host labels, open that file using `edit-config`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config netdata.conf
```

Create a new `[host labels]` section defining a new host label and its value for the system in question. Make sure not
to violate any of the [host label naming rules](../configuration-guide.md#netdata-labels).

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
    "_is_master": "false",
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
streaming/master status, and more.

They capture the following:

-   Kernel version
-   Operating system name and version
-   CPU architecture, system cores, CPU frequency, RAM, and disk space
-   Whether Netdata is running inside of a container, and if so, the OS and hardware details about the container's host
-   What virtualization layer the system runs on top of, if any
-   Whether the system is a streaming master or slave

If you want to organize your systems without manually creating host tags, try the automatic labels in some of the
features below.

## Host labels in streaming

You may have noticed the `_is_master` and `_is_slave` automatic labels from above. Host labels are also now streamed
from a slave to its master agent, which concentrates an entire infrastructure's OS, hardware, container, and
virtualization information in one place: the master.

Now, if you'd like to remind yourself of how much RAM a certain slave system has, you can simply access
`http://localhost:19999/host/SLAVE_NAME/api/v1/info` and reference the automatically-generated host labels from the
slave system. It's a vastly simplified way of accessing critical information about your infrastructure.

> ⚠️ Because automatic labels for slave nodes are accessible via API calls, and contain sensitive information like
> kernel and operating system versions, you should secure streaming connections with SSL. See the [streaming
> documentation](../..//streaming/README.md#securing-streaming-communications) for details. You may also want to use
> [access lists](../../web/server/README.md#access-lists) or [expose the API only to LAN/localhost
> connections](../netdata-security.md#expose-netdata-only-in-a-private-lan).

You can also use `_is_master`, `_is_slave`, and any other host labels in both health entities and metrics exporting.
Speaking of which...

## Host labels in health entities

You can use host labels to logically organize your systems by their type, purpose, or location, and then apply specific
alarms to them.

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

In a streaming configuration where a master agent is triggering alarms for its slaves, you could create health entities
that apply only to slaves:

```yaml
 host labels: _is_slave = true
```

Or when ephemeral Docker nodes are involved:

```yaml
 host labels: _container = docker
```

Of course, there are many more possibilities for intuitively organizing your systems with host labels. See the [health
documentation](../../health/REFERENCE.md#alarm-line-host-labels) for more details, and then get creative!

## Host labels in metrics exporting

If you have enabled any metrics exporting via our experimental [exporters](../../exporting/README.md), any new host
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
more about exporting, read the [documentation](../../exporting/README.md).

## What's next?

Host labels are a brand-new feature to Netdata, and yet they've already propagated deeply into some of its core
functionality. We're just getting started with labels, and will keep the community apprised of additional functionality
as it's made available. You can also track [issue #6503](https://github.com/netdata/netdata/issues/6503), which is where
the Netdata team first kicked off this work.

It should be noted that while the Netdata dashboard does not expose either user-configured or automatic host labels, API
queries _do_ showcase this information. As always, we recommend you secure Netdata 

-   [Expose Netdata only in a private LAN](../netdata-security.md#expose-netdata-only-in-a-private-lan)
-   [Enable TLS/SSL for web/API requests](../../web/server/README.md#enabling-tls-support)
-   Put Netdata behind a proxy
    -   [Use an authenticating web server in proxy
        mode](../netdata-security.md#use-an-authenticating-web-server-in-proxy-mode)
    -   [Nginx proxy](../Running-behind-nginx.md)
    -   [Apache proxy](../Running-behind-apache.md)
    -   [Lighttpd](../Running-behind-lighttpd.md)
    -   [Caddy](../Running-behind-caddy.md)

If you have issues or questions around using host labels, don't hesitate to [file an
issue](https://github.com/netdata/netdata/issues/new?labels=bug%2C+needs+triage&template=bug_report.md) on GitHub. We're
excited to make host labels even more valuable to our users, which we can only do with your input.
