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

This guide covers two scenarios:

1. **Unclaiming** - Disconnect a node from your current Space (keeps agent running)
2. **Reclaiming** - Connect an unclaimed node to a new Space

Use this process when you need to move a node from one Space to another without disrupting the agent's operation.

## Prerequisites

- The node must have Netdata Agent installed
- You need the claiming token and room keys for the **new** Space
- Optionally: access to the node via SSH or terminal

## Unclaim a Node from Current Space

### Linux-based Installations

1. SSH into the node or access its terminal

2. Remove the Cloud connection directory:

   ```bash
   sudo rm -rf /var/lib/netdata/cloud.d/
   ```

3. Verify the node no longer appears in your Space

### Docker-based Installations

1. Enter the running container:

   ```bash
   docker exec -it CONTAINER_NAME sh
   ```

2. Remove the connection files:

   ```bash
   rm -rf /var/lib/netdata/cloud.d/
   rm /var/lib/netdata/registry/netdata.public.unique.id
   ```

3. Exit the container and recreate it with the new claim token

## Reclaim to a New Space

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
- Review agent logs: `grep -i CLAIM /var/log/netdata/error.log`

**Reconnection fails:**
- Run `netdatacli aclk-state` to check connection status
- Ensure the agent has internet access to `app.netdata.cloud`
- Check for firewall blocking port 443

## Related Documentation

- [Remove a node from Netdata Cloud entirely](/docs/learn/remove-node.md) - For permanent node removal
- [Connect Agent to Cloud](/src/claim/README.md) - Initial connection setup