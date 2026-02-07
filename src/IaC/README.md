# Netdata Infrastructure as Code (IaC)

Deploy and configure Netdata agents at scale using your preferred configuration management tool.

## Supported Tools

| Tool | Status | Directory |
|------|--------|-----------|
| Ansible | Ready | [ansible/](ansible/) |
| Terraform | Planned | - |
| Puppet | Planned | - |
| Chef | Planned | - |
| Salt | Planned | - |

## Key Concepts

### Profiles

Profiles let you define different Netdata configurations for different roles in your infrastructure.

**Common profiles:**
- **standalone** - Single agent, full local storage, no streaming
- **parent** - Receives metrics from children, larger retention
- **child** - Streams metrics to parent, moderate local storage
- **child_minimal** - Streams metrics to parent, minimal local resources

Each profile is a bundle of configuration files. You assign profiles to hosts, and the provisioning tool deploys the right files.

```
Host: web-server-01 -> Profile: child
Host: web-server-02 -> Profile: child
Host: metrics-parent -> Profile: parent
```

### File-Based Configuration

Configuration is managed at the **file level**, not by patching individual settings:
- You provide complete configuration files (`netdata.conf`, `stream.conf`, etc.)
- The tool deploys your files to the right locations
- Changes trigger a Netdata restart automatically

This approach is simpler and more predictable than merging partial configurations.

### Cloud Claiming

Netdata Cloud claiming is supported and enabled by default:
- Provide your claim token and room ID
- The tool writes `claim.conf` and reloads the claiming state
- Proxy support included for restricted networks

To disable claiming, set `netdata_claim_enabled: false`.

## Quick Start

1. **Choose your tool** - Pick the provisioning system you already use
2. **Copy the example** - Each tool has an example inventory/configuration
3. **Set your values** - Claim token, profiles, hosts
4. **Run** - Apply the configuration to your infrastructure

See the README in each tool's directory for specific instructions.

## Directory Structure

```
src/IaC/
  README.md           # This file
  AGENTS.md           # Internal guidelines for AI agents
  ansible/            # Ansible role and playbooks
  terraform/          # (planned)
  puppet/             # (planned)
  chef/               # (planned)
  salt/               # (planned)
```

## What Gets Configured

The IaC tools can manage:
- Netdata installation (via kickstart.sh)
- Cloud claiming
- Streaming (parent/child relationships)
- Database retention settings
- Web server mode
- Machine learning on/off
- Health alerts on/off
- Custom configuration files
- Plugin configurations

## Requirements

- Target nodes accessible via SSH (or equivalent for your tool)
- Sudo/root access on target nodes
- Internet access to download Netdata (or local mirror)
- Claim token from Netdata Cloud (if claiming)

## Security Notes

- Store claim tokens securely (Ansible Vault, Terraform secrets, etc.)
- The tools do not delete unmanaged files
- Configuration files may contain sensitive data - set appropriate permissions
