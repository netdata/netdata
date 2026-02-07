# Deploy Netdata with Ansible

Install and configure Netdata agents across your infrastructure using Ansible.

## What You Can Do

- Install Netdata on multiple servers in one run
- Connect agents to Netdata Cloud automatically
- Set up streaming (parent/child) architectures
- Deploy different configurations to different server roles
- Manage custom alert rules and plugin settings

## Prerequisites

- Ansible installed on your control machine
- SSH access to target servers
- Sudo privileges on target servers
- (Optional) Netdata Cloud claim token from https://app.netdata.cloud

## Quick Start

### Step 1: Copy the Example

```bash
# Copy the example inventory to your own
cp -r inventories/example inventories/myinfra
```

### Step 2: Define Your Servers

Edit `inventories/myinfra/inventory.yml`:

```yaml
all:
  hosts:
    web-01:
      ansible_host: 192.168.1.10
      netdata_profiles: [standalone]
    web-02:
      ansible_host: 192.168.1.11
      netdata_profiles: [standalone]
    db-01:
      ansible_host: 192.168.1.20
      netdata_profiles: [standalone]
```

### Step 3: Configure Claiming (Optional)

Edit `inventories/myinfra/group_vars/all.yml`:

```yaml
# Get these from Netdata Cloud -> Space settings -> Nodes
netdata_claim_token: "YOUR_TOKEN_HERE"
netdata_claim_rooms: "YOUR_ROOM_ID"

# To skip Cloud claiming entirely:
# netdata_claim_enabled: false
```

### Step 4: Run

```bash
ansible-playbook -i inventories/myinfra/inventory.yml playbooks/netdata.yml
```

## Common Scenarios

### Standalone Agents

Every server runs independently with full local storage.

```yaml
# inventory.yml
all:
  hosts:
    server-01:
      ansible_host: 10.0.0.1
      netdata_profiles: [standalone]
    server-02:
      ansible_host: 10.0.0.2
      netdata_profiles: [standalone]
```

### Parent-Child Streaming

Children stream metrics to a central parent. Useful for:
- Centralized dashboards
- Reduced storage on edge nodes
- Aggregating metrics from many servers

```yaml
# inventory.yml
all:
  hosts:
    # This server receives metrics from children
    metrics-parent:
      ansible_host: 10.0.0.100
      netdata_profiles: [parent]

    # These servers stream to the parent
    app-01:
      ansible_host: 10.0.0.1
      netdata_profiles: [child]
    app-02:
      ansible_host: 10.0.0.2
      netdata_profiles: [child]
```

You'll need to configure the streaming keys - see the profile files in `files/profiles/`.

### Minimal Children (Edge/IoT)

For resource-constrained nodes that only stream, with no local storage:

```yaml
netdata_profiles: [child_minimal]
```

## Using Profiles

Profiles are bundles of configuration files for different server roles.

**Built-in profiles:**
| Profile | Use Case |
|---------|----------|
| `standalone` | Independent agent, full features |
| `parent` | Receives streams from children |
| `child` | Streams to parent, has local storage |
| `child_minimal` | Streams to parent, minimal resources |

**Assign profiles per host:**
```yaml
server-01:
  netdata_profiles: [child]
```

**Create your own profiles** by adding files to `files/profiles/yourprofile/` and defining them in `group_vars/all.yml`.

## Adding Custom Configuration Files

Deploy your own Netdata configuration files:

```yaml
# group_vars/all.yml
netdata_managed_files:
  # Custom alert rules for all servers
  - src: files/global/health.d/my-alerts.conf
    dest: health.d/my-alerts.conf

  # Custom plugin config
  - src: files/global/go.d/httpcheck.conf
    dest: go.d/httpcheck.conf
```

Place your files in `inventories/myinfra/files/global/`.

## Using with AWX / Automation Controller

The role works standalone - import it into your existing playbooks:

```yaml
# your-playbook.yml
- hosts: netdata_servers
  roles:
    - netdata
```

Store secrets (claim tokens) in your controller's credential store or Ansible Vault.

## Verify Installation

After running the playbook:

```bash
# Check service is running
ssh server-01 'systemctl status netdata'

# Check claiming (if enabled)
ssh server-01 'cat /var/lib/netdata/cloud.d/claimed_id'

# View the dashboard
open http://server-01:19999
```

If you claimed to Netdata Cloud, your nodes should appear in your Space within a minute.

## Variables Reference

### Claiming
| Variable | Default | Description |
|----------|---------|-------------|
| `netdata_claim_enabled` | `true` | Connect to Netdata Cloud |
| `netdata_claim_token` | - | Your claim token (required if claiming) |
| `netdata_claim_rooms` | - | Room ID(s) to join |
| `netdata_claim_url` | `https://app.netdata.cloud` | Cloud URL |
| `netdata_claim_proxy` | - | Proxy: `env`, `none`, or URL |

### Installation
| Variable | Default | Description |
|----------|---------|-------------|
| `netdata_release_channel` | `stable` | `stable` or `nightly` |
| `netdata_install_only_if_missing` | `true` | Skip if already installed |

### Database
| Variable | Default | Description |
|----------|---------|-------------|
| `netdata_db_mode` | - | `dbengine`, `ram`, or `none` |
| `netdata_db_retention` | - | Retention in seconds |
| `netdata_dbengine_multihost_disk_space` | - | Disk space in MB |

### Features
| Variable | Default | Description |
|----------|---------|-------------|
| `netdata_ml_enabled` | - | Machine learning on/off |
| `netdata_health_enabled` | - | Alerts on/off |
| `netdata_web_mode` | - | Web server mode |
| `netdata_hostname` | - | Override hostname |

### Streaming (Child)
| Variable | Default | Description |
|----------|---------|-------------|
| `netdata_stream_enabled` | - | Enable streaming |
| `netdata_stream_destinations` | `[]` | Parent addresses |
| `netdata_stream_api_key` | - | API key for authentication |

### Streaming (Parent)
| Variable | Default | Description |
|----------|---------|-------------|
| `netdata_stream_parent_keys` | `[]` | Allowed child keys |

## Troubleshooting

**Playbook fails with "claim token required"**
- Set `netdata_claim_token` and `netdata_claim_rooms`, or
- Set `netdata_claim_enabled: false` to skip claiming

**Nodes not appearing in Cloud**
- Check the agent can reach `app.netdata.cloud` (port 443)
- If behind a proxy, set `netdata_claim_proxy`

**Streaming not working**
- Ensure parent has `[web].mode = static-threaded` (the parent profile sets this)
- Verify API keys match between parent and child configs
- Check firewall allows port 19999 between nodes
