"""
Test reporters for formatting and outputting test results.
"""

from typing import Any, Dict, List

from .executors import TestExecutor, TestResult, TestStatus


class TestReporter:
    """Reports test results in various formats."""

    def __init__(self, output_format: str = "markdown"):
        self.output_format = output_format

    def generate_report(self, executor: TestExecutor) -> str:
        """Generate a report from test results."""
        if self.output_format == "markdown":
            return self._generate_markdown(executor)
        elif self.output_format == "json":
            return self._generate_json(executor)
        else:
            return self._generate_text(executor)

    def _generate_markdown(self, executor: TestExecutor) -> str:
        """Generate markdown report."""
        summary = executor.get_summary()
        
        report = "# Netdata Documentation Test Results\n\n"
        report += f"**Total:** {summary['total']}  \n"
        report += f"**Passed:** {summary['passed']} ✅  \n"
        report += f"**Failed:** {summary['failed']} ❌  \n"
        report += f"**Skipped:** {summary['skipped']} ⏭️  \n"
        report += f"**Errors:** {summary['errors']} ⚠️  \n\n"
        
        report += "## Details\n\n"
        for result in executor.results:
            status_emoji = {
                TestStatus.PASS: "✅",
                TestStatus.FAIL: "❌",
                TestStatus.SKIP: "⏭️",
                TestStatus.ERROR: "⚠️"
            }.get(result.status, "❓")
            
            report += f"### {status_emoji} {result.claim.claim_id}\n"
            report += f"**Status:** {result.status.value}  \n"
            report += f"**Message:** {result.message}  \n"
            report += f"**Duration:** {result.duration_ms:.1f}ms  \n\n"
        
        return report

    def _generate_json(self, executor: TestExecutor) -> str:
        """Generate JSON report."""
        import json
        
        data = {
            "summary": executor.get_summary(),
            "results": [
                {
                    "claim_id": r.claim.claim_id,
                    "status": r.status.value,
                    "message": r.message,
                    "duration_ms": r.duration_ms
                }
                for r in executor.results
            ]
        }
        return json.dumps(data, indent=2)

    def _generate_text(self, executor: TestExecutor) -> str:
        """Generate plain text report."""
        summary = executor.get_summary()
        
        lines = [
            "Netdata Documentation Test Results",
            "=" * 50,
            f"Total: {summary['total']}",
            f"Passed: {summary['passed']}",
            f"Failed: {summary['failed']}",
            f"Skipped: {summary['skipped']}",
            f"Errors: {summary['errors']}",
            ""
        ]
        
        for result in executor.results:
            lines.append(f"[{result.status.value}] {result.claim.claim_id}: {result.message}")
        
        return "\n".join(lines)
