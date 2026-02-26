#!/usr/bin/env python3
"""
Netdata Documentation Testing Agent - CLI

Validates Netdata documentation claims against a live Netdata Agent.
"""

import argparse
import sys
from pathlib import Path

def main():
    parser = argparse.ArgumentParser(
        description="Netdata Documentation Testing Agent",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  %(prog)s docs/config.md                    # Test single file
  %(prog)s docs/*.md --verbose               # Test all docs with details
  %(prog)s --from-pr 21711                   # Test PR documentation
  %(prog)s docs/config.md --dry-run          # Show test plan only
        """
    )
    
    parser.add_argument("files", nargs="*", help="Documentation files to test")
    parser.add_argument("--from-pr", type=int, help="Test documentation from GitHub PR")
    parser.add_argument("--dry-run", action="store_true", help="Show test plan without executing")
    parser.add_argument("--verbose", "-v", action="store_true", help="Verbose output")
    parser.add_argument("--output", "-o", choices=["markdown", "json", "text"], 
                       default="markdown", help="Output format")
    
    args = parser.parse_args()
    
    print("=" * 60)
    print("Netdata Documentation Testing Agent")
    print("=" * 60)
    
    # Import here to avoid circular imports
    from netdata_tester.parser import DocumentationParser
    from netdata_tester.executors import TestExecutor
    from netdata_tester.managers import ConfigManager
    from netdata_tester.reporters import TestReporter
    from netdata_tester.utils import VMConfig
    
    if args.from_pr:
        print(f"\n📥 Testing PR #{args.from_pr}")
        print("Note: PR fetching not yet implemented")
        return 0
    
    if not args.files:
        print("\n❌ Error: No files specified")
        parser.print_help()
        return 1
    
    # Parse documentation
    parser_obj = DocumentationParser(verbose=args.verbose)
    all_claims = []
    
    print("\n📄 Parsing documentation files...")
    for file_path in args.files:
        try:
            claims = parser_obj.parse_file(file_path)
            all_claims.extend(claims)
            print(f"  ✓ {file_path}: {len(claims)} claims")
        except Exception as e:
            print(f"  ✗ {file_path}: {e}")
            return 1
    
    print(f"\n📊 Total claims extracted: {len(all_claims)}")
    
    if args.dry_run:
        print("\n🔍 DRY RUN - Test Plan:")
        print("-" * 60)
        for claim in all_claims:
            print(f"  [{claim.claim_type.value:15}] {claim.claim_id}")
        return 0
    
    # Execute tests
    print("\n🧪 Executing tests...")
    vm_config = VMConfig()
    config_manager = ConfigManager(vm_config)
    executor = TestExecutor(vm_config, config_manager, verbose=args.verbose)
    
    results = executor.execute_all(all_claims)
    
    # Generate report
    reporter = TestReporter(output_format=args.output)
    report = reporter.generate_report(executor)
    
    print("\n" + "=" * 60)
    print(report)
    
    # Return exit code
    summary = executor.get_summary()
    return 0 if summary['failed'] == 0 else 1

if __name__ == "__main__":
    sys.exit(main())
