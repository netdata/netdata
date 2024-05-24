# Deployment strategies

## Deployment Options Overview

This section provides a quick overview of a few common deployment options for Netdata. The next sections go into configuration examples and further reading.

### Stand-alone Deployment

To help our users have a complete experience of Netdata when they install it for the first time, the Netdata Agent with default configuration is a complete monitoring solution out of the box, with features enabled and available.

The Agent will act as a **stand-alone** Agent by default. This is great to start out with for small setups and home labs. By [connecting each Agent to Cloud](/src/claim/README.md), you can see an overview of all your nodes, with aggregated charts and centralized alerting, without setting up a Parent.

![image](https://github.com/netdata/netdata/assets/116741/6a638175-aec4-4d46-85a6-520c283ab6a8)

### Parent – Child Deployment

An Agent connected to a Parent is called a **Child**. It will **stream metrics** to its Parent. The Parent can then take care of storing metrics on behalf of that node (with longer retention), handle metrics queries for showing dashboards and provide alerting.

When using Cloud, it is recommended that just the Parent is connected to Cloud. Child Agents can then be configured to have short retention, in RAM instead of on Disk, and have alerting and other features disabled.

Due to the fact they don't need to connect to Cloud themselves, those Children can then be further secured by not allowing outbound traffic.

![image](https://github.com/netdata/netdata/assets/116741/cb65698d-a6b7-43ee-a2d1-c30d0a46f084)

This setup allows for leaner Child nodes and is good for setups with more than a handful of nodes. Metrics remain accessible if the Child node is temporarily unavailable or decommissioned, although there is no failover in case the Parent becomes unavailable.

### Active–Active Parent Deployment

For high availability, Parents can be configured to stream data for their Children between them, and keep their data sets in sync. Children are configured with the addresses of both Parents, but will only stream to one of them at a time. When one Parent becomes unavailable, the Child reconnects to the other. When the first Parent becomes available again, that Parent will catch up by receiving the backlog from the second.

With both Parent Agents connected to Cloud, Cloud will route queries to either of them transparently, depending on their availability. Alerts trigger on either Parent will stream to Cloud, and Cloud will deduplicate and debounce state changes to prevent spurious notifications.

![image](https://github.com/netdata/netdata/assets/116741/6ae2b10c-7f7d-4503-aac4-0a9381c6f80b)

## Deployment Configuration Details

### Stand-alone

The stand-alone setup is configured out of the box with reasonable defaults, but please consult our [configuration documentation](/docs/netdata-agent/configuration/README.md) for details, including the overview of [common configuration changes](/docs/netdata-agent/configuration/common-configuration-changes.md).

### Parent – Child

For setups involving Parent and Child Agents, they need to be configured for [streaming](docs/observability-centralization-points/metrics-centralization-points/configuration.md), through the configuration file `stream.conf`.

This will instruct the Child to stream data to the Parent and the Parent to accept streaming connections for one or more Child Agents. To secure this connection, both need a shared API key (to replace the string `API_KEY` in the examples below). Additionally, the Child can be configured with one or more addresses of Parent Agents (`PARENT_IP_ADDRESS`).

An API key is a key created with `uuidgen` and is used for authentication and/or customization on the Parent side. For example, a Child can stream using the API key, and a Parent can be configured to accept connections from the Child, but it can also apply different options for Children by using multiple different API keys. The easiest setup uses just one API key for all Child Agents.

#### Child config

As mentioned above, we do not recommend to claim the Child to Cloud directly during your setup.

This is done in order to reduce the footprint of the Netdata Agent on your production system, as some capabilities can be switched OFF for the Child and kept ON for the Parent.

In this example, Machine Learning and Alerting are disabled for the Child, so that the Parent can take the load. We also use RAM instead of disk to store metrics with limited retention, covering temporary network issues.

##### netdata.conf

On the child node, edit `netdata.conf` by using the [edit-config](docs/netdata-agent/configuration/README.md#edit-netdataconf) script and set the following parameters:

```yaml
[db]
    # https://github.com/netdata/netdata/blob/master/src/database/README.md
    # none = no retention, ram = some retention in ram
    mode = ram
    # The retention in seconds.
    # This provides some tolerance to the time the child has to find a parent in
    # order to transfer the data. For IoT this can be lowered to 120.
    retention = 1200
    # The granularity of metrics, in seconds.
    # You may increase this to lower CPU resources.
    update every = 1
[ml]
    # Disable Machine Learning
    enabled = no
[health]
    # Disable Health Checks (Alerting)
    enabled = no
[web]
    # Disable remote access to the local dashboard
    bind to = lo
[plugins]
    # Uncomment the following line to disable all external plugins on extreme
    # IoT cases by default.
    # enable running new plugins = no
```

##### stream.conf

To edit `stream.conf`, use again the [edit-config](docs/netdata-agent/configuration/README.md#edit-netdataconf) script and set the following parameters:

```yaml
[stream]
    # Stream metrics to another Netdata
    enabled = yes
    # The IP and PORT of the parent
    destination = PARENT_IP_ADDRESS:19999
    # The shared API key, generated by uuidgen
    api key = API_KEY
```

#### Parent config

For the Parent, besides setting up streaming, this example also provides configuration for multiple [tiers of metrics storage](/docs/netdata-agent/configuration/optimizing-metrics-database/change-metrics-storage.md#calculate-the-system-resources-ram-disk-space-needed-to-store-metrics), for 10 Children, with about 2k metrics each. This allows for:

- 1s granularity at tier 0 for 1 week
- 1m granularity at tier 1 for 1 month
- 1h granularity at tier 2 for 1 year

Requiring:

- 25GB of disk
- 3.5GB of RAM (2.5GB under pressure)

##### netdata.conf

On the Parent, edit `netdata.conf` by using the [edit-config](docs/netdata-agent/configuration/README.md#edit-netdataconf) script and set the following parameters:

```yaml
[db]
    mode = dbengine
    storage tiers = 3
    # To allow memory pressure to offload index from ram
    dbengine page descriptors in file mapped memory = yes
    # storage tier 0
    update every = 1
    dbengine multihost disk space MB = 12000
    dbengine page cache size MB = 1400
    # storage tier 1
    dbengine tier 1 page cache size MB = 512
    dbengine tier 1 multihost disk space MB = 4096
    dbengine tier 1 update every iterations = 60
    dbengine tier 1 backfill = new
    # storage tier 2
    dbengine tier 2 page cache size MB = 128
    dbengine tier 2 multihost disk space MB = 2048
    dbengine tier 2 update every iterations = 60
    dbengine tier 2 backfill = new
[ml]
    # Enabled by default
    # enabled = yes
[health]
    # Enabled by default
    # enabled = yes
[web]
    # Enabled by default
    # bind to = *
```

##### stream.conf

On the Parent node, edit `stream.conf` by using the [edit-config](docs/netdata-agent/configuration/README.md#edit-netdataconf) script and set the following parameters:

```yaml
[API_KEY]
    # Accept metrics streaming from other Agents with the specified API key
    enabled = yes
```

### Active–Active Parents

In order to setup active–active streaming between Parent 1 and Parent 2, Parent 1 needs to be instructed to stream data to Parent 2 and Parent 2 to stream data to Parent 1. The Child Agents need to be configured with the addresses of both Parent Agents. An Agent will only connect to one Parent at a time, falling back to the next upon failure. These examples use the same API key between Parent Agents and for connections for Child Agents.

On both Netdata Parent and all Child Agents, edit `stream.conf` by using the [edit-config](docs/netdata-agent/configuration/README.md#edit-netdataconf) script:

#### stream.conf on Parent 1

```yaml
[stream]
    # Stream metrics to another Netdata
    enabled = yes
    # The IP and PORT of Parent 2
    destination = PARENT_2_IP_ADDRESS:19999
    # This is the API key for the outgoing connection to Parent 2
    api key = API_KEY
[API_KEY]
    # Accept metrics streams from Parent 2 and Child Agents
    enabled = yes
```

#### stream.conf on Parent 2

```yaml
[stream]
    # Stream metrics to another Netdata
    enabled = yes
    # The IP and PORT of Parent 1
    destination = PARENT_1_IP_ADDRESS:19999
    api key = API_KEY
[API_KEY]
    # Accept metrics streams from Parent 1 and Child Agents
    enabled = yes
```

#### stream.conf on Child Agents

```yaml
[stream]
    # Stream metrics to another Netdata
    enabled = yes
    # The IP and PORT of the parent
    destination = PARENT_1_IP_ADDRESS:19999 PARENT_2_IP_ADDRESS:19999
    # The shared API key, generated by uuidgen
    api key = API_KEY
```

## Further Reading

We strongly recommend the following configuration changes for production deployments:

1. Understand Netdata's [security and privacy design](/docs/security-and-privacy-design/README.md) and [secure your nodes](/docs/netdata-agent/securing-netdata-agents.md)

   To safeguard your infrastructure and comply with your organization's security policies.

2. [Optimize the Netdata Agents system utilization and performance](/docs/netdata-agent/configuration/optimize-the-netdata-agents-performance.md)

   To save valuable system resources, especially when running on weak IoT devices.

We also suggest that you:

1. [Use Netdata Cloud to access the dashboards](/docs/netdata-cloud/monitor-your-infrastructure.md)

   For increased security, user management and access to our latest features, tools and troubleshooting solutions.

2. [Change how long Netdata stores metrics](/docs/netdata-agent/configuration/optimizing-metrics-database/change-metrics-storage.md)

   To control Netdata's memory use, when you have a lot of ephemeral metrics.

3. [Use host labels](/docs/netdata-agent/configuration/organize-systems-metrics-and-alerts.md)

   To organize systems, metrics, and alerts.
