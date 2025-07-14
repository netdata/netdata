# Organize systems, metrics, and alerts

When you monitor dozens or hundreds of systems, you need powerful ways to keep everything organized. Netdata helps you structure your infrastructure with Spaces, Rooms, virtual nodes, host labels, and metric labels.

## Choose your organization strategy

Netdata provides multiple organization methods that work together:

- **Spaces and Rooms**: Group your infrastructure and team members
- **Virtual nodes**: Monitor multi-component systems as separate entities  
- **Host labels**: Tag systems by purpose, location, or any custom criteria
- **Metric labels**: Filter and group metrics within charts


### Organize your infrastructure and team

<details>
<summary><strong>Spaces</strong> are your primary collaboration environment where you:</summary>
<br/>
• Organize team members and manage access levels<br/>
• Connect nodes for monitoring<br/>
• Create a unified monitoring environment
</details><br/>

<details>
<summary><strong>Rooms</strong> function as organizational units within Spaces, providing:</summary>
<br/>
• Infrastructure-wide dashboards<br/>
• Real-time metrics visualization<br/>
• Focused monitoring views<br/>
• Flexible node grouping
</details><br/>

:::info

Each node belongs to exactly one Space but can be assigned to multiple Rooms within that Space.

:::

### Set up your organization

1. **Create a Space** using the plus (+) icon in the left-most sidebar
2. **Invite team members** and set their access levels
3. **Create Rooms** to organize nodes by:
   - Service type (Nginx, MySQL, Pulsar)
   - Purpose (webserver, database, application)
   - Location or infrastructure type (cloud provider, bare metal, containers)

:::tip

Most organizations need only one Space. Create multiple Rooms within that Space to organize your infrastructure effectively.

:::

Learn more in our [Spaces and Rooms documentation](/docs/netdata-cloud/organize-your-infrastructure-invite-your-team.md).

## Virtual nodes

### Monitor complex systems as separate entities

Virtual nodes let you split multi-component systems into distinct, monitorable units. For example, you can monitor each Windows server in your infrastructure as its own node, even when collecting metrics through a single Netdata Agent.

To create a virtual node for your Windows server:

1. Define the virtual node in `/etc/netdata/vnodes/vnodes.conf`:

    ```yaml
    - hostname: win_server1
      guid: <value>
    ```

    :::tip
    Generate a valid GUID using `uuidgen` on Linux or `[guid]::NewGuid()` in Windows PowerShell.
    :::

2. Add the vnode configuration to your data collection job in `go.d/windows.conf`:

    ```yaml
    jobs:
      - name: win_server1
        vnode: win_server1
        url: http://203.0.113.10:9182/metrics
    ```

## Host labels

### Tag your systems for smarter monitoring

Host labels help you:
- Create alerts that adapt to each system's purpose
- Archive metrics with proper categorization for analysis
- Track ephemeral containers in Kubernetes clusters

### Use automatic labels

Netdata automatically generates host labels when it starts, capturing:

| Label Category | Information Captured |
|----------------|---------------------|
| System Info | Kernel version, OS name and version |
| Hardware | CPU architecture, cores, frequency, RAM, disk space |
| Environment | Container details, Kubernetes node status |
| Infrastructure | Virtualization layer, Parent-child streaming status |

View your automatic labels at `http://HOST-IP:19999/api/v1/info`:

```json
{
  ...
  "host_labels": {
    "_is_k8s_node": "false",
    "_is_parent": "false",
    ...
```

### Create custom labels

Add your own labels to categorize systems by any criteria you need.

1. Edit your Netdata configuration:

    ```bash
    cd /etc/netdata   # Replace with your Netdata config directory
    sudo ./edit-config netdata.conf
    ```

2. Add a `[host labels]` section:

    ```text
    [host labels]
        type = webserver
        location = us-seattle
        installed = 20200218
    ```

    :::info Label naming rules
    - Names cannot start with `_`
    - Use only letters, numbers, dots, and dashes
    - Values cannot contain: `!` ` ` `'` `"` `*`
    :::

3. Enable your labels without restarting Netdata:

    ```bash
    netdatacli reload-labels
    ```

4. Verify your labels at `http://HOST-IP:19999/api/v1/info`

### Stream labels from Child to Parent

In Parent-Child setups, host labels automatically stream from children to the parent node. Access any child's labels through the parent at:
`http://localhost:19999/host/CHILD_HOSTNAME/api/v1/info`

:::warning

Child node labels contain sensitive system information. Secure your streaming connections with SSL and consider using [access lists](/src/web/server/README.md#access-lists) or [restricting API access](/docs/netdata-agent/securing-netdata-agents.md#restrict-dashboard-access-to-private-lan).

:::

### Apply labels to alerts

Create targeted alerts based on host labels. For example, monitor disk space only on webservers:

```yaml
template: disk_fill_rate
      on: disk.space
  lookup: max -1s at -30m unaligned of avail
    calc: ($this - $avail) / (30 * 60)
   every: 15s
host labels: type = webserver
```

Target systems by multiple criteria:

| Target | Host Label | Use Case |
|--------|------------|----------|
| Specific OS | `_os_name = Debian*` | Apply alerts to Debian systems |
| Child nodes only | `_is_child = true` | Monitor streaming children |
| Docker containers | `_container = docker` | Container-specific alerts |

See the [health documentation](/src/health/REFERENCE.md#alert-line-host-labels) for more possibilities.

### Export labels with metrics

When using [metrics exporters](/src/exporting/README.md), include host labels with your exported data:

```text
[exporting:global]
enabled = yes
send configured labels = yes
send automatic labels = no
```

Configure per-connection settings:

```text
[opentsdb:my_instance3]
enabled = yes
destination = localhost:4242
data source = sum
update every = 10
send charts matching = system.cpu
send configured labels = no
send automatic labels = yes
```

## Metric labels

### Filter and group metrics within charts

Netdata's aggregate charts let you filter and group metrics using label name-value pairs. All go.d plugin collectors support labels at the collection job level.

Configure metric labels when collecting from multiple sources. For example, label two Apache servers by service and location:

```yaml
jobs:
  - name: my_webserver1
    url: http://host1/server-status?auto
    labels:
      service: "Payments"
      location: "Atlanta"
  - name: my_webserver2
    url: http://host2/server-status?auto
    labels:
      service: "Payments"
      location: "New York"
```

:::tip

Define as many label pairs as you need across all your data collection jobs to create meaningful groupings in your dashboards.

:::

## Next steps

1. **Start with Spaces and Rooms** to organize your infrastructure and team
2. **Add host labels** to categorize your systems
3. **Configure metric labels** for detailed filtering within charts
4. **Set up virtual nodes** if you monitor complex, multi-component systems