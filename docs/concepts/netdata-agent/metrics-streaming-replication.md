<!--
title: "Metrics streaming-replication"
sidebar_label: "Metrics streaming-replication"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/metrics-streaming-replication.md"
sidebar_position: "1300"
learn_status: "Published"
learn_topic_type: "Concepts"
learn_rel_path: "Concepts/Netdata agent"
learn_docs_purpose: "Explain the streaming and replication concepts"
-->

Each node running Netdata can stream the metrics it collects, in real time, to another node. Streaming allows you to
replicate metrics data across multiple nodes, or centralize all your metrics data into a single time-series database
(TSDB).

### Streaming

Streaming is the process where data transfer from one Agent (**Child**) to another (**Parent**). When one node streams
metrics to another, the
node receiving metrics practically owns the data in its database, it can
visualize them on the dashboard, run health checks to trigger alarms, send notifications, and export all metrics to an
external TSDB. When Netdata streams metrics to another Netdata, the receiving one is able to perform everything a
Netdata instance is capable of.

Streaming lets you decide exactly how you want to store and maintain metrics data. While we believe Netdata's
[distributed architecture](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-architecture/distributed-data-architecture.md)
is ideal for speed and scale, streaming
provides centralization options for those who want to maintain only a single TSDB instance.

### Streaming cases

There are three types of nodes in Netdata's streaming ecosystem.

- **Parent**: A node, running Netdata, that receives streamed metric data.
- **Child**: A node, running Netdata, that streams metric data to one or more parent.
- **Proxy**: A node, running Netdata, that receives metric data from a child and "forwards" them on to a
  separate parent node.

Netdata uses API keys, which are just random GUIDs, to create an endpoint on the Agent and authorize the communication
between child and parent nodes. We recommend using `uuidgen` for generating API keys, which can then be used across any
number of streaming connections. Or, you can generate unique API keys for each parent-child relationship.

Once the parent node authorizes the child's API key, the child can start streaming metrics.

It's important to note that the streaming connection uses TCP, UDP, or Unix sockets, _not HTTP_. To proxy streaming
metrics, you need to use a proxy that tunnels [OSI layer 4-7
traffic](https://en.wikipedia.org/wiki/OSI_model#Layer_4:_Transport_Layer) without interfering with it, such as
[SOCKS](https://en.wikipedia.org/wiki/SOCKS) or Nginx's [TCP/UDP load
balancing](https://docs.nginx.com/nginx/admin-guide/load-balancer/tcp-udp-load-balancer/).

### Replication

The replication process is a subprocess of the streaming. When you activate replication all the available data in the
particular node are mirrored to the parent. Below you can see the configuration of an Agent in Child mode which is
ready to replicate its data when streaming is enabled.

```yaml
[ db ]
  . . .
  # enable replication = yes
  # seconds to replicate = 86400
  # seconds per replication step = 600
  . . . 
```

The child and the parent may have different data retention policies for the same metrics. Alerts for the child are
triggered by **both** the child and the parent. It is possible to enable different alert
configurations on the parent and the child. In order for custom chart names on the child to work correctly, follow the
form `type.name`. The parent will truncate the `type` part and substitute the original chart `type` to store the name in
the database.

### Supported streaming configurations

Netdata supports any combination of parent, child, and proxy nodes that you can imagine. Any node can act as both a
parent, child, or proxy at the same time, sending or receiving streaming metrics from any number of other nodes.

Here are a few example streaming configurations:

- **Headless collector**:
    - Child `A`, _without_ a database or web dashboard, streams metrics to parent `B`.
    - `A` metrics are only available via the local Agent dashboard for `B`.
    - `B` generates alarms for `A`.
- **Replication**:
    - Child `A`, _with_ a database and web dashboard, streams metrics to parent `B`.
    - `A` metrics are available on both local Agent dashboards, and can be stored with the same or different metrics
      retention policies.
    - Both `A` and `B` generate alarms.
- **Proxy**:
    - Child `A`, _with or without_ a database, sends metrics to proxy `C`, also _with or without_ a database. `C` sends
      metrics to parent `B`.
    - Any node with a database can generate alarms.

### Viewing streamed metrics

Parent nodes feature a **Replicated Nodes** section in the left-hand panel, which opens with the hamburger icon
![Hamburger icon](https://raw.githubusercontent.com/netdata/netdata-ui/master/src/components/icon/assets/hamburger.svg)
in the top navigation. The parent node, plus any child nodes, appear here. Click on any of the hostnames to switch
between parent and child dashboards, all served by the
parent's [web server](https://github.com/netdata/netdata/blob/master/web/server/README.md).

![Switching between
](https://user-images.githubusercontent.com/1153921/110043346-761ec000-7d04-11eb-8e58-77670ba39161.gif)

Each child dashboard is also available directly at the following URL pattern:
`http://PARENT-NODE:19999/host/CHILD-HOSTNAME`.

## Related topics

### Related Concepts

- [ACLK](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/aclk.md)
- [Registry](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/registry.md)
- [Metrics streaming/replication](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/metrics-streaming-replication.md)
- [Metrics exporting](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/metrics-exporting.md)
- [Metrics collection](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/metrics-collection.md)
- [Metrics storage](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/metrics-storage.md)

### Related References

- [Streaming reference](https://github.com/netdata/netdata/blob/master/streaming/README.md)
