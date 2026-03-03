#!/usr/bin/env python3
"""
Netdata Documentation Tester - Main CLI Entry Point
"""

import sys
import argparse
import os
from pathlib import Path

from tester import SSHClient, ClaimExtractor, Executor, Reporter
from tester.claim_extractor import Step, StepType
from config import VM_HOST, VM_USER, VM_PASSWORD, NETDATA_URL, OUTPUT_DIR


def main():
    parser = argparse.ArgumentParser(description='Test Netdata documentation claims')
    parser.add_argument('doc_file', help='Path to documentation file')
    parser.add_argument('--vm-host', help='Test VM IP (or set TEST_VM_HOST env var)')
    parser.add_argument('--vm-user', help='SSH user (or set TEST_VM_USER env var)')
    parser.add_argument('--netdata-url', help='Netdata URL (or set NETDATA_URL env var)')
    parser.add_argument('--output-dir', default=OUTPUT_DIR, help='Output directory')
    parser.add_argument('--format', choices=['markdown', 'json'], default='markdown', help='Report format')

    args = parser.parse_args()

    if not Path(args.doc_file).exists():
        print(f"Error: Documentation file not found: {args.doc_file}")
        sys.exit(1)

    # Use CLI args or fall back to environment variables
    vm_host = args.vm_host or VM_HOST
    vm_user = args.vm_user or VM_USER
    vm_password = os.environ.get('TEST_VM_PASSWORD', VM_PASSWORD)
    netdata_url = args.netdata_url or NETDATA_URL or (f'http://{vm_host}:19999' if vm_host else '')

    print(f"Testing documentation: {args.doc_file}")
    print(f"Test VM: {vm_host}")
    print(f"Netdata URL: {netdata_url}")
    print("-" * 60)

    ssh_client = SSHClient(vm_host, vm_user, vm_password)
    claim_extractor = ClaimExtractor()
    executor = Executor(ssh_client, netdata_url)
    reporter = Reporter(args.output_dir)

    print("Parsing documentation...")
    claims = claim_extractor.parse_file(args.doc_file)

    code_claims = [c for c in claims if c['type'] in ['configuration', 'command', 'api', 'behavioral']]
    workflow_claims = [c for c in claims if c['type'] == 'workflow']

    print(f"Found {len(code_claims)} code block claims")
    print(f"Found {len(workflow_claims)} workflow claims")
    print(f"Total: {len(claims)} testable claims")
    print()

    if not claims:
        print("No testable claims found in documentation")
        sys.exit(0)

    test_results = []

    print("=" * 60)
    print("TESTING CODE BLOCKS")
    print("=" * 60)
    for i, claim in enumerate(code_claims, 1):
        print(f"[{i}/{len(code_claims)}] Testing {claim['type']}: {claim['description']}")

        if claim['type'] == 'command':
            result = executor.test_command(claim)
        elif claim['type'] == 'configuration':
            result = executor.test_configuration(claim)
        elif claim['type'] == 'api':
            result = executor.test_api_endpoint(claim)
        elif claim['type'] == 'behavioral':
            result = executor.test_behavioral_claim(claim)
        else:
            result = {'claim': claim, 'status': 'SKIP', 'error': 'Unknown claim type'}

        test_results.append(result)
        print(f"  Status: {result['status']}")
        print()

    print("=" * 60)
    print("TESTING WORKFLOWS")
    print("=" * 60)
    for i, claim in enumerate(workflow_claims, 1):
        print(f"[{i}/{len(workflow_claims)}] Testing workflow: {claim['description']}")

        # Convert dict steps to Step objects (if not already Step objects)
        step_objects = []
        for idx, step_item in enumerate(claim.get('steps', []), 1):
            # Check if already a Step object
            if hasattr(step_item, 'instruction'):
                step_item.number = idx
                step_objects.append(step_item)
            else:
                # It's a dict, convert to Step
                step = Step(
                    type=StepType[step_item.get('type', 'COMMAND').upper()] if isinstance(step_item.get('type'), str) else step_item.get('type', StepType.COMMAND),
                    instruction=step_item.get('instruction', ''),
                    expected=step_item.get('expected', 'Step completes successfully'),
                    number=idx
                )
                step_objects.append(step)

        # Create Workflow object
        workflow = type('Workflow', (), {
            'description': claim['description'],
            'start_line': int(claim['line_range'].split('-')[0]) if '-' in claim['line_range'] else int(claim['line_range']),
            'end_line': int(claim['line_range'].split('-')[1]) if '-' in claim['line_range'] else int(claim['line_range']),
            'steps': step_objects
        })()

        result = executor.execute_workflow(workflow)
        test_results.append(result)
        print(f"  Status: {result['status']}")
        if result.get('failed_at_step'):
            print(f"  Failed at step: {result['failed_at_step']}")
            print(f"  Error: {result.get('error', 'Unknown error')}")
        print()

    print("Generating report...")
    if args.format == 'markdown':
        report = reporter.generate_markdown_report(test_results, args.doc_file, netdata_url)
    else:
        report = reporter.generate_json_report(test_results, args.doc_file, netdata_url)

    report_file = reporter.save_report(report, args.doc_file, args.format)

    total = len(test_results)
    passed = sum(1 for r in test_results if r['status'] == 'PASS')
    failed = sum(1 for r in test_results if r['status'] == 'FAIL')
    partial = sum(1 for r in test_results if r['status'] == 'PARTIAL')

    print()
    print("=" * 60)
    print("TEST SUMMARY")
    print("=" * 60)
    print(f"Total: {total} | Passed: {passed} | Failed: {failed} | Partial: {partial}")
    print(f"Report: {report_file}")
    print("=" * 60)

    if failed > 0:
        sys.exit(1)


if __name__ == '__main__':
    main()
