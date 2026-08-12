<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/npm/topology/dependency-mapping.md"
sidebar_label: "Application Dependency Mapping"
learn_status: "Published"
learn_rel_path: "Network Performance Monitoring/Topologies"
keywords: ['dependency mapping', 'service map', 'network connections', 'sockets', 'processes', 'containers', 'kubernetes', 'systemd', 'topology']
endmeta-->

<!-- markdownlint-disable-file -->

# Application Dependency Mapping

The same topology view that draws your switches also draws your software. On every host, Netdata reads the kernel's live
socket table and turns it into a dependency map: which process talks to which, over which port — attributed to the
container, image, systemd unit, or Kubernetes pod that owns it.

There is nothing to instrument. No language agents, no sidecars, no code changes, no service mesh. These are the sockets
your kernel already has, read continuously and drawn as a graph.

![Live network connections function](https://www.netdata.cloud/img/dashboard-screens/functions-network-connections.png)

## What it maps

Every connection Netdata sees is attributed to the software on both ends of it, as far as the host can tell:

- **The process** — the running program, its command line, the user it runs as, and its parent.
- **The container** — the container name and the image it came from.
- **The Kubernetes workload** — the pod, the namespace, and the workload it belongs to.
- **The systemd unit** — for services that aren't containerized.
- **The host itself** — for connections that belong to no particular service.

The result is a graph of your applications rather than a list of sockets: this pod talks to that database, this service
depends on that queue, this host reaches out to that external address.

## Group the map the way you think

The map redraws around whichever level you want to reason about:

- **By node** — how your hosts depend on each other.
- **By container** — how your containers and pods depend on each other, with the processes inside them collapsed away.
- **By process name** — all instances of a service treated as one, so a 12-replica deployment is one box, not twelve.
- **By PID** — every individual process, when you need to see which specific worker is responsible.

Start grouped, then drill down. A dependency map that shows every process on a busy host is accurate and unreadable;
the value is in choosing the altitude that answers your question.

## What you can do with it

- **Understand a service you didn't build** — see what it actually talks to, instead of reading a diagram someone drew
  two years ago.
- **Check the blast radius before you change something** — see what depends on the service you're about to restart,
  move, or retire.
- **Find the unexpected dependency** — the connection to a machine nobody remembered, the service still calling a
  deprecated endpoint, the process reaching outside your network.
- **Confirm what an incident touches** — when a service degrades, see which other services sit downstream of it.
- **Verify segmentation** — see whether things that should not talk to each other are talking to each other.

## Platform support

| Platform | What you get |
|:---|:---|
| **Linux** | Full process, container, Kubernetes, and systemd attribution. |
| **Windows** | Connection monitoring, including SMB. |
| **FreeBSD** | Connection monitoring. |

On Linux, container and Kubernetes attribution comes from the cgroup each socket's process belongs to, so it works for
Docker, containerd, CRI-O, LXC, and plain systemd services without any per-runtime configuration.

## How to open it

The map is served by the **`topology:network-connections`** function — open it from the topology view. It comes up on
its own: the plugin is enabled by default and has nothing to configure.

Two levels of detail are available:

- **Aggregated** (the default) — the dependency graph.
- **Detailed** — the same graph plus the individual socket evidence behind every link, when you need to see the exact
  connections a link is made of.

The related **Network Connections** function shows the same sockets as a searchable table, with states, ports, and
per-connection metrics — useful when you already know what you're looking for and want the raw rows rather than the
graph.

## What's next

- [Overview](/docs/npm/topology/README.md) — the other sources the topology view brings together.
- [Discovery Methods](/docs/npm/topology/discovery-methods.md) — how the network device fabric is discovered.
