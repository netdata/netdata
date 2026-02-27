# Netdata Documentation Tester

General-purpose documentation testing agent for validating Netdata documentation claims against live installations.

## Configuration

The tester reads configuration from environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `VM_HOST` | Test VM hostname/IP | (required) |
| `VM_USER` | SSH username | (required) |
| `VM_PASSWORD` | SSH password | (required) |
| `NETDATA_URL` | Netdata agent URL | http://VM_HOST:19999 |
| `OUTPUT_DIR` | Test results directory | test_results |

### Setting Up Credentials

```bash
# Set environment variables
export VM_HOST="your-vm-host"
export VM_USER="your-username"
export VM_PASSWORD="your-password"
export NETDATA_URL="http://your-vm-host:19999"
```

## Test Environment Access

**IMPORTANT: VPN Required**

The tester requires access to a Linux VM (Debian 12 recommended) for validating documentation claims.

### Connection Requirements

- SSH access with sudo privileges
- Netdata agent installed and running
- VPN access if VM is on internal network

### VPN Instructions

1. Connect to the corporate VPN
2. Wait for connection to establish
3. Verify connectivity: `ping -c 3 $VM_HOST`
4. Proceed with testing

## Quick Start

```bash
# Set credentials
export VM_HOST="10.10.30.140"
export VM_USER="cm"
export VM_PASSWORD="123"

# Test a documentation file
python3 docs_tester/main.py path/to/documentation.md
```

## What It Tests

1. **Configuration Examples** - Validates YAML/CONF blocks
2. **Command Examples** - Executes BASH/SH commands on VM
3. **API Endpoints** - Tests API accessibility and responses
4. **Multi-Step Workflows** - Extracts and validates step-by-step procedures

## Files

- `main.py` - CLI entry point
- `run_tester.py` - Convenience wrapper
- `pr_commenter.py` - PR commenting utility
- `tester/` - Core testing modules
