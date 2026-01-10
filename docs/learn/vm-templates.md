# VM Templates and Clones

When creating VM templates or cloning VMs, you need to reset the node identity so each clone gets a unique identity in Netdata Cloud.

## Understanding Node Identity

Netdata uses multiple identity files that work together:

### Machine GUID (Primary Node Identity)

The **machine GUID** is the unique identifier for this specific node. It is used by Netdata Cloud and Parents to uniquely identify this node.

- **Primary file**: `/var/lib/netdata/registry/netdata.public.unique.id`
- **Recovery backup**: `/var/lib/netdata/status-netdata.json` (and fallback locations)

**Generation behavior**:
1. On first start, Netdata tries to read the GUID from the primary file
2. If missing or invalid, falls back to the status file backup
3. If still missing, generates a new random UUID
4. The GUID is then saved to the primary file with a timestamp

The machine GUID **never changes** once created, unless manually deleted.

> **Note**: The SQLite databases (`netdata-meta.db`) store metadata about nodes this agent has seen (for Parents that receive data from Children), but they do **not** affect the agent's own identity. The agent's identity comes exclusively from the GUID file and status file backups.

### Claimed ID (ACLK Authentication)

The **claimed ID** is a separate random UUID generated during the claiming process. It authenticates the Agent-Cloud Link (ACLK) connection.

- **Primary storage**: `/var/lib/netdata/cloud.d/cloud.conf` (key: `claimed_id`)
- **Legacy file**: `/var/lib/netdata/cloud.d/claimed_id`
- **RSA keypair**: `/var/lib/netdata/cloud.d/private.pem` and `public.pem`

The claimed ID can be regenerated independently of the machine GUID by removing the `cloud.d` directory and re-claiming.

### Node ID (Cloud-Assigned)

The **node ID** is assigned by Netdata Cloud when the agent first connects. It links the machine GUID to your Cloud space.

- **Stored in**: `/var/cache/netdata/netdata-meta.db` (in `node_instance` table)

### Database Files (Metric Metadata)

The SQLite databases and dbengine files store metadata about nodes and their metrics:

- **Location**: `/var/cache/netdata/netdata-meta.db`, `/var/cache/netdata/context-meta.db`
- **dbengine data**: `/var/cache/netdata/dbengine/` (and per-node subdirectories)

These files do **not** determine the agent's own identity, but keeping them in templates causes problems:

1. **Profile switch**: The agent detects multiple node identities and assumes it's a Parent, switching to a parent runtime profile (larger caches, different behavior)
2. **Stale node reporting**: The agent reports all known nodes to Netdata Cloud, including the old template node
3. **Cloud confusion**: Netdata Cloud believes the template node has multiple Parents (all the clones)
4. **Persistent stale entries**: The template's identity appears as a stale node in Cloud until all clones rotate their databases due to retention settings (can take years with tiering)

### Status File (Recovery Backup)

The status file provides crash recovery and stores the machine GUID as a backup.

- **Primary location**: `/var/lib/netdata/status-netdata.json`
- **Fallback locations** (tried in order if primary fails):
  - `/var/cache/netdata/status-netdata.json`
  - `/tmp/status-netdata.json`
  - `/run/status-netdata.json`
  - `/var/run/status-netdata.json`

## Preparing a Template

To ensure each cloned VM gets a unique identity, you must remove **all** identity-related files.

> **Note**: The paths below assume default installation. If you customized `[directories]` in `netdata.conf`, adjust paths accordingly:
> - `lib` setting → affects `/var/lib/netdata/` paths
> - `cache` setting → affects `/var/cache/netdata/` paths

```bash
# Stop Netdata
sudo systemctl stop netdata

# Remove the machine GUID file
sudo rm -f /var/lib/netdata/registry/netdata.public.unique.id

# Remove ALL status file locations (recovery backups)
sudo rm -f /var/lib/netdata/status-netdata.json
sudo rm -f /var/cache/netdata/status-netdata.json
sudo rm -f /tmp/status-netdata.json
sudo rm -f /run/status-netdata.json
sudo rm -f /var/run/status-netdata.json

# Remove ACLK authentication (claimed_id, RSA keys, cloud config)
sudo rm -rf /var/lib/netdata/cloud.d/

# Remove databases and metric data (prevents stale node reporting to Cloud)
sudo rm -f /var/cache/netdata/netdata-meta.db
sudo rm -f /var/cache/netdata/netdata-meta.db-wal
sudo rm -f /var/cache/netdata/netdata-meta.db-shm
sudo rm -f /var/cache/netdata/context-meta.db
sudo rm -f /var/cache/netdata/context-meta.db-wal
sudo rm -f /var/cache/netdata/context-meta.db-shm
sudo rm -rf /var/cache/netdata/dbengine/

# DO NOT remove /etc/netdata/claim.conf - it enables auto-claiming under the new identity
# DO NOT start Netdata - the template must have no identity for clones to create new ones
```

After this, convert the VM to a template.

### Understanding Auto-Claiming

There are two ways to enable auto-claiming on first boot:

#### Option 1: claim.conf file (Recommended)

The kickstart script creates `/etc/netdata/claim.conf` when called with claiming parameters:

```ini
[global]
   url = https://app.netdata.cloud
   token = YOUR_SPACE_TOKEN
   rooms = ROOM_ID1,ROOM_ID2
```

**Keep this file** in your template.

#### Option 2: Environment variables

Set these environment variables in your VM's environment (e.g., via systemd override, `/etc/environment`, or cloud-init):

| Variable | Description | Required |
|----------|-------------|----------|
| `NETDATA_CLAIM_TOKEN` | Your Netdata Cloud Space token | Yes |
| `NETDATA_CLAIM_ROOMS` | Comma-separated Room IDs | No |
| `NETDATA_CLAIM_URL` | Cloud URL (default: `https://app.netdata.cloud`) | No |
| `NETDATA_CLAIM_PROXY` | Proxy server URL | No |

**Auto-claiming flow** (either method):
1. Clone boots → Netdata generates new machine GUID
2. Netdata reads `claim.conf` or environment variables → auto-claims to Cloud
3. Cloud assigns new node ID → node appears in your space

## Cloning from Template

When you clone a VM from the template:

1. **First boot**: Netdata generates a new unique machine GUID
2. **Auto-claims**: If `/etc/netdata/claim.conf` exists, claims to Cloud automatically
3. **Cloud registration**: Cloud assigns a new node ID, node appears in your space

## Troubleshooting Cloned VMs

If cloned VMs share the same identity in Cloud:

| Symptom | Cause | Solution |
|---------|-------|----------|
| Same node appears multiple times | Template had GUID stored | Reset GUID before templating |
| Clones can't connect to Parent | Duplicate machine GUID | Delete GUID file on affected clones, restart |
| Cloud shows unstable connections | Two agents with same GUID | One kicks the other offline repeatedly |
| Stale "template" node in Cloud | Database files kept in template | Wait for retention to expire, or manually delete databases on all clones |
| Clones using Parent profile | Database has multiple node identities | Delete `netdata-meta.db*` and `dbengine/` on clones |

**Common mistakes**:
- Forgot to remove `/var/lib/netdata/status-netdata.json` (GUID recovered from backup)
- Forgot to remove databases/dbengine (causes agent to report stale nodes to Cloud)
- Started Netdata before templating (regenerated the GUID)

## Quick Reference: Identity Files

| File | Contains | Must Remove | Why |
|------|----------|-------------|-----|
| `/var/lib/netdata/registry/netdata.public.unique.id` | Machine GUID | Yes | Primary identity file |
| `/var/lib/netdata/status-netdata.json` | Machine GUID backup | Yes | GUID recovered from here |
| `/var/cache/netdata/status-netdata.json` | Machine GUID backup | Yes | GUID recovered from here |
| `/tmp/status-netdata.json` | Machine GUID backup | Yes | GUID recovered from here |
| `/run/status-netdata.json` | Machine GUID backup | Yes | GUID recovered from here |
| `/var/run/status-netdata.json` | Machine GUID backup | Yes | GUID recovered from here |
| `/var/lib/netdata/cloud.d/` | Claimed ID, RSA keys | Yes | ACLK authentication |
| `/var/cache/netdata/netdata-meta.db*` | Node metadata | Yes | Prevents stale node reporting |
| `/var/cache/netdata/context-meta.db*` | Context metadata | Yes | Prevents stale node reporting |
| `/var/cache/netdata/dbengine/` | Metric data | Yes | Prevents stale node reporting |
| `/etc/netdata/claim.conf` | Claiming credentials | **No** | Enables auto-claiming |

## FAQ

### Proxmox

**Q: How do I prepare a template in Proxmox?**

A: Install Netdata on a VM, run the reset commands from "Preparing a Template" above, then right-click the VM and select "Convert to Template".

**Q: Can I use cloud-init with Netdata templates?**

A: Yes. Cloud-init runs on first boot, and Netdata generates a new machine GUID on its first start. Keep `/etc/netdata/claim.conf` in the template for auto-claiming.

### Vagrant

**Q: How to create a Vagrant box with Netdata?**

A: Install Netdata on the base VM, run the reset commands, then run `vagrant package --output my-box.box`. Users who run `vagrant up` will get a unique identity on first boot.

### libvirt/KVM

**Q: How to clone a KVM VM with Netdata?**

A: Use `virt-sysprep` to reset all identity files:

```bash
virt-sysprep -a myvm.qcow2 \
  --delete /var/lib/netdata/registry/netdata.public.unique.id \
  --delete /var/lib/netdata/status-netdata.json \
  --delete /var/cache/netdata/status-netdata.json \
  --delete '/var/cache/netdata/netdata-meta.db*' \
  --delete '/var/cache/netdata/context-meta.db*' \
  --delete /var/cache/netdata/dbengine/ \
  --delete /var/lib/netdata/cloud.d/
```

### Cloud Images (AWS, Azure, GCP)

**Q: How to create a custom AMI with Netdata?**

A: Launch an instance, install Netdata with claiming, run the reset commands (keeping `claim.conf`), create an AMI. Instances launched from your AMI will auto-claim with unique identities.

### Terraform

**Q: How to provision VMs with Netdata using Terraform?**

A: Use cloud-init to install Netdata on first boot:

```yaml
# cloud-init config
runcmd:
  - curl -fsSL https://get.netdata.cloud/kickstart.sh -o /tmp/kickstart.sh
  - bash /tmp/kickstart.sh --claim-token TOKEN --claim-rooms ROOM_ID
```

Netdata will generate a unique GUID and auto-claim on first boot.

### Containers

**Q: Do I need to reset identity for containers?**

A: No. Containers get a unique machine GUID on first start automatically because they start with empty volumes. Each new container instance has its own identity.

### General

**Q: What happens if I reboot a cloned VM?**

A: The machine GUID stays the same. Netdata only generates a new GUID when the primary file AND all backups are missing. Reboots preserve the identity.

**Q: Can I use the same claim token for multiple clones?**

A: Yes. Each clone generates its own machine GUID and claimed ID. They use the same token to authenticate but appear as separate nodes in Cloud.

**Q: What happens if two VMs share the same machine GUID?**

A: Both VMs will have the same identity and **cannot connect concurrently**:

1. **Parent connections**: Only one can stream at a time. The second fails to connect.
2. **Cloud connections**: The second VM kicks the first offline. Cloud detects the duplicate and disconnects the old client.

This causes unstable connections and confusion in Cloud. Always reset **all** identity files before templating.

**Q: What if I only deleted the GUID file but clones still share identity?**

A: The machine GUID was recovered from a backup location. Delete all files listed in "Quick Reference: Identity Files" above.

**Q: Why remove database files if they don't affect node identity?**

A: The databases store metadata about all nodes this agent has seen. If kept in a template:
- Each clone reports the old template node to Netdata Cloud
- Cloud sees the template node as having multiple Parents (all clones)
- The stale template entry persists until retention expires (possibly years)
- The agent may switch to a Parent runtime profile (larger caches, different behavior)

**Q: I already deployed clones with database files - how do I fix it?**

A: On each affected clone:
```bash
sudo systemctl stop netdata
sudo rm -f /var/cache/netdata/netdata-meta.db*
sudo rm -f /var/cache/netdata/context-meta.db*
sudo rm -rf /var/cache/netdata/dbengine/
sudo systemctl start netdata
```
The stale template node will eventually disappear from Cloud as clones rotate their data. Historical metrics on the clones will be lost.
