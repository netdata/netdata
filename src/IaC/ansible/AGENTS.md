# Ansible Implementation - Agent Technical Reference

This document provides technical details for AI agents working on the Ansible provisioning system.

## Architecture

```
src/IaC/ansible/
  playbooks/netdata.yml      # Main playbook (imports the role)
  roles/netdata/             # Reusable role
    defaults/main.yml        # Default variables
    handlers/main.yml        # Service handlers
    tasks/                   # Task files
    templates/               # Jinja2 templates
  inventories/example/       # Example inventory
    inventory.yml            # Host definitions
    group_vars/              # Variable files per group
    files/                   # Configuration files
      global/                # Applied to all hosts
      profiles/              # Per-profile file bundles
  e2e/                       # End-to-end tests
```

## Task Execution Order

The role executes tasks in this order (see `roles/netdata/tasks/main.yml`):

1. **install.yml** - Install Netdata via kickstart.sh
2. **config-dir.yml** - Detect/set config directory path
3. **profiles.yml** - Resolve profile definitions to file lists
4. **managed-files.yml** - Deploy managed configuration files
5. **configure.yml** - Apply ini-style configuration tweaks
6. **stream.yml** - Configure streaming (child and parent)
7. **service.yml** - Ensure service is running
8. **claim.yml** - Handle Cloud claiming

## Key Variables

### Installation
```yaml
netdata_kickstart_url: "https://get.netdata.cloud/kickstart.sh"
netdata_release_channel: "stable"  # or "nightly"
netdata_install_only_if_missing: true
netdata_kickstart_extra_args: []
```

### Claiming
```yaml
netdata_claim_enabled: true
netdata_claim_token: ""      # Required when enabled
netdata_claim_rooms: ""      # Required when enabled
netdata_claim_url: "https://app.netdata.cloud"
netdata_claim_proxy: ""      # "env", "none", or URL
netdata_claim_insecure: false
netdata_reclaim: false
```

### Paths
```yaml
netdata_config_dir: ""       # Auto-detected if empty
netdata_lib_dir: "/var/lib/netdata"
netdata_group: "netdata"
```

### Database
```yaml
netdata_db_mode: ""          # "dbengine", "ram", "none"
netdata_db_retention: ""     # Seconds
netdata_db_update_every: ""  # Seconds
netdata_dbengine_multihost_disk_space: ""  # MB
```

### Streaming (Child)
```yaml
netdata_stream_enabled: ""   # true/false
netdata_stream_destinations: []  # ["parent1:19999", "parent2:19999"]
netdata_stream_api_key: ""
netdata_stream_section_options: {}  # Extra [stream] options
```

### Streaming (Parent)
```yaml
netdata_stream_parent_keys: []
# Each entry:
#   - key: "API_KEY_STRING"
#     allow_from: "*"  # Optional
#     options: {}      # Optional extra settings
```

### Profiles
```yaml
netdata_profiles: []  # Per-host list: [profile1, profile2]
netdata_profiles_definitions: {}
# Structure:
#   profile_name:
#     managed_files:
#       - src: files/profiles/X/netdata.conf
#         dest: netdata.conf
#         type: netdata_conf
```

### Managed Files
```yaml
netdata_managed_files: []
# Each entry:
#   - src: path/to/source
#     dest: relative/to/config/dir
#     type: netdata_conf|stream_conf|health|plugin_conf|generic
#     template: false  # Optional, default false
#     owner: root      # Optional
#     group: netdata   # Optional
#     mode: "0644"     # Optional
```

## Profile Resolution Logic

File: `roles/netdata/tasks/profiles.yml`

1. Start with empty file list
2. For each profile in `netdata_profiles`:
   - Look up definition in `netdata_profiles_definitions`
   - Add `managed_files` from the profile
3. Check for destination collisions (same `dest` from multiple sources)
4. Fail if collision detected
5. Merge with host-level `netdata_managed_files`
6. Result stored in `netdata_managed_files_effective`

## Managed Files Deployment

File: `roles/netdata/tasks/managed-files.yml`

1. For each file in `netdata_managed_files_effective`:
   - If `template: true`: use `template` module
   - Otherwise: use `copy` module
2. Source path resolution:
   - Relative paths resolved from inventory directory
   - Absolute paths used as-is
3. Destination path resolution:
   - Relative paths prefixed with `netdata_config_dir`
   - Absolute paths used as-is
4. Register changes for restart decision

## Claiming Implementation

File: `roles/netdata/tasks/claim.yml`

When `netdata_claim_enabled: true`:
1. Write `claim.conf` from template (`templates/claim.conf.j2`)
2. Check if already claimed (`/var/lib/netdata/cloud.d/claimed_id`)
3. If not claimed or `netdata_reclaim: true`:
   - Run `netdatacli reload-claiming-state`

When `netdata_claim_enabled: false`:
1. Remove `claim.conf` if it exists
2. Do NOT remove `/var/lib/netdata/cloud.d/` (preserves claim state)

## Handler Triggers

File: `roles/netdata/handlers/main.yml`

- `Restart Netdata` - Triggered by:
  - Managed file changes
  - Configuration changes via `ini_file`
  - Service state changes

- `Reload Netdata health` - Triggered by:
  - Health config file changes (type: health)

## Testing

E2E tests use:
- **libvirt VMs**: Ubuntu 22.04, Rocky 9 (UEFI boot required)
- **Docker**: Debian 12 systemd container

Test flow:
1. Provision VMs/containers
2. Generate test inventory with profiles
3. Run playbook
4. Validate: service status, claiming, streaming

Secrets stored in: `src/IaC/.netdata-iac-claim.env` (not committed)

## Common Modifications

### Adding a New Profile

1. Create files in `inventories/example/files/profiles/<profile_name>/`
2. Add definition to `group_vars/all.yml`:
   ```yaml
   netdata_profiles_definitions:
     new_profile:
       managed_files:
         - src: files/profiles/new_profile/netdata.conf
           dest: netdata.conf
           type: netdata_conf
   ```
3. Assign to hosts in inventory: `netdata_profiles: [new_profile]`

### Adding Global Files

Add to `netdata_managed_files` in `group_vars/all.yml`:
```yaml
netdata_managed_files:
  - src: files/global/health.d/custom.conf
    dest: health.d/custom.conf
    type: health
```

### Supporting New OS

1. Update `install.yml` if package manager handling needed
2. Add to E2E test matrix in `e2e/run.sh`
3. Test kickstart.sh compatibility
