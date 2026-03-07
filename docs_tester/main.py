#!/usr/bin/env python3
"""
Netdata Documentation Tester - Main CLI Entry Point

This tester follows the documentation exactly as a developer would,
executing every testable claim on a live Netdata installation.
"""

import sys
import argparse
import os
import json
from pathlib import Path
from datetime import datetime

from tester import SSHClient, ClaimExtractor, Executor, Reporter
from tester.claim_extractor import Step, StepType, contains_sensitive_placeholder
from config import VM_HOST, VM_USER, VM_PASSWORD, NETDATA_URL, OUTPUT_DIR


def main():
    parser = argparse.ArgumentParser(description='Test Netdata documentation claims')
    parser.add_argument('doc_file', help='Path to documentation file')
    parser.add_argument('--vm-host', help='Test VM IP (or set TEST_VM_HOST env var)')
    parser.add_argument('--vm-user', help='SSH user (or set TEST_VM_USER env var)')
    parser.add_argument('--netdata-url', help='Netdata URL (or set NETDATA_URL env var)')
    parser.add_argument('--output-dir', default=OUTPUT_DIR, help='Output directory')
    parser.add_argument('--format', choices=['markdown', 'json'], default='markdown', help='Report format')
    parser.add_argument('--dry-run', action='store_true', help='Extract claims and print test plan without executing')
    parser.add_argument('--timeout', type=int, default=30, help='Timeout per test in seconds (default: 30)')
    parser.add_argument('--log-dir', default='/var/log/netdata_tester', help='Log directory')

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
    print(f"Timeout: {args.timeout}s per test")
    if args.dry_run:
        print("MODE: DRY RUN (no execution)")
    print("-" * 60)

    claim_extractor = ClaimExtractor()

    # Write claim extraction review file
    print("\nParsing documentation...")
    claims = claim_extractor.parse_file(args.doc_file)
    timestamp = datetime.now().strftime('%Y%m%d_%H%M%S')
    claims_file = f'/tmp/netdata_tester_claims_{timestamp}.json'
    with open(claims_file, 'w') as f:
        json.dump(claims, f, indent=2)
    print(f"Claims written to: {claims_file}")

    # Filter claims
    code_claims = [c for c in claims if c['type'] in ['configuration', 'command', 'api', 'behavioral']]
    workflow_claims = [c for c in claims if c['type'] == 'workflow']

    # Print test plan
    print("\n" + "=" * 60)
    print("TEST PLAN")
    print("=" * 60)
    all_claims = code_claims + workflow_claims
    for i, claim in enumerate(all_claims, 1):
        desc = claim.get('description', 'N/A')[:80]
        claim_type = claim.get('type', 'N/A')
        # Check for sensitive placeholders
        content = claim.get('content', '') or claim.get('description', '')
        if contains_sensitive_placeholder(content):
            desc += " [CONTAINS SENSITIVE PLACEHOLDER]"
        print(f"[{i}] {claim_type}: {desc}")
    print()

    print(f"Found {len(code_claims)} code block claims")
    print(f"Found {len(workflow_claims)} workflow claims")
    print(f"Total: {len(all_claims)} testable claims")
    print()

    if not all_claims:
        print("No testable claims found in documentation")
        sys.exit(0)

    # Dry run mode - just print plan
    if args.dry_run:
        print("=" * 60)
        print("DRY RUN - No tests executed")
        print("=" * 60)
        sys.exit(0)

    # Initialize actual test components
    ssh_client = SSHClient(vm_host, vm_user, vm_password)
    executor = Executor(ssh_client, netdata_url, timeout=args.timeout)
    reporter = Reporter(args.output_dir)

    # Start logging
    executor.logger.start(args.doc_file)
    log_file = executor.logger.log_file

    # Capture Netdata version at start
    print("\nCapturing Netdata version...")
    version = executor.capture_version()
    print(f"Netdata version: {version}")

    # Check Netdata health before starting
    print("\nChecking Netdata health...")
    if not executor.check_netdata_health():
        print("WARNING: Netdata is not responding to API requests")
        print("Attempting to wait for Netdata to become healthy...")
        if not executor.wait_for_netdata(max_wait=60):
            print("ERROR: Netdata is not healthy. Aborting tests.")
            sys.exit(1)
    else:
        print("Netdata is healthy")
    print("-" * 60)

    # Check for prerequisites first
    prereq_claim = None
    if claims and claims[0].get('type') == 'prerequisite':
        prereq_claim = claims[0]
        print(f"\nFound prerequisites: {prereq_claim.get('prerequisites', [])}")

    test_results = []

    # Test prerequisites first
    if prereq_claim:
        print("=" * 60)
        print("CHECKING PREREQUISITES")
        print("=" * 60)
        # TODO: Implement prerequisite checking logic
        print("Prerequisites assumed satisfied (manual check required)")
        print()

    print("=" * 60)
    print("TESTING CODE BLOCKS")
    print("=" * 60)
    for i, claim in enumerate(code_claims, 1):
        print(f"[{i}/{len(code_claims)}] Testing {claim['type']}: {claim['description']}")

        # Check for sensitive placeholders
        content = claim.get('content', '') or claim.get('description', '')
        if contains_sensitive_placeholder(content):
            result = {
                'claim': claim,
                'status': 'SKIPPED',
                'error': 'SKIPPED: requires real credentials (contains placeholder)'
            }
            print(f"  Status: SKIPPED (contains sensitive placeholder)")
        elif claim['type'] == 'command':
            result = executor.test_command(claim)
        elif claim['type'] == 'configuration':
            result = executor.test_configuration(claim)
        elif claim['type'] == 'api':
            result = executor.test_api_endpoint(claim)
        elif claim['type'] == 'behavioral':
            result = executor.test_behavioral_claim(claim)
        else:
            result = {'claim': claim, 'status': 'SKIPPED', 'error': 'Unknown claim type'}

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
                step_type = step_item.get('type', 'COMMAND')
                if isinstance(step_type, str):
                    step_type = StepType[step_type.upper()]
                step = Step(
                    type=step_type,
                    instruction=step_item.get('instruction', ''),
                    expected=step_item.get('expected', 'Step completes successfully'),
                    number=idx
                )
                step_objects.append(step)

        # Create Workflow object
        line_range = claim['line_range']
        if '-' in line_range:
            start_line = int(line_range.split('-')[0])
            end_line = int(line_range.split('-')[1])
        else:
            start_line = end_line = int(line_range)

        workflow = type('Workflow', (), {
            'description': claim['description'],
            'start_line': start_line,
            'end_line': end_line,
            'steps': step_objects
        })()

        result = executor.execute_workflow(workflow)
        test_results.append(result)
        print(f"  Status: {result['status']}")
        if result.get('failed_at_step'):
            print(f"  Failed at step: {result['failed_at_step']}")
            print(f"  Error: {result.get('error', 'Unknown error')}")
        print()

    # Restore all backed up files
    print("=" * 60)
    print("CLEANING UP")
    print("=" * 60)
    restore_results = executor.restore_all()
    for r in restore_results:
        status = "restored" if r['restored'] else "FAILED"
        print(f"  {r['file']}: {status}")

    # Restart Netdata service
    print("\nRestarting Netdata service...")
    restart_result = ssh_client.sudo('systemctl restart netdata')
    if restart_result.get('success'):
        print("  Netdata service restarted")
    else:
        print(f"  WARNING: Failed to restart Netdata: {restart_result.get('stderr', '')}")
    print()

    print("Generating report...")
    if args.format == 'markdown':
        report = reporter.generate_markdown_report(test_results, args.doc_file, netdata_url)
    else:
        report = reporter.generate_json_report(test_results, args.doc_file, netdata_url)

    report_file = reporter.save_report(report, args.doc_file, args.format)

    # Count statuses
    total = len(test_results)
    passed = sum(1 for r in test_results if r.get('status') == 'PASS')
    failed = sum(1 for r in test_results if r.get('status') == 'FAIL')
    skipped = sum(1 for r in test_results if r.get('status') in ['SKIPPED', 'SKIP'])
    blocked = sum(1 for r in test_results if any(s.get('status') == 'BLOCKED' for s in r.get('steps', [])))

    print()
    print("=" * 60)
    print("TEST SUMMARY")
    print("=" * 60)
    print(f"Documentation page: {args.doc_file}")
    print(f"Netdata version: {version}")
    print(f"Total claims tested: {total}")
    print(f"PASS:     {passed}")
    print(f"FAIL:     {failed}")
    print(f"SKIPPED:  {skipped}")
    print(f"BLOCKED:  {blocked}")
    print()
    print("=" * 60)

    if failed > 0:
        sys.exit(1)


if __name__ == '__main__':
    main()
