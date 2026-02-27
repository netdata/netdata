#!/usr/bin/env python3
"""
Netdata Documentation Tester - CLI Wrapper
Convenient wrapper for running documentation tests
"""

import os
import sys
import subprocess
from pathlib import Path

def main():
    # Add the tester directory to path
    tester_dir = Path(__file__).parent
    sys.path.insert(0, str(tester_dir))
    
    from tester import DocumentationTester
    
    # Default configuration - load from environment variables
    config = {
        'vm_host': os.environ.get('TEST_VM_HOST'),
        'vm_user': os.environ.get('TEST_VM_USER'),
        'vm_password': os.environ.get('TEST_VM_PASSWORD'),
        'netdata_url': os.environ.get('NETDATA_URL'),
        'output_dir': 'test_results'
    }
    
    if len(sys.argv) < 2:
        print("Usage: python run_tester.py <documentation_file> [options]")
        print()
        print("Example:")
        print("  python run_tester.py ./docs/netdata-conf.md")
        print()
        sys.exit(1)
    
    # Parse arguments
    doc_file = sys.argv[1]
    
    # Override config if provided
    for i in range(2, len(sys.argv), 2):
        if i + 1 < len(sys.argv):
            key = sys.argv[i].lstrip('--').replace('-', '_')
            value = sys.argv[i + 1]
            config[key] = value
    
    # Run tests
    tester = DocumentationTester(config)
    
    if not Path(doc_file).exists():
        print(f"Error: Documentation file not found: {doc_file}")
        sys.exit(1)
    
    print(f"Testing: {doc_file}")
    print(f"VM: {config['vm_user']}@{config['vm_host']}")
    print()
    
    claims = tester.parse_documentation(doc_file)
    print(f"Found {len(claims)} testable claims")
    print()
    
    if not claims:
        print("No testable claims found")
        sys.exit(0)
    
    # Test each claim
    for i, claim in enumerate(claims, 1):
        print(f"[{i}/{len(claims)}] Testing {claim['type']}...")
        result = tester.test_claim(claim)
        tester.test_results.append(result)
        status_icon = {'PASS': '✅', 'FAIL': '❌', 'PARTIAL': '⚠️'}.get(result['status'], '❓')
        print(f"  {status_icon} {result['status']}")
        if result.get('error'):
            print(f"  Error: {result['error']}")
        print()
    
    # Generate report
    report = tester.generate_report(doc_file)
    report_file = tester.save_report(report, doc_file)
    
    # Summary
    total = len(tester.test_results)
    passed = sum(1 for r in tester.test_results if r['status'] == 'PASS')
    failed = sum(1 for r in tester.test_results if r['status'] == 'FAIL')
    partial = sum(1 for r in tester.test_results if r['status'] == 'PARTIAL')
    
    print("=" * 60)
    print("SUMMARY")
    print("=" * 60)
    print(f"Total: {total} | ✅ Passed: {passed} | ❌ Failed: {failed} | ⚠️ Partial: {partial}")
    print(f"Report: {report_file}")
    print("=" * 60)
    
    sys.exit(1 if failed > 0 else 0)

if __name__ == '__main__':
    main()
