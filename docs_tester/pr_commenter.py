#!/usr/bin/env python3
"""
PR Comment Generator for Documentation Tester
Generates and posts GitHub PR comments with test results
"""

import sys
import argparse
import subprocess
from pathlib import Path


def generate_pr_comment(report_file: str, pr_number: str) -> str:
    """Generate PR comment from test report"""

    report_content = Path(report_file).read_text()

    lines = report_content.split('\n')
    summary = {}
    in_summary = False

    for line in lines:
        if line.startswith('## Summary'):
            in_summary = True
            continue
        if in_summary and line.startswith('##'):
            break
        if in_summary:
            if '**Total Claims**' in line:
                summary['total'] = line.split(':')[1].strip()
            elif '**Passed**' in line:
                summary['passed'] = line.split(':')[1].strip()
            elif '**Failed**' in line:
                summary['failed'] = line.split(':')[1].strip()
            elif '**Partial**' in line:
                summary['partial'] = line.split(':')[1].strip()

    failed_count = int(summary.get('failed', '0').split()[0])
    partial_count = int(summary.get('partial', '0').split()[0])

    comment = f"""## 🤖 Documentation Test Results

I've tested the claims in this documentation update against a live Netdata installation.

### Summary
- **Total Claims**: {summary.get('total', 'N/A')}
- ✅ **Passed**: {summary.get('passed', '0')}
- ❌ **Failed**: {summary.get('failed', '0')}
- ⚠️ **Partial**: {summary.get('partial', '0')}
"""

    if failed_count > 0:
        comment += f"\n**⚠️ {failed_count} test(s) failed** - Please review and fix\n"

    if partial_count > 0:
        comment += f"\n**ℹ️ {partial_count} test(s) require manual verification**\n"

    comment += f"""
### Test Details

<details>
<summary>Click to view full test report</summary>

{report_content}

</details>

---
*Automated testing by Netdata Documentation Tester*
"""

    return comment


def post_pr_comment(pr_number: str, comment: str):
    """Post comment to GitHub PR using gh CLI"""
    try:
        import tempfile
        with tempfile.NamedTemporaryFile(mode='w', suffix='.md', delete=False) as f:
            f.write(comment)
            temp_path = f.name

        try:
            cmd = ['gh', 'pr', 'comment', pr_number, '--body-file', temp_path]
            subprocess.run(cmd, check=True)

            print(f"✅ Comment posted to PR #{pr_number}")
            print(f"   View: https://github.com/netdata/netdata/pull/{pr_number}")
        finally:
            Path(temp_path).unlink()

    except subprocess.CalledProcessError as e:
        print(f"❌ Failed to post comment: {e}")
        sys.exit(1)


def main():
    parser = argparse.ArgumentParser(
        description='Post documentation test results to GitHub PR'
    )
    parser.add_argument('report_file', help='Path to test report file')
    parser.add_argument('pr_number', help='PR number to comment on')
    parser.add_argument('--dry-run', action='store_true',
                        help='Generate comment without posting')

    args = parser.parse_args()

    if not Path(args.report_file).exists():
        print(f"Error: Report file not found: {args.report_file}")
        sys.exit(1)

    comment = generate_pr_comment(args.report_file, args.pr_number)

    if args.dry_run:
        print("=" * 60)
        print("DRY RUN - Comment that would be posted:")
        print("=" * 60)
        print()
        print(comment)
        print()
        print("=" * 60)
    else:
        # Only check auth when actually posting
        try:
            subprocess.run(['gh', 'auth', 'status'],
                          capture_output=True, check=True)
        except (subprocess.CalledProcessError, FileNotFoundError):
            print("Error: GitHub CLI (gh) not authenticated or not found")
            print("Run: gh auth login")
            sys.exit(1)

        post_pr_comment(args.pr_number, comment)


if __name__ == '__main__':
    main()
