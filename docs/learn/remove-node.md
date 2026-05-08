# Removing a node from your Netdata Cloud Space

You can remove a node from your Space in Netdata Cloud, but the process depends on the node's current state and requires different approaches for different scenarios.

## Understanding Node States

Before attempting to remove a node, it's important to understand what each status means:

* **Online** (Live): Node is actively connected and streaming data
* **Stale**: Node disconnected, but a Parent connected to Netdata Cloud has its historical data
* **Offline** (Unreachable): Node is disconnected and no longer has data available for querying

For a complete explanation of node states, state transitions, and when nodes move between states, see [Node States and Transitions](/docs/netdata-cloud/node-states-and-transitions.md).

<details>
<summary><strong>Why Can't I Delete Stale Nodes?</strong></summary><br/>

**Stale means this node is a Child that has stopped streaming to its Parent, but the Parent still retains its historical data.**

You can't delete a Stale node because the Parent is alive/connected to the Cloud and has data for the Stale node. The UI disables the "Remove" option to protect this historical data and maintain the parent-child relationship integrity.<br/>

This is why stale nodes show "Delete is disabled" - the system prevents deletion while the parent node still holds queryable metrics data for that child.<br/>
</details>


## Quick Decision Guide

**What type of node is it?**

```
🔴 Standalone node (connects directly to Cloud) → Removing Standalone Nodes
🟡 Child node (streams through a Parent Agent) → Removing Child Nodes
```

**Then, what's the node status?**

```
📴 Node shows "Offline" → Use the UI removal method
🟡 Node shows "Stale" → Use the CLI method
```

:::note

You need **Admin** role in your Space to remove nodes. The CLI method requires a system with Netdata Agent installed.

:::

## Singular or Standalone Nodes

These are nodes that connect directly to Netdata Cloud without streaming through a Parent Agent.

<details>
<summary><strong>Removing Offline Standalone Nodes (UI Method)</strong></summary><br/>

**When to use**: Your standalone node shows as **Offline** status in Netdata Cloud.

:::note

Stale status does not apply to standalone nodes — stale specifically means a child node stopped streaming to its parent.

:::

**Steps**:
1. Stop the Netdata Agent on the node you want to remove
2. In Netdata Cloud, go to **Space Settings > Nodes** (click the ⚙️ cog icon below the spaces list).
3. Locate the offline node in the list
4. Select the trash icon to remove it

:::note

The **Remove** option is only available in the **Space Settings** view. It will appear disabled in the "All Nodes" room or other parts of the UI.

:::

</details>

## Child Nodes

These are nodes that stream metrics through a Parent Agent. When a child node stops streaming to its parent, it becomes **Stale**.

<details>
<summary><strong>Removing Stale Child Nodes (CLI Method)</strong></summary><br/>

**When to use**: Your child node shows as **Stale** status and UI shows "Delete is disabled".

**Step 1: Get the Node Identifier**
1. In Netdata Cloud, navigate to the stale node
2. Click the **node information (i)** button 
3. Click **"View node info in JSON"**
4. Copy the identifier from the JSON data (node_id, machine_guid, or hostname)

**Step 2: Remove the Stale Node**
Run this command on the **Parent Agent** that holds the stale child's data:

```bash
netdatacli remove-stale-node <identifier>
```

Replace `<identifier>` with one of:
- `node_id` - The node's unique identifier
- `machine_guid` - The machine GUID from the node info
- `hostname` - The node's hostname

:::important

This command must be run on the **Parent Agent** that holds the node's metrics data, not on any arbitrary machine with Netdata Agent installed.

:::

:::note

If a node is represented by multiple Parent Agents in an HA setup, this command must be executed on **each** Parent Agent.

:::

**What happens next**: The command marks the node as ephemeral and removes it so it is no longer available for queries, from both the Netdata Agent dashboard and Netdata Cloud. The node is fully removed — it does not transition to Offline status.

</details>

<details>
<summary><strong>Removing All Child Nodes from a Parent</strong></summary><br/>

**When to use**: You need to remove all stale child nodes from a Parent Agent at once.

```bash
# Remove ALL stale child nodes from this Parent (use with extreme caution)
netdatacli remove-stale-node ALL_NODES
```

:::caution

This command affects all disconnected child nodes on the Parent Agent where it is run. Use with caution in production environments.

:::

</details>

## Prevention and Best Practices

### Auto-scaling/Spot Instances
For environments with auto-scaling cloud instances or spot instances that get terminated frequently, consider configuring nodes as ephemeral:

```ini
# In netdata.conf
[global]
is ephemeral node = yes
```

:::tip

For nodes that are part of streaming configurations, see [Nodes Ephemerality](/docs/nodes-ephemerality.md) for more advanced configuration options.

:::

### Prevent Automatic Reconnection
To prevent removed nodes from reappearing:
* Remove or clear any existing `claim.conf` file 
* Clear related environment variables on the node

## Troubleshooting

**"Delete is disabled"**: The node is Stale, not Offline. Use the CLI approach for child nodes.

**"Command not found"**: Ensure you're running `netdatacli` on a system with Netdata Agent installed.

**"Permission denied"**: You need **Admin** role in the Space to remove nodes.

**Node reappears after removal**: The agent may still be running and configured to reconnect. Stop the agent and clear claim configuration.

## Additional Resources

For a complete reference on node states and transitions, see [Node States and Transitions](/docs/netdata-cloud/node-states-and-transitions.md).

For more advanced configuration options with streaming setups, see [Nodes Ephemerality](/docs/nodes-ephemerality.md).

To avoid removal issues when cloning VMs, see [VM Templates](/docs/learn/vm-templates.md) for proper identity cleanup.

To understand how node identity affects removal and cleanup, see [Node Identities](/docs/learn/node-identities.md).
