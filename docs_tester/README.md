# Netdata Documentation Tester

General-purpose documentation testing agent for validating Netdata documentation claims against live installations.

## Test Environment Access

**IMPORTANT: VPN Required**

The tester requires access to a Debian 12 VM for validating documentation claims.

### Connection Details

- **Host**: 10.10.30.140
- **OS**: Linux (Debian 12)
- **User**: cm
- **Password**: 123
- **Access**: This user has root access via sudo

### VPN Instructions

You must be connected to the corporate VPN before running the tester.

1. Connect to the corporate VPN
2. Wait for connection to establish
3. Verify connectivity: `ping -c 3 10.10.30.140`
4. Proceed with testing

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
