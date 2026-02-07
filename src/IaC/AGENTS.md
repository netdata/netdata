# IaC Provisioning - Agent Guidelines

This document defines the rules and principles that **all** Netdata provisioning systems (Ansible, Terraform, Puppet, Chef, Salt) must follow.

## Core Principles

### 1. Installation Method
- All systems use `kickstart.sh` as the installation baseline
- The kickstart script auto-selects native packages when available
- Support both `stable` and `nightly` release channels
- Installation should be idempotent (re-running produces same result)

### 2. Cloud Claiming
- Claiming to Netdata Cloud is enabled by default
- Claiming is done by writing `claim.conf` directly and running `netdatacli reload-claiming-state`
- Do NOT use `netdata-claim.sh` script
- Support proxy configuration (`env`, `none`, or explicit URL)
- Support `insecure` option for environments with TLS inspection
- Disabling claim removes only `claim.conf`, preserves `/var/lib/netdata/cloud.d`

### 3. Profile System (File-Level Isolation)
- Profiles are **file-level bundles** of configuration
- Each profile defines a list of managed files
- A host can use multiple profiles **only if they touch different files**
- File collisions (same destination from multiple profiles) must be rejected
- Profiles do NOT merge configuration - they provide complete files

Standard profile types:
- `standalone` - Single agent, no streaming
- `child` - Streams to parent, may have local storage
- `child_minimal` - Streams to parent, minimal local resources
- `parent` - Receives streams from children

### 4. Managed Files
- Managed files have `src` (source path) and `dest` (destination relative to config dir)
- Files can be static or templates
- File types: `netdata_conf`, `stream_conf`, `health`, `plugin_conf`, `generic`
- Changes to managed files trigger Netdata restart
- No change = no restart (idempotent)
- Do NOT delete unmanaged files - only manage explicit files

### 5. Configuration Approach
- Full config file ownership (not partial patching)
- Users provide complete `netdata.conf`, `stream.conf`, etc.
- Optional ini-style tweaks can be applied after full files
- If providing full config files, leave per-setting variables empty

### 6. Standard Variables (Cross-Tool Consistency)

All provisioning tools must support these variable names:

**Installation:**
- `netdata_release_channel` - `stable` or `nightly`
- `netdata_install_only_if_missing` - Skip if already installed

**Claiming:**
- `netdata_claim_enabled` - Enable/disable claiming
- `netdata_claim_token` - Cloud claim token
- `netdata_claim_rooms` - Cloud room IDs
- `netdata_claim_url` - Cloud API URL
- `netdata_claim_proxy` - Proxy setting
- `netdata_claim_insecure` - Allow insecure TLS
- `netdata_reclaim` - Force reclaim if already claimed

**Database:**
- `netdata_db_mode` - `dbengine`, `ram`, or `none`
- `netdata_db_retention` - Retention in seconds
- `netdata_dbengine_multihost_disk_space` - Disk space in MB

**Streaming:**
- `netdata_stream_enabled` - Enable streaming
- `netdata_stream_destinations` - List of parent addresses
- `netdata_stream_api_key` - API key for streaming
- `netdata_stream_parent_keys` - Parent key definitions (for parents)

**Other:**
- `netdata_web_mode` - Web server mode
- `netdata_ml_enabled` - Machine learning on/off
- `netdata_health_enabled` - Alerts on/off
- `netdata_hostname` - Override hostname

### 7. Directory Structure

Each provisioning system follows this layout:
```
src/IaC/<system>/
  README.md              # End-user documentation
  AGENTS.md              # Technical details for AI agents
  roles/ or modules/     # Reusable components
  playbooks/ or manifests/
  inventories/example/   # Example configuration
    inventory.*          # Host definitions
    group_vars/          # Variable files
    files/               # Configuration files
      global/            # Files for all hosts
      profiles/          # Per-profile file bundles
  e2e/                   # End-to-end tests
    README.md
    run.sh
```

### 8. Testing Requirements

Each system must have E2E tests covering:
- Minimum OS matrix: Ubuntu + Debian + RHEL family
- Service starts and runs
- Claiming works (when enabled)
- Streaming connects (parent/child scenarios)
- Profile application works correctly

E2E environment: libvirt VMs + Docker containers

### 9. Documentation Requirements

Each system must document:
- Prerequisites (what users need installed)
- Quickstart (copy, configure, run)
- Variable reference (all supported variables)
- Profile system (how to define and use)
- Enterprise integration (AWX, Terraform Cloud, etc.)
- Validation steps (how to verify success)

## Implementation Order

Systems are implemented one at a time in this order:
1. Ansible (DONE)
2. Terraform
3. Puppet
4. Chef
5. Salt
