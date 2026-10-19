#!/usr/bin/env python3
"""
Netdata Documentation Tester - CLI Wrapper
Convenient wrapper for running documentation tests
"""

import sys
from pathlib import Path

from tester import SSHClient, ClaimExtractor, Executor, Reporter
from config import VM_HOST, VM_USER, VM_PASSWORD, NETDATA_URL, OUTPUT_DIR


def main():
    config = {
        'vm_host': VM_HOST,
        'vm_user': VM_USER,
        'vm_password': VM_PASSWORD,
        'netdata_url': NETDATA_URL,
        'output_dir': OUTPUT_DIR
    }

    if len(sys.argv) < 2:
        print("Usage: python run_tester.py <documentation_file> [options]")
        print()
        print("Example:")
        print("  python run_tester.py ./docs/netdata-conf.md")
        print()
        sys.exit(1)

    doc_file = sys.argv[1]

    for i in range(2, len(sys.argv), 2):
        if i + 1 < len(sys.argv):
            key = sys.argv[i].lstrip('--').replace('-', '_')
            value = sys.argv[i + 1]
            config[key] = value

    ssh_client = SSHClient(config['vm_host'], config['vm_user'], config['vm_password'])
    claim_extractor = ClaimExtractor()
    executor = Executor(ssh_client, config['netdata_url'])
    reporter = Reporter(config['output_dir'])

    if not Path(doc_file).exists():
        print(f"Error: Documentation file not found: {doc_file}")
        sys.exit(1)

    print(f"Testing: {doc_file}")
    print(f"VM: {config['vm_user']}@{config['vm_host']}")
    print()

    claims = claim_extractor.parse_file(doc_file)
    print(f"Found {len(claims)} testable claims")
    print()

    if not claims:
        print("No testable claims found")
        sys.exit(0)

    test_results = []

    for i, claim in enumerate(claims, 1):
        claim_type = claim['type']

        if claim_type == 'command':
            result = executor.test_command(claim)
        elif claim_type == 'configuration':
            result = executor.test_configuration(claim)
        elif claim_type == 'api':
            result = executor.test_api_endpoint(claim)
        elif claim_type == 'behavioral':
            result = executor.test_behavioral_claim(claim)
        elif claim_type == 'workflow':
            workflow = type('Workflow', (), {
                'description': claim['description'],
                'start_line': int(claim['line_range'].split('-')[0]) if '-' in claim['line_range'] else int(claim['line_range']),
                'end_line': int(claim['line_range'].split('-')[1]) if '-' in claim['line_range'] else int(claim['line_range']),
                'steps': claim.get('steps', [])
            })()
            result = executor.execute_workflow(workflow)
        else:
            result = {'claim': claim, 'status': 'SKIP', 'error': 'Unknown claim type'}

        test_results.append(result)

        status_icon = {'PASS': '✅', 'FAIL': '❌', 'PARTIAL': '⚠️'}.get(result['status'], '❓')
        print(f"[{i}/{len(claims)}] Testing {claim['type']}...")
        print(f"  {status_icon} {result['status']}")
        if result.get('error'):
            print(f"  Error: {result['error']}")
        print()

    report = reporter.generate_markdown_report(test_results, doc_file, config['netdata_url'])
    report_file = reporter.save_report(report, doc_file)

    total = len(test_results)
    passed = sum(1 for r in test_results if r['status'] == 'PASS')
    failed = sum(1 for r in test_results if r['status'] == 'FAIL')
    partial = sum(1 for r in test_results if r['status'] == 'PARTIAL')

    print("=" * 60)
    print("SUMMARY")
    print("=" * 60)
    print(f"Total: {total} | ✅ Passed: {passed} | ❌ Failed: {failed} | ⚠️ Partial: {partial}")
    print(f"Report: {report_file}")
    print("=" * 60)

    sys.exit(1 if failed > 0 else 0)


if __name__ == '__main__':
    main()
