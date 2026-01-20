# Unclaiming and Reclaiming a Node

:::note

**What's the difference between unclaiming/reclaiming and removing a node?**

| Action | What it does | Agent status |
|--------|--------------|--------------|
| **Unclaim & Reclaim** | Disconnect from current Space, connect to a new Space | Agent keeps running |
| **Remove** | Permanently delete node from Netdata Cloud | Agent may keep running but disconnected |

If you want to **move a node to a different Space**, use the **unclaim and reclaim** process on this page.

If you want to **permanently remove a node** from Netdata Cloud entirely, see our guide: [Removing a node from your Netdata Cloud Space](/docs/learn/remove-node.md)

:::

This guide covers how to move a node from one Space to another without removing it from Netdata Cloud entirely.

## Why Move a Node Between Spaces?

You might need to move a node to a different Space when:
- Reorganizing your infrastructure monitoring structure
- Transferring node ownership between teams or departments
- Consolidating multiple Spaces into one

## Prerequisites

- The node must have Netdata Agent installed
- You need the claiming token and room keys for the **new** Space
- Access to the node via SSH or terminal

:::info

**Need a claim token?** See **[Regenerate Claiming Token](/src/claim/README.md#regenerate-claiming-token)** to generate a new token for your Space (Space Administrator required).

:::

## Step 1: Unclaim from Current Space

See our **[Reconnect Agent](/docs/netdata-cloud/connect-agent.md#reconnect-agent)** guide for the exact commands to:
- Remove the Cloud connection directory (Linux)
- Remove connection files and recreate container (Docker)

:::warning

**Restart the agent after removing cloud.d/**

If you don't restart the agent after removing `/var/lib/netdata/cloud.d/`, the node will remain connected to Netdata Cloud until the agent restarts.

To apply the unclaiming change immediately, run:

```bash
sudo systemctl restart netdata
```

:::

## Step 2: Reclaim to New Space

### Option 1: Quick Reclaim (Recommended)

Run the standard claim command with your new Space's token:

```bash
bash <(curl -Ss https://my-netdata.io/kickstart.sh) --claim-token YOUR_NEW_TOKEN --claim-rooms YOUR_ROOMS --claim-url https://app.netdata.cloud
```

### Option 2: Configuration File

Create `/INSTALL_PREFIX/etc/netdata/claim.conf` with:

```bash
[global]
   url = https://app.netdata.cloud
   token = NETDATA_CLOUD_SPACE_TOKEN
   rooms = ROOM_KEY1,ROOM_KEY2
```

Then restart the agent or run:

```bash
netdatacli reload-claiming-state
```

### Option 3: Environment Variables

For Docker/container deployments, set:

```bash
NETDATA_CLAIM_TOKEN=YOUR_NEW_TOKEN
NETDATA_CLAIM_ROOMS=ROOM_KEY1,ROOM_KEY2
```

## Verification

After reclaiming, verify the node appears in:

- Your new Space in Netdata Cloud
- The Rooms you specified
- The node status shows as "Online"

## Troubleshooting

**Node doesn't appear in new Space:**
- Verify the claim token is correct for the new Space
- Check `/var/lib/netdata/cloud.d/` was removed before reclaiming
- Review agent logs using:
   - `journalctl --namespace netdata -b 0 | grep -i CLAIM`
- or:
   - `grep -i CLAIM /var/log/netdata/daemon.log`

**Reconnection fails:**
- Check connection status: `curl http://localhost:19999/api/v1/aclk`
- Ensure the agent has internet access to `app.netdata.cloud`
- Check for firewall blocking port 443

## Related Documentation

- [Remove a node from Netdata Cloud entirely](/docs/learn/remove-node.md) - For permanent node removal
- [Connect Agent to Cloud](/src/claim/README.md) - Initial connection setup
- [Reconnect Agent](/docs/netdata-cloud/connect-agent#reconnect-agent) - Linux/Docker based installations
