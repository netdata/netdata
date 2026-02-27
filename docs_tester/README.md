# Netdata Documentation Tester

General-purpose documentation testing agent for validating Netdata documentation claims against live installations.

## Environment Setup

Set the following environment variables before running the tester:

```bash
export TEST_VM_HOST="<vm-host-ip>"       # Test VM IP address
export TEST_VM_USER="<vm-user>"           # SSH username (must have sudo access)
export TEST_VM_PASSWORD="<vm-password>"   # SSH password
export NETDATA_URL="http://<vm-host-ip>:19999"  # Netdata URL
```

### Example

```bash
export TEST_VM_HOST="10.10.30.140"
export TEST_VM_USER="cm"
export TEST_VM_PASSWORD="123"
export NETDATA_URL="http://10.10.30.140:19999"
```

**IMPORTANT**: VPN Required for accessing test VMs.

## Quick Start

```bash
# Test a documentation file
python3 docs_tester/tester.py path/to/documentation.md
```

## What It Tests

1. **Configuration Examples** - Validates YAML/CONF blocks
2. **Command Examples** - Executes BASH/SH commands on VM
3. **API Endpoints** - Tests API accessibility and responses
4. **Multi-Step Workflows** - Extracts and validates step-by-step procedures

## Files

- `tester.py` - Main testing engine with workflow support
- `pr_commenter.py` - PR commenting utility
- `run_tester.py` - CLI wrapper
