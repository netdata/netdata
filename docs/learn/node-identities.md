# Node Identities

:::tip

**What You'll Learn**

How Netdata identifies nodes across Agents, Parents, and Cloud - and why each identity type matters for your infrastructure.

:::

Netdata uses several identity mechanisms to uniquely identify nodes, authenticate connections, and track metrics across your infrastructure. Understanding these identities is essential when:

- Creating VM templates or golden images
- Troubleshooting connection issues
- Moving nodes between Spaces
- Setting up Parent-Child streaming
- Configuring virtual nodes for remote monitoring

## Agent Self Identity

Every Netdata Agent has a **Machine GUID** - a UUID that uniquely identifies this specific node.

| Property          | Value                                                                                  |
|-------------------|----------------------------------------------------------------------------------------|
| **File (Linux)**  | `/var/lib/netdata/registry/netdata.public.unique.id`                                   |
| **File (Windows)**| `C:\Program Files\Netdata\var\lib\netdata\registry\netdata.public.unique.id`           |
| **Format**        | UUID (e.g., `a1b2c3d4-e5f6-7890-abcd-ef1234567890`)                                    |
| **Generated**     | On first start, if missing                                                             |
| **Persistence**   | Permanent - never changes once created                                                 |

### Generation Behavior

On startup, Netdata determines the Machine GUID:

1. Read from primary file (`netdata.public.unique.id`)
2. If missing/invalid, read from status file backup
3. If still missing, generate new random UUID
4. Save to primary file with timestamp

### Status File Backups

The Machine GUID is also stored in status files for crash recovery:

| Location                                 | Purpose        |
|------------------------------------------|----------------|
| `/var/lib/netdata/status-netdata.json`   | Primary backup |
| `/var/cache/netdata/status-netdata.json` | Fallback 1     |
| `/tmp/status-netdata.json`               | Fallback 2     |
| `/run/status-netdata.json`               | Fallback 3     |
| `/var/run/status-netdata.json`           | Fallback 4     |

:::note

**Windows status file paths**

On Windows, the status file lives under the install directory (`C:\Program Files\Netdata`):

- Primary: `C:\Program Files\Netdata\var\lib\netdata\status-netdata.json`
- Fallback: `C:\Program Files\Netdata\var\cache\netdata\status-netdata.json`

The same additional fallback locations used on Linux (`/tmp`, `/run`, `/var/run`) also apply on Windows builds, but they resolve through the runtime's own path translation rather than a fixed path under the install directory. If duplicate identities persist after cleaning the two locations above, check those locations too.

:::

:::note

**Redundant Storage**

The Machine GUID is stored redundantly across multiple locations. If the primary file is missing or corrupted, Netdata automatically recovers the GUID from backup locations. This ensures identity persistence across crashes and unexpected shutdowns.

:::

:::warning

**GUID Must Be Unique**

If two Agents have the same Machine GUID:

- They cannot connect to the same Parent simultaneously
- Cloud kicks the older connection offline when the second connects
- This causes unstable "flapping" connections

See [VM Templates](/docs/learn/vm-templates.md) for how to avoid this when cloning VMs.

:::

## Virtual Nodes (vnodes)

**Virtual Nodes** allow Go collectors to report metrics as if they came from separate logical nodes. This is useful for monitoring remote systems, containers, or logical entities that don't run their own Netdata Agent (SNMP devices, cloud provider db instances, etc.).

| Property      | Value                                                                                     |
|---------------|-------------------------------------------------------------------------------------------|
| **Directory** | `vnodes/` in your [Netdata config directory](/docs/netdata-agent/configuration/README.md) |
| **Format**    | YAML files (`.yaml`, `.yml`, `.conf`)                                                     |
| **Identity**  | User-defined GUID in config file                                                          |

### Configuration

Each virtual node is defined in a YAML file:

```yaml
- name: my-remote-server
  hostname: remote-server.example.com
  guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890
  labels:
    environment: production
    datacenter: us-east
```

| Field      | Required | Description                                     |
|------------|----------|-------------------------------------------------|
| `hostname` | Yes      | Display name shown in dashboards and Cloud      |
| `guid`     | Yes      | UUID that uniquely identifies this virtual node |
| `name`     | No       | Internal reference name                         |
| `labels`   | No       | Key-value pairs for filtering and organization  |

:::warning

**GUID Uniqueness**

Each virtual node GUID must be unique across your entire infrastructure. Using the same GUID as another node (real or virtual) causes identity conflicts - the same problems as [duplicate Machine GUIDs](#agent-self-identity).

:::

### Creating Virtual Nodes via the GUI (Dynamic Configuration)

In addition to the YAML file method, you can create, edit, test, and remove virtual nodes directly from the Netdata UI using [dynamic configuration (dyncfg)](/docs/netdata-agent/configuration/dynamic-configuration.md). Both methods produce a working vnode that collectors can attach metrics to, though field requirements differ — see the table below. The Vnodes GUI path is available under the go.d plugin's dynamic configuration view.

:::note

**Where to find it in the GUI**

In the Netdata UI, open the node's dynamic configuration view and look for the **Vnodes** entry under the **go.d** plugin (`/collectors/go.d/Vnodes`).

:::

The GUI form exposes `hostname`, `guid`, and `labels` — the same fields as YAML, minus `name` which the Agent ignores:

| Field      | Required in the GUI | Description                                                                                                                         |
|------------|---------------------|-------------------------------------------------------------------------------------------------------------------------------------|
| `guid`     | Yes                 | UUID that uniquely identifies this virtual node. Must be a valid UUID.                                                              |
| `hostname` | No                  | Display name shown in dashboards and Cloud. If left blank, the job name you assign when creating the vnode is used as the hostname. |
| `labels`   | No                  | Key-value pairs for filtering and organization.                                                                                     |

Generate a `guid` with `uuidgen` on Linux/macOS, or `[guid]::NewGuid()` in PowerShell. Each `guid` must be unique across your infrastructure, as described in the GUID Uniqueness warning above. Each `hostname` must also be unique across all vnodes — the Agent rejects a new vnode whose hostname matches an existing one. See [Does renaming a virtual node change its identity?](#does-renaming-a-virtual-node-change-its-identity) for how each field affects the vnode.

:::info

**Changes apply without an Agent restart**

Vnode changes applied via the GUI propagate immediately to all running collector jobs that reference the vnode — no restart or reload is needed.

:::

:::warning

**Removing vnodes depends on how they were created**

- Virtual nodes you create through the GUI are stored with the `dyncfg` source type and can be removed from the GUI.

- A vnode that is currently referenced by one or more collector jobs cannot be removed until those references are cleared; the remove action is blocked in that case.

- Virtual nodes defined in YAML files in the `vnodes/` directory of your [Netdata config directory](/docs/netdata-agent/configuration/README.md) have a `stock` or `user` source type — attempting to remove them returns an error: only `dyncfg` vnodes can be removed.

- To remove a file-based vnode, delete or edit its YAML file and [restart the Netdata Agent](/docs/netdata-agent/start-stop-restart.md).

:::

### Attaching a Virtual Node to a Collector Job

Defining a vnode is step 1. The vnode becomes active when a collector job references it by setting `vnode` in the job's configuration to the vnode's **hostname**:

```yaml
jobs:
  - name: win_server1
    vnode: win_server1
    url: http://203.0.113.10:9182/metrics
```

The `vnode` value must exactly match the `hostname` of the vnode definition. The Agent resolves the vnode at job startup — if the referenced hostname is not registered, the job fails to start.

:::note

**SNMP** collectors behave differently: their `create_vnode: true` option auto-creates the vnode from the job configuration, so no separate vnode definition step is needed.

:::

### How Virtual Nodes Work

1. Define a vnode (YAML file or GUI) with a unique `hostname` and `guid`
2. Configure the collector job with `vnode: <hostname>` to attach it
3. Metrics are tagged with the vnode's GUID instead of the Agent's Machine GUID
4. Cloud sees the vnode as a separate node in your Space
5. Parent nodes store vnode metadata alongside Children metadata

Virtual nodes appear in Netdata Cloud as independent nodes, with their own dashboards and alert states.

## Hostname Override

By default, Netdata auto-detects the system hostname. When the system hostname is configured as an IP address (common on some cloud VMs or home servers), nodes appear in Dashboards and Netdata Cloud with that raw IP instead of a readable name.

:::info

This setting changes the **display name** only — it does not affect the node's identity (Machine GUID, Node ID, or Claimed ID).

:::

To configure a custom hostname, see [Customizing Your Node Name](/src/daemon/config/README.md#customizing-your-node-name).

## Node Lifecycle and Ephemerality

Every node has a **lifecycle** determined by its **ephemerality** setting. By default, all nodes are **permanent** - they are expected to maintain connectivity, and disconnections trigger alerts.

Ephemeral nodes are designed for dynamic infrastructure:

- No alerts when they disconnect
- Automatic cleanup after configurable timeout
- Perfect for auto-scaling and short-lived workloads

To change a node's ephemerality, edit `netdata.conf`:

```ini
[global]
is ephemeral node = yes   # Ephemeral - no alerts, auto-cleanup
is ephemeral node = no    # Permanent (default) - alerts on disconnect
```

See [Node Ephemerality](/docs/nodes-ephemerality.md) for detailed configuration options.

## Cloud: Node Identity

When an Agent connects to Netdata Cloud, it receives a **Node ID** that links the Machine GUID to your Space.

| Property        | Value                                                        |
|-----------------|--------------------------------------------------------------|
| **Assigned by** | Netdata Cloud                                                |
| **Stored in**   | `/var/cache/netdata/netdata-meta.db` (`node_instance` table) |
| **Purpose**     | Links Machine GUID to your Cloud Space                       |

Machine GUIDs and Cloud Node IDs map 1-to-1.

### Node Instances

A single Machine GUID can have multiple **Node Instances** in Cloud when the same node connects through different Parents. Each Parent-node connection creates a separate node instance. Although, Node Instances are critical in metrics routing decisions and alerts deduplication, they are usually not visible to users.

## Agent-Cloud Link (ACLK) Identity

The **Agent-Cloud Link (ACLK)** uses separate credentials for authentication:

| File                                   | Purpose                                    |
|----------------------------------------|--------------------------------------------|
| `/var/lib/netdata/cloud.d/cloud.conf`  | Cloud configuration, contains `claimed_id` |
| `/var/lib/netdata/cloud.d/private.pem` | RSA private key                            |
| `/var/lib/netdata/cloud.d/public.pem`  | RSA public key                             |

### Claimed ID

The **Claimed ID** is a random UUID generated during the claiming process. It's separate from the Machine GUID, as it uniquely identifies the link between the Agent and Cloud.

| Property         | Description                                                     |
|------------------|-----------------------------------------------------------------|
| **Purpose**      | Authenticates the ACLK connection to Netdata Cloud              |
| **Generated**    | During the claiming process                                     |
| **Independence** | Separate from Machine GUID - they can change independently      |
| **Regeneration** | A new Claimed ID is generated each time the agent is re-claimed |

:::note

**Custom Paths**

If you customized `[directories]` in `netdata.conf`:

- `lib` setting affects `/var/lib/netdata/` paths
- `cache` setting affects `/var/cache/netdata/` paths

:::

:::note

On Windows, the ACLK credentials live under `C:\Program Files\Netdata\var\lib\netdata\cloud.d\`.

:::

## Parent: Children Identities

When a Netdata Agent operates as a **Parent** (receiving metrics from Children), it stores metadata about all nodes it has seen.

| Property     | Value                                |
|--------------|--------------------------------------|
| **Database** | `/var/cache/netdata/netdata-meta.db` |
| **Table**    | `node_instance`                      |
| **Contains** | GUIDs of all Children ever connected |

### Relationship Between Metadata and Metrics

Each metric in Netdata has a UUID. The metadata database (`netdata-meta.db`) links these UUIDs to nodes, charts, and dimensions. The dbengine stores metric samples indexed by these UUIDs.

| Component                              | Purpose                                              |
|----------------------------------------|------------------------------------------------------|
| **Metadata** (`netdata-meta.db`)       | Links metric UUIDs to node GUIDs, charts, dimensions |
| **Dbengine** (`dbengine*` directories) | Stores actual metric samples indexed by UUID         |

The metadata acts as an index - without it, metric samples in dbengine cannot be associated with their source nodes or chart definitions.

### Key Point: Metadata DB Does Not Determine Agent Identity

The metadata database stores information about **all nodes** (including the Agent itself), but this data exists only to link nodes with their metrics. The Agent's identity is **not** determined by the database - it comes exclusively from:

- The GUID file
- Status file backups

### Multiple Node Identities in Database

When a database contains metadata for multiple nodes (from Children or [Virtual Nodes](#virtual-nodes-vnodes)), Netdata:

1. **Reports all nodes** - All known nodes are reported to Netdata Cloud
2. **Retention persistence** - Node entries persist in Cloud until database retention expires (can be years with tiering)

This is normal for Parent nodes receiving data from Children, and for Agents using Virtual Nodes. See [VM Templates](/docs/learn/vm-templates.md) for implications when cloning VMs.

## FAQ

<details>
<summary>How do I find my node's Machine GUID?</summary>

Read the GUID file:

**Linux:**

```bash
cat /var/lib/netdata/registry/netdata.public.unique.id
```

**Windows (PowerShell):**

```powershell
Get-Content "C:\Program Files\Netdata\var\lib\netdata\registry\netdata.public.unique.id"
```

</details>

<details>
<summary>Can I change my node's Machine GUID?</summary>

Yes, but it will appear as a new node in Netdata Cloud and Netdata Parents. Delete the GUID file and status backups, then restart Netdata. See [VM Templates](/docs/learn/vm-templates.md) for the complete procedure.

</details>

<details>
<summary>Why does my node keep going offline/online in Cloud?</summary>

Two agents likely have the same Machine GUID. This causes "flapping" as Cloud kicks the older connection when the second connects. Each agent needs a unique GUID.

</details>

<details>
<summary>What's the difference between Machine GUID, Node ID, and Claimed ID?</summary>

- **Machine GUID**: Agent-generated, identifies the node itself, never changes
- **Node ID**: Cloud-assigned, links the Machine GUID to your Space
- **Claimed ID**: Agent-generated during claiming, identifies the ACLK connection

</details>

<details>
<summary>Do containers need unique GUIDs?</summary>

Containers with ephemeral storage get unique GUIDs automatically on each start. Containers with persistent volumes at `/var/lib/netdata/` retain their GUID across restarts.

</details>

<details>
<summary>Can the same node exist in multiple Spaces?</summary>

No. A node can only exist in one Space. To move a node to a different Space, unclaim it first, then claim to the new Space.

</details>

<details>
<summary>Does ephemerality affect my node's identity?</summary>

No. **Identity** and **ephemerality** are separate concepts:

- **Identity** (Machine GUID, Node ID) uniquely identifies the node
- **Ephemerality** determines lifecycle behavior (alerts, cleanup)

A node can be permanent or ephemeral and still have a unique identity. Changing ephemerality does not change or reset identity.

</details>

<details>
<summary>Can I change a node's ephemerality after cloning?</summary>

Yes. Edit `netdata.conf` in your [Netdata config directory](/docs/netdata-agent/configuration/README.md) and restart Netdata:

```ini
[global]
is ephemeral node = yes   # Enable ephemeral behavior
is ephemeral node = no    # Revert to permanent (default)
```

Changes apply immediately. Ephemerality is stored as a host label and propagates to Parents and Netdata Cloud.

</details>

<details>
<summary>How do I rename a node?</summary>

A node's display name is determined by the `hostname` setting in `netdata.conf` under the `[global]` section. To rename a node, edit `netdata.conf` and set:

```ini
[global]
    hostname = my-new-node-name
```

Use the [`edit-config` script](/docs/netdata-agent/configuration/README.md#edit-configuration-files) to safely edit configuration files, then [restart Netdata](/docs/netdata-agent/start-stop-restart.md).

:::note

Changing the hostname does **not** change the Machine GUID, Node ID, or Claimed ID. The node remains the same entity in Netdata Cloud and on Parent nodes. Historical metrics are preserved because they are keyed by Machine GUID, not hostname.

:::

:::warning

**Do not use `NETDATA_HOSTNAME` as an environment variable to set the hostname.**

`NETDATA_HOSTNAME` is an output variable set by the Netdata daemon at runtime for use by plugins and scripts — it is not an input configuration. To override a node's name within Netdata, use the `hostname` setting in `netdata.conf`. If `hostname` is not set, Netdata falls back to the system/container hostname.

:::

The updated hostname propagates to Parent nodes and Netdata Cloud on the next connection.

For virtual nodes, see [Does renaming a virtual node change its identity?](#does-renaming-a-virtual-node-change-its-identity).

</details>

<a id="does-renaming-a-virtual-node-change-its-identity"></a>
<details>
<summary>Does renaming a virtual node change its identity?</summary>

A virtual node's identity is determined by its **`guid`** field — not its `hostname` or `name`. The fields behave as follows:

- **`guid`** — This is the vnode's identity. Changing it creates an entirely new node in Netdata Cloud. The old vnode's historical data remains under the old GUID but is no longer associated with the new one.
- **`hostname`** — This is used as the internal lookup key in the Agent and as the display name in dashboards. Changing `hostname` while keeping the same `guid` renames the display without creating a new node identity.
- **`name`** — The Agent ignores this field. When set to a value different from `hostname`, the Agent logs a warning and overrides it with `hostname`.

**To preserve data continuity when renaming a vnode**, change only the `hostname` field in its YAML file in the `vnodes/` directory of your [Netdata config directory](/docs/netdata-agent/configuration/README.md) and keep the `guid` unchanged. If a true identity change is needed, accept that historical data belongs to the old identity.

</details>

<details>
<summary>How do I find the UUID of my existing vnode?</summary>

The GUID for each virtual node is stored in its YAML configuration file in the `vnodes/` directory of your [Netdata config directory](/docs/netdata-agent/configuration/README.md). To look it up:

```bash
# Default path — adjust if your Netdata config directory differs.
cat /etc/netdata/vnodes/*
```

Each file contains a `guid` field that uniquely identifies the vnode:

```yaml
- name: my-remote-server
  hostname: remote-server.example.com
  guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890
```

The `guid` value is the vnode's UUID. You do **not** need to query any internal database — the YAML configuration file is the authoritative source for the vnode GUID. See [Virtual Nodes](#virtual-nodes-vnodes) for the full configuration reference.

</details>
