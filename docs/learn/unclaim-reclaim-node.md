# Unclaiming and Reclaiming a Node

:::note

**What's the difference between unclaiming/reclaiming and removing a node?**

| Action                | What it does                                          | Agent status                            |
| --------------------- | ----------------------------------------------------- | --------------------------------------- |
| **Unclaim & Reclaim** | Disconnect from current Space, connect to a new Space | Agent keeps running                     |
| **Remove**            | Permanently delete node from Netdata Cloud            | Agent may keep running but disconnected |

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

<details>
<summary><strong>Linux-based Installations</strong></summary><br/>

Delete the `cloud.d/` directory in your Netdata library directory.

```bash
cd /var/lib/netdata   # Replace with your Netdata library directory, if not /var/lib/netdata/
sudo rm -rf cloud.d/
```

:::warning

**Restart the agent after removing cloud.d/**

If you don't restart the agent after removing `/var/lib/netdata/cloud.d/`, the node will remain connected to Netdata Cloud until the agent restarts.

To apply the unclaiming change immediately, run:

```bash
sudo systemctl restart netdata
```

:::

</details>

<details>
<summary><strong>Docker-based Installations</strong></summary><br/>

To remove a node from your Space and connect it to another, follow these steps:

1. **Enter the running container** you wish to remove from your Space

   ```bash
   docker exec -it CONTAINER_NAME sh
   ```

   Replace `CONTAINER_NAME` with either the container's name or ID.

2. **Delete the connection files and machine GUID**

   ```bash
   rm -rf /var/lib/netdata/cloud.d/
   rm /var/lib/netdata/registry/netdata.public.unique.id
   ```

   :::important

   Docker unclaiming requires deleting **both** `cloud.d/` **and** `netdata.public.unique.id`. The Linux procedure only deletes `cloud.d/`, but Docker-based installations must also remove the machine GUID to ensure a clean reconnection to the new Space.

   :::

3. **Stop and remove the container**

   **Docker CLI:**

   ```bash
   docker stop CONTAINER_NAME
   docker rm CONTAINER_NAME
   ```

   Replace `CONTAINER_NAME` with either the container's name or ID.

   **Docker Compose:**

   Inside the directory that has the `docker-compose.yml` file, run:

   ```bash
   docker compose down
   ```

   **Docker Swarm:**

   Run the following, and replace `STACK` with your Stack's name:

   ```bash
   docker stack rm STACK
   ```

</details>

## Step 2: Reclaim to New Space

### Option 1: Quick Reclaim (Recommended)

Run the standard claim command with your new Space's token:

```bash
bash <(curl -Ss https://get.netdata.cloud/kickstart.sh) --claim-token YOUR_NEW_TOKEN --claim-rooms YOUR_ROOMS --claim-url https://app.netdata.cloud
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

### Option 3: Environment Variables (Docker)

For Docker-based installations, you must recreate the container with the new Space's claim token. Go to your new Space in Netdata Cloud, copy the installation command with the new claim token, and recreate the container.

When using `docker run`, include the claiming environment variables:

```bash
docker run -d \
  -e NETDATA_CLAIM_TOKEN=YOUR_NEW_TOKEN \
  -e NETDATA_CLAIM_ROOMS=ROOM_KEY1,ROOM_KEY2 \
  ...other options... \
  netdata/netdata
```

When using `docker-compose.yml`, update the environment section with the new Space's claim token:

```yaml
environment:
  NETDATA_CLAIM_TOKEN: YOUR_NEW_TOKEN
  NETDATA_CLAIM_ROOMS: ROOM_KEY1,ROOM_KEY2
```

Then start the container with `docker compose up -d`. The node should appear online in the new Space.

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
- [Connect Agent to Cloud](/src/claim/README.md#connect-agent-to-cloud) - Initial connection setup
- [Reconnect Agent](/src/claim/README.md#reconnect-agent) - Linux/ Docker-based installations
