# Removing a node from your Netdata Cloud Space

You can remove a node from your Space in Netdata Cloud, but the process depends on the node's current state and requires different approaches for different scenarios.

## Understanding Node States

Before attempting to remove a node, it's important to understand what each status means:

* **Online**: Node is actively connected and streaming data
* **Stale**: Node is a child that stopped streaming to its parent, but the parent still has its historical data
* **Offline**: Node is disconnected and no longer has data available for querying

<details>
<summary><strong>Why Can't I Delete Stale Nodes?</strong></summary><br/>

**Stale means this node is a Child that has stopped streaming to its Parent, but the Parent still retains its historical data.**

You can't delete a Stale node because the Parent is alive/connected to the Cloud and has data for the Stale node. The UI disables the "Remove" option to protect this historical data and maintain the parent-child relationship integrity.<br/>

This is why stale nodes show "Delete is disabled" - the system prevents deletion while the parent node still holds queryable metrics data for that child.<br/>
</details>


## Quick Decision Guide

**What's your node status?**

```
🔴 Node shows "Offline" → Use Method 1 (UI Method)
🟡 Node shows "Stale" → Use Method 2 (CLI Method) ← MOST COMMON ISSUE  
📦 Multiple nodes to remove → Use Method 3 (Bulk Operations)
```

:::note

You need **Admin** role in your Space to remove nodes. The CLI method requires a system with Netdata Agent installed.

:::

## Removal Methods

<details>
<summary><strong>Method 1: Removing Offline Nodes (UI Method)</strong></summary><br/>

**When to use**: Your node shows as **Offline** status in Netdata Cloud.

**Steps**:
1. Stop the Netdata Agent on the node you want to remove
2. In Netdata Cloud, go to **Space Settings > Nodes**
3. Locate the offline node in the list
4. Select the trash icon to remove it

:::note

The **Remove** option is only available in the **Space Settings** view. It will appear disabled in the "All Nodes" room or other parts of the UI.

:::

</details>

<details>
<summary><strong>Method 2: Removing Stale Nodes (CLI Method)</strong></summary><br/>

**When to use**: Your node shows as **Stale** status and UI shows "Delete is disabled".

**Step 1: Get the Node UUID**
1. In Netdata Cloud, navigate to the stale node
2. Click the **node information (i)** button 
3. Click **"View node info in JSON"**
4. Copy the UUID from the JSON data (it will be copied to your clipboard)

**Step 2: Remove the Stale Node**
Run this command on any node with Netdata Agent installed:

```bash
netdatacli remove-stale-node <UUID>
```

Replace `<UUID>` with the node's actual identifier from Step 1.

**What happens next**: The command unregisters and removes the node from the cloud. The node status should change from **Stale → Offline** in Netdata Cloud, then you can remove it via the UI method if needed.

</details>

<details>
<summary><strong>Method 3: Bulk Operations</strong></summary><br/>

**When to use**: You need to remove multiple nodes at once.

You can use the remove-stale-node command with different identifiers:

```bash
# Using machine GUID
netdatacli remove-stale-node <machine_guid>

# Using hostname  
netdatacli remove-stale-node <hostname>

# Remove ALL stale nodes (use with extreme caution)
netdatacli remove-stale-node ALL_NODES
```

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

For nodes that are part of streaming configurations, see the [Nodes Ephemerality documentation](https://learn.netdata.cloud/docs/observability-centralization-points/nodes-ephemerality) for more advanced configuration options.

:::

### Prevent Automatic Reconnection
To prevent removed nodes from reappearing:
* Remove or clear any existing `claim.conf` file 
* Clear related environment variables on the node

## Troubleshooting

**"Delete is disabled"**: The node is Stale, not Offline. Use Method 2 (CLI approach).

**"Command not found"**: Ensure you're running `netdatacli` on a system with Netdata Agent installed.

**"Permission denied"**: You need **Admin** role in the Space to remove nodes.

**Node reappears after removal**: The agent may still be running and configured to reconnect. Stop the agent and clear claim configuration.

## Additional Resources

For more advanced configuration options with streaming setups, see the [Nodes Ephemerality documentation](https://learn.netdata.cloud/docs/observability-centralization-points/nodes-ephemerality).
