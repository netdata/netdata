# PR Commenting - How It Works

## Overview

The documentation tester can automatically post test results to GitHub PRs using the `gh` CLI.

## How It Works

### 1. Test Documentation
Run tests on a documentation file:

```bash
python3 netdata_tester/docs_tester/tester.py docs/your-file.md
```

This generates a report like: `test_results/your-file_20260226_135110.md`

### 2. Generate PR Comment
The PR commenter reads the test report and generates a formatted comment:

```bash
python3 netdata_tester/docs_tester/pr_commenter.py \
    test_results/your-file_TIMESTAMP.md \
    12345
```

### 3. Post Comment (or Dry Run)
```bash
# Preview without posting (recommended first)
python3 netdata_tester/docs_tester/pr_commenter.py \
    test_results/report.md \
    12345 --dry-run

# Actually post to PR
python3 netdata_tester/docs_tester/pr_commenter.py \
    test_results/report.md \
    12345
```

## Complete Workflow

### Option 1: Automated Workflow Script

Use the complete workflow script that handles everything:

```bash
# Test and comment on PR 21806
./netdata_tester/pr_workflow.sh 21806

# Test a specific documentation file
./netdata_tester/pr_workflow.sh 21806 docs/netdata-conf.md
```

The workflow script:
1. ✅ Fetches PR information
2. ✅ Finds modified documentation files
3. ✅ Runs documentation tests
4. ✅ Generates test report
5. ✅ Previews PR comment
6. ✅ Posts comment to PR (with confirmation)

### Option 2: Manual Workflow

```bash
# Step 1: Run tests
python3 netdata_tester/docs_tester/tester.py docs/reporting.md

# Output: Report saved to: test_results/reporting_20260226_135110.md

# Step 2: Preview comment
python3 netdata_tester/docs_tester/pr_commenter.py \
    test_results/reporting_20260226_135110.md \
    21806 --dry-run

# Step 3: Post comment
python3 netdata_tester/docs_tester/pr_commenter.py \
    test_results/reporting_20260226_135110.md \
    21806
```

## Comment Format

The generated PR comment looks like this:

```markdown
## 🤖 Documentation Test Results

I've tested the claims in this documentation update against a live Netdata installation.

### Summary
- **Total Claims**: 5
- ✅ **Passed**: 3 ✅
- ❌ **Failed**: 1 ❌
- ⚠️ **Partial**: 1 ⚠️

### Test Details

<details>
<summary>Click to view full test report</summary>

# Documentation Test Report

**Document**: docs/your-file.md
**Test Date**: 2026-02-26 13:51:10
**Test Environment**: Netdata at http://10.10.30.140:19999

[...full report...]

</details>

---
*Automated testing by Netdata Documentation Tester*
```

## Requirements

### GitHub CLI (gh)
Must be installed and authenticated:

```bash
# Check authentication
gh auth status

# If not authenticated
gh auth login
```

### Permissions
The GitHub token needs:
- `repo` scope (to post comments)
- Already verified ✅

## Examples

### Example 1: All Tests Pass

```bash
# Run tests
python3 netdata_tester/docs_tester/tester.py docs/nodes-ephemerality.md

# Post to PR 21822
python3 netdata_tester/docs_tester/pr_commenter.py \
    test_results/nodes-ephemerality_20260226_140000.md \
    21822
```

Result on PR:
```
## 🤖 Documentation Test Results

### Summary
- ✅ **Passed**: 5
- ❌ **Failed**: 0
```

### Example 2: Some Tests Fail

```bash
# Run tests
python3 netdata_tester/docs_tester/tester.py docs/config-file.md

# Post to PR 21804
python3 netdata_tester/docs_tester/pr_commenter.py \
    test_results/config-file_20260226_140000.md \
    21804
```

Result on PR:
```
## 🤖 Documentation Test Results

⚠️ 2 test(s) failed - Please review and fix

### Summary
- ✅ **Passed**: 3
- ❌ **Failed**: 2
```

## Troubleshooting

### Error: GitHub CLI not authenticated

```
Error: GitHub CLI (gh) not authenticated or not found
```

**Solution:**
```bash
gh auth login
```

### Error: Permission denied

```
Error: Failed to post comment: HTTP 403
```

**Solution:**
- Check gh token has `repo` permissions
- Verify you have write access to the repository

### Error: PR not found

```
Error: Pull request not found
```

**Solution:**
- Verify PR number is correct
- Check you're in the correct repository
- Ensure PR exists and is accessible

## Best Practices

1. **Always dry-run first**
   ```bash
   --dry-run
   ```
   Review the comment before posting

2. **Test locally before posting**
   Make sure tests run successfully locally

3. **Use descriptive report filenames**
   The report filename is included in the PR comment

4. **Keep comments concise**
   The detailed report is hidden in a collapsible section

5. **Handle failures gracefully**
   Failed tests are clearly highlighted in the summary

## Automation Ideas

### Cron Job for PR Monitoring
```bash
#!/bin/bash
# Check for new PRs every hour
gh pr list --state open --json number,title | \
  jq -r '.[] | select(.title | contains("docs")) | .number' | \
  while read pr; do
    ./netdata_tester/pr_workflow.sh $pr
  done
```

### GitHub Action Integration
Create a workflow that:
1. Triggers on PR updates to docs/
2. Runs documentation tests
3. Posts comments with results

### Webhook Integration
Set up a webhook that:
1. Listens for PR events
2. Runs tests on modified files
3. Posts comments automatically

## Summary

✅ **Yes, it can post comments on PRs!**

The workflow:
1. Test documentation → Generate report
2. Read report → Generate PR comment
3. Use `gh` CLI → Post comment to PR

All components are working and ready to use.
