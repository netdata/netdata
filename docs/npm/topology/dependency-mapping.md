<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/npm/topology/dependency-mapping.md"
sidebar_label: "Application Dependency Mapping"
learn_status: "Published"
learn_rel_path: "Network Performance Monitoring/Topologies"
keywords: ['dependency mapping', 'service map', 'network connections', 'sockets', 'processes', 'containers', 'kubernetes', 'systemd', 'topology']
endmeta-->

<!-- markdownlint-disable-file -->

# Application Dependency Mapping

The same topology view that draws your switches also draws your software. On a monitored host, Netdata reads the
kernel's live socket table and turns it into a dependency map: which process talks to which, over which port —
attributed, on Linux, to the container, image, systemd unit, or Kubernetes pod that owns it.

There is nothing to instrument. No language agents, no sidecars, no code changes, no service mesh. These are the sockets
your kernel already has, enumerated fresh each time you open the map and drawn as a graph. It is a live picture of now,
not a stored history — the map shows what is connected at the moment you ask.

![Live network connections function](https://www.netdata.cloud/img/dashboard-screens/functions-network-connections.png)

## What it maps

For every TCP and UDP socket on the host, IPv4 and IPv6, Netdata identifies the local process behind it:

- **The process** — the running program, its command line, the user it runs as, and its parent.
- **The container** *(Linux)* — the container name and the image it came from.
- **The Kubernetes workload** *(Linux)* — the pod, the namespace, and the workload it belongs to.
- **The systemd unit** *(Linux)* — for services that aren't containerized.
- **The host itself** — for connections that belong to no particular service.

The other end depends on where it is. When both ends of a connection live on the same host, Netdata draws a direct
process-to-process link. When the peer is elsewhere, it is drawn as an endpoint actor carrying its address, and the map
records the matching keys needed to resolve it against the process serving it on another monitored host.

The result is a graph of your applications rather than a list of sockets: this pod talks to that database, this service
depends on that queue, this host reaches out to that external address.

Identification is best-effort. A socket whose owning process Netdata can see but cannot name — one that exited between
reads, for example — is still drawn, with `[unknown]` where the name or user would be. Enumerating every process's
sockets needs privileged access, which standard installations grant; where it is missing the map is quietly
incomplete rather than wrong.

Running the Agent in a container needs more than the default: the host network namespace and the host `/proc` to see
anything beyond the container's own sockets, `SYS_ADMIN` to reach sibling containers' connections, and `SYS_PTRACE` to
attribute connections to processes — without `SYS_PTRACE` you get the connections but not the software behind them,
which is the point of this map. Netdata's own container images and Helm chart request these already. On macOS, a
non-privileged or TCC-restricted Agent omits protected processes altogether. The plugin logs a warning when it detects
that its view was truncated.

## Group the map the way you think

The map redraws around whichever level you want to reason about:

- **By process name** (the default) — every process with the same name on that host treated as one, so eight worker
  processes of the same service are one box, not eight.
- **By container** — how the host's containers and pods relate, with the processes inside them collapsed away.
- **By PID** — every individual process, when you need to see which specific worker is responsible.

Grouping applies within the host: actors are identified per node, so identical processes on two different hosts stay
two boxes.

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

| Platform | Dependency map | Process identity | Container / Kubernetes / systemd |
|:---|:---|:---|:---|
| **Linux** | yes | yes | yes |
| **FreeBSD** | yes | yes | no |
| **macOS** | yes | yes | no |
| **Windows** | yes | yes | no |

On Linux, container and Kubernetes attribution comes from the cgroup each socket's process belongs to. Netdata
recognizes Docker, Kubernetes, Podman, LXC, systemd-nspawn, KVM guests, and plain systemd units, with no per-runtime
configuration. On FreeBSD, macOS, and Windows
the map is drawn from processes and endpoints only — the cgroup enrichment that supplies container and workload
identity is Linux-specific. On Windows, UDP rows are listener-only because the IP Helper API does not expose
remote endpoints.

Windows has this map too, drawn from processes and endpoints only (no container/Kubernetes/systemd
attribution, matching FreeBSD and macOS). UDP sockets appear as listeners only because the IP Helper API does not
expose remote endpoints.

## How to open it

The map is served by the **`topology:network-connections`** function — open it from the topology view. It comes up on
its own: the plugin is enabled by default and needs no setup. Its one setting, `apps lookup cache size`, is the number
of per-PID entries the APPS_LOOKUP cache keeps — the cgroup identity already resolved for each process. It lives in
`netdata.conf` under `[plugin:network-viewer]` and rarely needs changing; raising it does not make more processes
resolvable, it only keeps more of the resolved ones cached.

The function is not anonymous. It exposes process names, command lines, users, and every address the host talks to, so
it requires a signed-in Netdata identity, membership of the same Space as the node, and permission to view sensitive
data. If the map is missing while the node is otherwise healthy, check your access before suspecting the collector.

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
