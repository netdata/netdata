# Quick Start Guide - Documentation Tester Agent

## Installation

No installation required - all files are in `/Users/kanela/src/netdata/ai-agent/netdata_tester/`

## Quick Test

```bash
# Test a documentation file
python3 netdata_tester/docs_tester/tester.py docs/reporting.md

# Or use the wrapper
./netdata_tester/run_docs_tester.sh docs/reporting.md
```

## Output

- Reports saved to `test_results/` directory
- Format: `{filename}_{timestamp}.md`
- Includes pass/fail status, steps, and errors

## Examples

### Test a single file
```bash
python3 netdata_tester/docs_tester/tester.py docs/nodes-ephemerality.md
```

### Test with custom VM
```bash
python3 netdata_tester/docs_tester/tester.py docs/file.md \
    --vm-host 192.168.1.100 \
    --vm-user admin \
    --vm-password secret
```

### Post to PR
```bash
# Run test
python3 netdata_tester/docs_tester/tester.py docs/file.md

# Get the report file (look at output)
# Post to PR
python3 netdata_tester/docs_tester/pr_commenter.py \
    test_results/file_TIMESTAMP.md \
    12345
```

## What Gets Tested

- **Configurations**: YAML/CONF code blocks
- **Commands**: BASH/SH code blocks
- **API Endpoints**: HTTP/curl commands
- **Behavioral Claims**: Text with "will/should/must"

## Status Codes

- ✅ **PASS**: Claim tested successfully
- ❌ **FAIL**: Claim failed
- ⚠️ **PARTIAL**: Works but requires manual verification

## Troubleshooting

**SSH Connection Failed:**
- Verify VPN is connected
- Check VM is reachable: `ping 10.10.30.140`
- Verify credentials: `ssh cm@10.10.30.140`

**Command Failed:**
- Check report for specific error
- Verify command works manually on VM
- Check for missing dependencies on VM

**API Tests Failed:**
- Verify Netdata is running: `curl http://10.10.30.140:19999/api/v1/info`
- Check API endpoint URL
- Verify API version compatibility

## Full Documentation

See `netdata_tester/docs_tester/README.md` for comprehensive documentation.

## Support

For issues or questions, refer to the implementation guide at `netdata_tester/IMPLEMENTATION_SUMMARY.md`
