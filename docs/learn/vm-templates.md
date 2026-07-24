# VM Templates and Clones

:::danger

**Destructive Operations - Data Loss Warning**

The commands in this guide **permanently delete**:
- All historical metrics
- [Node identity](/docs/learn/node-identities.md#agent-self-identity)
- [Cloud connection](/docs/learn/node-identities.md#agent-cloud-link-aclk-identity)
- Alert history

**This is irreversible. There is no undo.**

Only run these commands on VMs you intend to convert to templates.
Running these on a production system will destroy your monitoring data.

:::

:::tip

**What You'll Learn**

How to prepare a VM template so each clone gets a unique Netdata identity and automatically connects to Netdata Cloud.

:::

## Prerequisites

- **Read first**: [Node Identities](/docs/learn/node-identities.md) - understand what you're deleting
- Netdata installed on a VM
- Hypervisor that supports templates or golden images
- (Optional) `/etc/netdata/claim.conf` configured for auto-claiming to Cloud

## Overview

To prepare a VM template:

1. **Stop Netdata** - Prevent file regeneration
2. **Delete identity and data files** - Force new identity on clone boot
3. **Keep claim.conf** - Enable auto-claiming (optional)
4. **Convert to template** - Without starting Netdata

## Node Types: Ephemeral vs Permanent

VMs cloned from templates can be configured as **ephemeral** (no alerts on disconnect, auto-cleanup after 24h — auto-scaling instances, spot VMs, short-lived workloads) or **permanent** (alerts trigger on disconnect, no automatic cleanup — the node stays visible until manually removed or its metrics fully rotate out via retention — long-running production systems).

To make cloned VMs ephemeral by default, add to `/etc/netdata/netdata.conf` **in the template**:

```ini
[global]
is ephemeral node = yes
```

See [Node Ephemerality](/docs/nodes-ephemerality.md) for full documentation, cleanup rules, and alerting details.

## Files to Delete

:::danger

**Verify you are on the correct VM before running these commands.**

:::

| Category | Files | What's Lost |
|----------|-------|-------------|
| **[Agent Identity](/docs/learn/node-identities.md#agent-self-identity)** | [GUID file](/docs/learn/node-identities.md#agent-self-identity), [status backups](/docs/learn/node-identities.md#status-file-backups) | Node identity |
| **[ACLK Auth](/docs/learn/node-identities.md#agent-cloud-link-aclk-identity)** | [`cloud.d/`](/docs/learn/node-identities.md#agent-cloud-link-aclk-identity) directory | Cloud connection, must re-claim |
| **[Node Metadata](/docs/learn/node-identities.md#parent-children-identities)** | `netdata-meta.db*`, `context-meta.db*` | Node metadata, metric mappings |
| **Metrics** | `dbengine*` directories (all tiers) | All historical metrics |

**Keep**: `/etc/netdata/claim.conf` - enables auto-claiming on clones

## Step-by-Step

### 1. Stop Netdata

#### Linux

```bash
sudo systemctl stop netdata
```

#### Windows (PowerShell)

```powershell
Stop-Service Netdata
```

Verify the service stopped with `Get-Service Netdata`. See [Service Control](/docs/netdata-agent/start-stop-restart.md) for details.

### 2. Delete All Identity and Data Files

:::danger

**Point of No Return**

The following commands permanently delete Netdata data. Verify you are on the template VM.

:::

#### Linux

```bash
# Machine GUID (Agent Self Identity)
sudo rm -f /var/lib/netdata/registry/netdata.public.unique.id

# Status file backups (GUID recovery locations)
sudo rm -f /var/lib/netdata/status-netdata.json
sudo rm -f /var/cache/netdata/status-netdata.json
sudo rm -f /tmp/status-netdata.json
sudo rm -f /run/status-netdata.json
sudo rm -f /var/run/status-netdata.json

# ACLK authentication (Claimed ID, RSA keys)
sudo rm -rf /var/lib/netdata/cloud.d/

# Databases and metrics (metadata, all dbengine tiers)
sudo rm -f /var/cache/netdata/netdata-meta.db*
sudo rm -f /var/cache/netdata/context-meta.db*
sudo rm -rf /var/cache/netdata/dbengine*
```

#### Windows (PowerShell)

Run in an elevated (Administrator) PowerShell session — paths assume the default Windows install location (`C:\Program Files\Netdata`). Deleting a file you don't have permission for fails silently under `-ErrorAction SilentlyContinue`, so a non-elevated session can leave identity files in place with no error shown.

```powershell
# Machine GUID (Agent Self Identity)
Remove-Item "C:\Program Files\Netdata\var\lib\netdata\registry\netdata.public.unique.id" -Force -ErrorAction SilentlyContinue

# Status file backups (GUID recovery locations)
Remove-Item "C:\Program Files\Netdata\var\lib\netdata\status-netdata.json" -Force -ErrorAction SilentlyContinue
Remove-Item "C:\Program Files\Netdata\var\cache\netdata\status-netdata.json" -Force -ErrorAction SilentlyContinue

# ACLK authentication (Claimed ID, RSA keys)
Remove-Item "C:\Program Files\Netdata\var\lib\netdata\cloud.d\*" -Recurse -Force -ErrorAction SilentlyContinue

# Databases and metrics (metadata, all dbengine tiers)
Remove-Item "C:\Program Files\Netdata\var\cache\netdata\netdata-meta.db*" -Force -ErrorAction SilentlyContinue
Remove-Item "C:\Program Files\Netdata\var\cache\netdata\context-meta.db*" -Force -ErrorAction SilentlyContinue
Remove-Item "C:\Program Files\Netdata\var\cache\netdata\dbengine*" -Recurse -Force -ErrorAction SilentlyContinue
```

### 3. Configure Auto-Claiming (Optional)

To have clones automatically claim to Netdata Cloud on first boot, ensure `claim.conf` exists.

#### Linux

```bash
cat /etc/netdata/claim.conf
```

#### Windows (PowerShell)

```powershell
Get-Content "C:\Program Files\Netdata\etc\netdata\claim.conf"
```

Should contain:
```ini
[global]
   url = https://app.netdata.cloud
   token = YOUR_SPACE_TOKEN
   rooms = ROOM_ID
```

### 4. Convert to Template

**Do not start Netdata.** Convert the VM to a template using your hypervisor.

## When Clones Boot

1. Netdata starts, no [GUID](/docs/learn/node-identities.md#agent-self-identity) found, generates new unique identity
2. If `claim.conf` exists, auto-claims to Cloud
3. Cloud assigns [Node ID](/docs/learn/node-identities.md#cloud-node-identity), new node appears in your Space

Each clone is a unique, independent node.

## Hypervisor Notes

The Netdata cleanup commands are the same for all hypervisors. The difference is **when** and **how** to run them.

| Hypervisor | Template Support | When to Clean | Automation |
|------------|------------------|---------------|------------|
| **Proxmox** | Convert to Template | Before conversion | cloud-init scripts |
| **VMware/vSphere** | VM Templates | Before conversion | Guest customization |
| **Hyper-V** | Checkpoints/Templates | Before checkpoint/export | PowerShell scripts |
| **libvirt/KVM** | virt-sysprep | During sysprep | `--delete` flags |
| **AWS** | AMI | Before image creation | user-data scripts |
| **Azure** | Managed Image | Before capture | cloud-init |
| **GCP** | Machine Image | Before creation | startup scripts |
| **Vagrant** | Box packaging | Before `vagrant package` | Vagrantfile provisioner |

<details>
<summary><strong>libvirt/KVM: virt-sysprep example</strong></summary>

```bash
virt-sysprep -a myvm.qcow2 \
  --delete /var/lib/netdata/registry/netdata.public.unique.id \
  --delete /var/lib/netdata/status-netdata.json \
  --delete /var/cache/netdata/status-netdata.json \
  --delete /tmp/status-netdata.json \
  --delete /run/status-netdata.json \
  --delete /var/run/status-netdata.json \
  --delete /var/lib/netdata/cloud.d \
  --delete '/var/cache/netdata/netdata-meta.db*' \
  --delete '/var/cache/netdata/context-meta.db*' \
  --delete '/var/cache/netdata/dbengine*'
```

</details>

<details>
<summary><strong>Cloud-init: Fresh install approach</strong></summary>

Alternative: Install Netdata on first boot instead of templating:

```yaml
# cloud-init user-data
runcmd:
  - curl -fsSL https://get.netdata.cloud/kickstart.sh -o /tmp/kickstart.sh
  - bash /tmp/kickstart.sh --claim-token TOKEN --claim-rooms ROOM_ID
```

Each instance installs fresh with unique identity.

</details>

## Troubleshooting

### Clones share the same identity

Cause: [GUID recovered from status backup](/docs/learn/node-identities.md#status-file-backups). Netdata checks multiple backup locations before generating a new GUID.

Solution: Delete **all** status file locations, not just the primary GUID file. See the cleanup commands in [Step 2](#2-delete-all-identity-and-data-files). The same fix applies on Windows — run the equivalent PowerShell commands in [Fixing Already-Deployed Clones](#fixing-already-deployed-clones).

### Clones don't connect to Parent

Cause: Either clones share the same [Machine GUID](/docs/learn/node-identities.md#agent-self-identity) (only one can connect at a time), or `stream.conf` wasn't configured in the template.

Solution:
- Verify each clone has a unique GUID: `cat /var/lib/netdata/registry/netdata.public.unique.id` (Linux) or `Get-Content "C:\Program Files\Netdata\var\lib\netdata\registry\netdata.public.unique.id"` (Windows PowerShell)
- Verify `stream.conf` exists and has the correct Parent destination and API key
- If GUIDs are duplicated, run the cleanup on each clone (loses metrics)

### Stale "template" node appears in Cloud

Cause: [Database files kept](/docs/learn/node-identities.md#multiple-node-identities-in-database) from the template. The template's node identity persists in the metadata.

Solution: Delete databases on all clones. This loses historical metrics but removes the stale node reference.

### Clones using Parent profile unexpectedly

Cause: Template had `stream.conf` with an enabled API key section (configured to receive streams, as Parent).

Solution: Reset `stream.conf` on clones or delete the API key sections that enable receiving.

### Unstable Cloud connections (flapping)

Cause: Two agents have the same [Machine GUID](/docs/learn/node-identities.md#agent-self-identity). Cloud kicks the older connection offline when the second connects.

Solution: Each agent needs a unique GUID. Run the cleanup procedure on affected clones.

### Clone doesn't auto-claim to Cloud

Cause: Missing `claim.conf` or environment variables not set.

Solution: Create `/etc/netdata/claim.conf` with your Space token.

### Fixing Already-Deployed Clones

If clones were deployed with identity files, run the cleanup on each affected clone.

#### Linux

```bash
# On each affected clone
sudo systemctl stop netdata

# Machine GUID
sudo rm -f /var/lib/netdata/registry/netdata.public.unique.id

# Status file backups (all locations)
sudo rm -f /var/lib/netdata/status-netdata.json
sudo rm -f /var/cache/netdata/status-netdata.json
sudo rm -f /tmp/status-netdata.json
sudo rm -f /run/status-netdata.json
sudo rm -f /var/run/status-netdata.json

# ACLK authentication (if re-claiming to Cloud)
sudo rm -rf /var/lib/netdata/cloud.d/

# Databases and metrics
sudo rm -f /var/cache/netdata/netdata-meta.db*
sudo rm -f /var/cache/netdata/context-meta.db*
sudo rm -rf /var/cache/netdata/dbengine*

sudo systemctl start netdata
```

#### Windows (PowerShell)

Run the following in an elevated PowerShell session on each affected clone.

```powershell
# On each affected clone
Stop-Service Netdata

# Machine GUID
Remove-Item "C:\Program Files\Netdata\var\lib\netdata\registry\netdata.public.unique.id" -Force -ErrorAction SilentlyContinue

# Status file backups (all locations)
Remove-Item "C:\Program Files\Netdata\var\lib\netdata\status-netdata.json" -Force -ErrorAction SilentlyContinue
Remove-Item "C:\Program Files\Netdata\var\cache\netdata\status-netdata.json" -Force -ErrorAction SilentlyContinue

# ACLK authentication (if re-claiming to Cloud)
Remove-Item "C:\Program Files\Netdata\var\lib\netdata\cloud.d\*" -Recurse -Force -ErrorAction SilentlyContinue

# Databases and metrics
Remove-Item "C:\Program Files\Netdata\var\cache\netdata\netdata-meta.db*" -Force -ErrorAction SilentlyContinue
Remove-Item "C:\Program Files\Netdata\var\cache\netdata\context-meta.db*" -Force -ErrorAction SilentlyContinue
Remove-Item "C:\Program Files\Netdata\var\cache\netdata\dbengine*" -Recurse -Force -ErrorAction SilentlyContinue

Start-Service Netdata
```

:::warning

This deletes all historical metrics on the clone. If you skip deleting `cloud.d/`, you must re-claim to Cloud manually.

:::

## FAQ

<details>
<summary>What if I reboot a clone?</summary>

Identity persists. Netdata only generates a new [GUID](/docs/learn/node-identities.md#agent-self-identity) when the file AND all [backups](/docs/learn/node-identities.md#status-file-backups) are missing.

</details>

<details>
<summary>Can multiple clones use the same claim token?</summary>

Yes. Each clone gets a unique [Machine GUID](/docs/learn/node-identities.md#agent-self-identity) and [Claimed ID](/docs/learn/node-identities.md#agent-cloud-link-aclk-identity). They authenticate with the same token but appear as separate nodes.

</details>

<details>
<summary>Do containers need this?</summary>

No. Containers start with empty volumes, so each gets a unique identity automatically.

</details>

<details>
<summary>Is my claim token secure in the template?</summary>

The token only allows claiming to your Space. It cannot read data or modify other nodes. Treat it like an API key - don't expose publicly, but it's safe in private templates.

</details>

