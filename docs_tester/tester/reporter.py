"""Test report generation"""

from datetime import datetime
from pathlib import Path
from typing import List, Dict, Any


class Reporter:
    """Generate test reports"""

    def __init__(self, output_dir: str = 'test_results'):
        self.output_dir = Path(output_dir)
        self.output_dir.mkdir(exist_ok=True)

    def generate_markdown_report(
        self, test_results: List[Dict[str, Any]], doc_file: str, netdata_url: str
    ) -> str:
        """Generate markdown test report"""
        total = len(test_results)
        passed = sum(1 for r in test_results if r['status'] == 'PASS')
        failed = sum(1 for r in test_results if r['status'] == 'FAIL')
        partial = sum(1 for r in test_results if r['status'] == 'PARTIAL')

        report = f"""# Documentation Test Report

**Document**: {doc_file}
**Test Date**: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}
**Test Environment**: Netdata at {netdata_url}

## Summary
- **Total Claims**: {total}
- **Passed**: {passed} ✅
- **Failed**: {failed} ❌
- **Partial**: {partial} ⚠️

## Test Results

"""

        for result in test_results:
            status_emoji = {
                'PASS': '✅',
                'FAIL': '❌',
                'PARTIAL': '⚠️',
                'SKIP': '⏭️',
                'PENDING': '⏳'
            }.get(result['status'], '❓')

            claim = result['claim']
            claim_type = claim['type'].title()
            line_range = claim['line_range']
            description = claim['description']

            report += f"""
### {status_emoji} {result['status']}: {claim_type} (Line {line_range})

**Claim**: {description}

**What was tested**:

"""

            if result.get('steps'):
                for step in result['steps']:
                    report += f"- {step['description']}\n"

            if result.get('steps'):
                report += f"""
**Result**: {result['status']}

"""

            if result.get('stdout'):
                report += f"**Output**:\n```\n{result['stdout'][:500]}\n```\n\n"

            if result.get('error'):
                report += f"**Error**: {result['error']}\n\n"

            report += "---\n\n"

        return report

    def generate_json_report(
        self, test_results: List[Dict[str, Any]], doc_file: str, netdata_url: str
    ) -> str:
        """Generate JSON test report"""
        import json

        report = {
            'document': doc_file,
            'test_date': datetime.now().isoformat(),
            'environment': netdata_url,
            'summary': {
                'total': len(test_results),
                'passed': sum(1 for r in test_results if r['status'] == 'PASS'),
                'failed': sum(1 for r in test_results if r['status'] == 'FAIL'),
                'partial': sum(1 for r in test_results if r['status'] == 'PARTIAL')
            },
            'results': test_results
        }

        return json.dumps(report, indent=2)

    def save_report(self, report: str, doc_file: str, format: str = 'markdown') -> str:
        """Save report to file"""
        doc_name = Path(doc_file).stem
        timestamp = datetime.now().strftime('%Y%m%d_%H%M%S')
        extension = 'md' if format == 'markdown' else 'json'
        report_file = self.output_dir / f'{doc_name}_{timestamp}.{extension}'

        report_file.write_text(report)
        print(f"Report saved to: {report_file}")

        return str(report_file)
