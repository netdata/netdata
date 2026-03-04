#!/usr/bin/env python3
"""
PR Comment Generator for Documentation Tester
Generates and posts GitHub PR comments with test results, including line-by-line feedback
"""

import sys
import re
import argparse
import subprocess
from pathlib import Path


def parse_report(report_file: str) -> dict:
    """Parse test report and extract detailed results"""
    content = Path(report_file).read_text()
    
    results = {
        'summary': {},
        'failed': [],
        'passed': [],
        'partial': [],
        'skipped': []
    }
    
    lines = content.split('\n')
    in_summary = False
    
    for line in lines:
        if line.startswith('## Summary'):
            in_summary = True
            continue
        if in_summary and line.startswith('##'):
            in_summary = False
        if in_summary:
            if '**Total' in line:
                results['summary']['total'] = re.search(r':\s*(\d+)', line).group(1) if re.search(r':\s*(\d+)', line) else '0'
            elif '**Passed' in line:
                results['summary']['passed'] = re.search(r':\s*(\d+)', line).group(1) if re.search(r':\s*(\d+)', line) else '0'
            elif '**Failed' in line:
                results['summary']['failed'] = re.search(r':\s*(\d+)', line).group(1) if re.search(r':\s*(\d+)', line) else '0'
            elif '**Partial' in line:
                results['summary']['partial'] = re.search(r':\s*(\d+)', line).group(1) if re.search(r':\s*(\d+)', line) else '0'
    
    current_claim = None
    current_status = None
    current_error = None
    current_line = None
    
    for i, line in enumerate(lines):
        if line.startswith('### '):
            if 'FAIL' in line:
                match = re.search(r'Line\s+(\d+)', line)
                if match:
                    current_line = int(match.group(1))
                current_status = 'fail'
            elif '✅ PASS' in line:
                current_status = 'pass'
            elif '⚠️ PARTIAL' in line:
                current_status = 'partial'
            elif '⏭️ SKIP' in line:
                current_status = 'skip'
        
        elif '**Error**:' in line:
            current_error = line.split('**Error**:')[1].strip()
        
        elif line.startswith('---') and current_status:
            if current_status == 'fail' and current_error:
                results['failed'].append({
                    'line': current_line,
                    'error': current_error
                })
            current_status = None
            current_error = None
            current_line = None
    
    return results


def generate_line_comment(claim: dict, doc_file: str) -> str:
    """Generate detailed line comment for a failed test"""
    error = claim.get('error', 'Unknown error')
    
    comment = f"""❌ **THIS DIDN'T REPRODUCE THE DOCUMENTED EXPECTED OUTCOME**

**Error**: {error}

The documentation claims this should work, but the live test failed."""

    return comment


def generate_pr_comment(report_file: str, pr_number: str) -> str:
    """Generate main PR comment from test report"""
    report_content = Path(report_file).read_text()
    results = parse_report(report_file)
    
    passed = int(results['summary'].get('passed', '0'))
    failed = int(results['summary'].get('failed', '0'))
    partial = int(results['summary'].get('partial', '0'))
    skipped = int(results['summary'].get('skipped', '0'))
    total = passed + failed + partial + skipped
    
    status_emoji = "✅" if failed == 0 else "❌"
    
    comment = f"""## 🤖 Documentation Test Results {status_emoji}

I've tested the claims in this documentation update against a live Netdata installation.

### Summary
| Status | Count |
|--------|-------|
| ✅ Passed | {passed} |
| ❌ Failed | {failed} |
| ⚠️ Partial | {partial} |
| ⏭️ Skipped | {skipped} |
| **Total** | **{total}** |
"""
    
    if failed > 0:
        comment += f"\n**⚠️ {failed} test(s) failed to reproduce documented behavior**\n"
        comment += "See inline comments for details on each failure.\n"
    
    if partial > 0:
        comment += f"\n**ℹ️ {partial} test(s) require manual verification**\n"
    
    comment += f"""
---
*Automated testing by Netdata Documentation Tester*
"""

    return comment, results


def post_line_comment(pr_number: str, comment: str, path: str = None, line: int = None):
    """Post a line-specific comment to GitHub PR"""
    try:
        cmd = ['gh', 'pr', 'review', pr_number, '--body', comment]
        
        if path and line:
            cmd.extend(['--path', str(path), '--line', str(line)])
        
        result = subprocess.run(cmd, capture_output=True, text=True)
        
        if result.returncode == 0:
            print(f"✅ Line comment posted (line {line})")
        else:
            print(f"⚠️ Line comment failed: {result.stderr}")
            
    except Exception as e:
        print(f"❌ Failed to post line comment: {e}")


def post_pr_comment(pr_number: str, comment: str):
    """Post general comment to GitHub PR"""
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
    parser.add_argument('--doc-file', help='Path to documentation file (for line comments)')
    parser.add_argument('--line-comments', action='store_true',
                        help='Post inline comments for each failure')
    parser.add_argument('--dry-run', action='store_true',
                        help='Generate comment without posting')

    args = parser.parse_args()

    if not Path(args.report_file).exists():
        print(f"Error: Report file not found: {args.report_file}")
        sys.exit(1)

    comment, results = generate_pr_comment(args.report_file, args.pr_number)

    if args.dry_run:
        print("=" * 60)
        print("DRY RUN - Comment that would be posted:")
        print("=" * 60)
        print()
        print(comment)
        print()
        
        if args.line_comments:
            print("Line comments that would be posted:")
            for fail in results['failed']:
                line_comment = generate_line_comment(fail, args.doc_file)
                print(f"  Line {fail['line']}: {fail['error'][:50]}...")
        
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

        # Post main comment
        post_pr_comment(args.pr_number, comment)
        
        # Post line comments if requested
        if args.line_comments and args.doc_file:
            for fail in results['failed']:
                if fail.get('line'):
                    line_comment = generate_line_comment(fail, args.doc_file)
                    post_line_comment(
                        args.pr_number, 
                        line_comment, 
                        args.doc_file, 
                        fail['line']
                    )


if __name__ == '__main__':
    main()
