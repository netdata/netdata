# Netdata Documentation Tester

Automated testing agent that validates Netdata documentation claims against live installations.

## Overview

The Documentation Tester:
- Parses documentation files to extract testable claims
- Implements configurations on live systems
- Executes commands and verifies outputs
- Tests API endpoints
- Validates UI/visual claims
- Generates detailed test reports
- Comments on GitHub PRs with results

## Requirements

- Python 3.8+
- sshpass (for SSH authentication)
- requests (Python library)

### Install Dependencies

```bash
pip install -r requirements.txt
```

On macOS:
```bash
brew install sshpass
```

On Debian/Ubuntu:
```bash
apt-get install sshpass
```

## Configuration

Edit the configuration in `run_tester.py` or use command-line arguments:

```python
config = {
    'vm_host': '10.10.30.140',      # Test VM IP
    'vm_user': 'cm',                  # SSH user
    'vm_password': '123',             # SSH password
    'netdata_url': 'http://10.10.30.140:19999',  # Netdata URL
    'output_dir': 'test_results'      # Output directory
}
```

## Usage

### Basic Usage

```bash
python run_tester.py <documentation_file>
```

### Examples

```bash
# Test a specific documentation file
python run_tester.py ./docs/netdata-conf.md

# Test with custom VM settings
python run_tester.py ./docs/collectors/example.md \
    --vm-host 192.168.1.100 \
    --vm-user testuser

# Test with custom output directory
python run_tester.py ./docs/alarms.md \
    --output-dir /tmp/test_results
```

### Using the Standalone Script

```bash
python tester.py docs/netdata-conf.md \
    --vm-host 10.10.30.140 \
    --vm-user cm \
    --vm-password 123 \
    --netdata-url http://10.10.30.140:19999
```

## What Gets Tested

### 1. Configuration Claims

Configuration blocks in code fences (```yaml, ```conf, ```ini):

**Example in documentation:**
```yaml
[global]
  update every = 1
```

**What the tester does:**
- Backs up original configuration
- Applies the documented configuration
- Restarts Netdata service
- Verifies service is running
- Restores original configuration
- Reports PASS/FAIL

### 2. Command Claims

Bash commands in code fences (```bash, ```sh):

**Example in documentation:**
```bash
netdatacli reload-health
```

**What the tester does:**
- Executes the command on the test VM
- Captures stdout, stderr, and exit code
- Compares against expected behavior
- Reports PASS/FAIL

### 3. API Endpoint Claims

API examples or HTTP requests:

**Example in documentation:**
```bash
curl http://localhost:19999/api/v1/info
```

**What the tester does:**
- Extracts the URL from the command
- Queries the API endpoint
- Verifies response code (200 = PASS)
- Reports PASS/FAIL

### 4. Behavioral Claims

Text statements with "will", "should", "must":

**Example in documentation:**
> This configuration will enable metric X

**What the tester does:**
- Flags as PARTIAL (requires manual verification)
- Includes in report with note

## Test Results

### PASS ✅
- Configuration loads without errors
- Service remains running
- Commands execute successfully
- API endpoints return 200
- Behavior matches documentation

### FAIL ❌
- Configuration rejected/invalid
- Service crashes or fails to restart
- Commands fail with non-zero exit
- API returns error codes
- Behavior differs from documentation

### PARTIAL ⚠️
- Works but with minor discrepancies
- Requires undocumented steps
- Manual verification required (behavioral claims)

## Report Format

Reports are generated in Markdown format:

```markdown
# Documentation Test Report

**Document**: docs/netdata-conf.md
**Test Date**: 2026-02-26 13:00:00
**Test Environment**: Netdata at http://10.10.30.140:19999

## Summary
- **Total Claims**: 5
- **Passed**: 3 ✅
- **Failed**: 1 ❌
- **Partial**: 1 ⚠️

## Test Results

### ✅ PASS: Configuration (Line 45-60)
**Claim**: Configuration example
**What was tested**: Applied config and restarted service
**Result**: PASS

---

### ❌ FAIL: Command (Line 120)
**Claim**: Command example
**What was tested**: Executed command
**Error**: Command failed with exit code 1
**Output**: ...
```

## Integration with GitHub PRs

To comment on PRs with test results:

```bash
# Get PR number and documentation changes
gh pr view <PR_NUMBER> --json files --jq '.files[].path'

# Run tests on modified docs
python run_tester.py docs/modified-file.md

# Post comment with results
gh pr comment <PR_NUMBER> --body-file test_results/report.md
```

## Safety Features

### Backup Before Testing
- Configuration files are backed up before modification
- Backups are timestamped for easy restoration
- Original state is always restored after testing

### Timeout Protection
- Commands timeout after 60 seconds
- API requests timeout after 10 seconds
- Failed tests don't block subsequent tests

### Isolation
- Each test runs independently
- State is restored between tests
- No persistent changes (unless intentional)

## Troubleshooting

### SSH Connection Issues
```
Permission denied, please try again
```
- Verify VM is reachable
- Check SSH credentials
- Ensure password authentication is enabled on VM

### Configuration Tests Failing
```
Failed to apply config
```
- Check configuration syntax
- Verify file paths exist on VM
- Review stderr for specific errors

### Service Won't Restart
```
Failed to restart netdata
```
- Check Netdata logs: `journalctl -u netdata`
- Verify configuration is valid
- Check for resource constraints

### API Tests Failing
```
API returned status code 404
```
- Verify Netdata is running
- Check API endpoint URL
- Confirm API version matches documentation

## Advanced Usage

### Testing Multiple Files

```bash
for file in docs/collectors/*.md; do
    python run_tester.py "$file"
done
```

### Generating JSON Reports

```bash
python tester.py docs/netdata-conf.md --format json
```

### Custom Timeout Settings

Edit `tester.py`:
```python
response = requests.get(url, timeout=20)  # Increase from 10s
```

## Development

### Adding New Test Types

1. Add claim type to `parse_documentation()`
2. Implement `test_<type>()` method
3. Update `test_claim()` to route to new method
4. Add appropriate report formatting

### Extending Report Format

Modify `generate_report()` to include additional fields or formatting.

## Contributing

When adding new test capabilities:
1. Add tests for new functionality
2. Update this README with examples
3. Document any new configuration options
4. Ensure backward compatibility

## License

Same as Netdata project.
