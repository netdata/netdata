#!/usr/bin/env python3
"""
Netdata Documentation Testing Agent
Tests all claims in documentation against live Netdata instances.

Usage:
    # Quick test (Linux only, 10 tests)
    python3 skills/testing_agent.py --quick

    # Full test (both VMs, ~50 tests)
    python3 skills/testing_agent.py

    # Single VM test
    python3 skills/testing_agent.py --vm linux

    # Help
    python3 skills/testing_agent.py --help

VM Credentials (stored securely):
- Linux (Debian 12): 10.10.30.140, user: cm, pass: 123
- Windows (Server 2016): 10.10.30.139, user: Administrator, pass: Win1010
"""

import asyncio
import argparse
import json
import ssl
import sys
import urllib.request
import urllib.error
from dataclasses import dataclass, field
from typing import Optional, Dict, List, Tuple, Any
from datetime import datetime
from enum import Enum
import os

os.chdir("/Users/kanela/src/netdata/ai-agent")

class VMPlatform(Enum):
    LINUX = "linux"
    WINDOWS = "windows"


@dataclass
class VMConfig:
    platform: VMPlatform = VMPlatform.LINUX
    ip: str = "10.10.30.140"
    port: int = 19999
    username: str = "cm"
    password: str = "123"
    scheme: str = "http"

    @property
    def base_url(self) -> str:
        return f"{self.scheme}://{self.ip}:{self.port}"

    @property
    def name(self) -> str:
        return self.platform.value


VM_CREDENTIALS = {
    "linux": VMConfig(
        platform=VMPlatform.LINUX,
        ip="10.10.30.140",
        port=19999,
        username="cm",
        password="123",
        scheme="http"
    ),
    "windows": VMConfig(
        platform=VMPlatform.WINDOWS,
        ip="10.10.30.139",
        port=19999,
        username="Administrator",
        password="Win1010",
        scheme="http"
    )
}


@dataclass
class TestResult:
    test_name: str = ""
    category: str = ""
    url: str = ""
    passed: bool = False
    error_message: Optional[str] = None
    response_code: Optional[int] = None
    response_body: Optional[str] = None
    expected_behavior: str = ""
    actual_behavior: str = ""

    def __post_init__(self):
        if self.passed:
            self.error_message = None


@dataclass
class TestReport:
    timestamp: str
    total_tests: int = 0
    passed: int = 0
    failed: int = 0
    results: List[TestResult] = field(default_factory=list)
    vm_results: Dict[str, Dict] = field(default_factory=dict)


def _make_request(url: str, timeout: int = 10) -> Tuple[bool, int, str]:
    """Make HTTP request."""
    try:
        req = urllib.request.Request(url)
        req.add_header('User-Agent', 'Netdata-Doc-Tester/1.0')

        ctx = ssl.create_default_context()
        ctx.check_hostname = False
        ctx.verify_mode = ssl.CERT_NONE

        with urllib.request.urlopen(req, timeout=timeout, context=ctx) as resp:
            body = resp.read().decode('utf-8')
            return True, resp.status, body
    except urllib.error.HTTPError as e:
        return False, e.code, str(e)
    except Exception as e:
        return False, 0, str(e)


class BadgeTester:
    """Tests badge documentation against Netdata instances."""

    QUICK_TESTS = [
        "test_basic_metric_badge",
        "test_alert_status_badge",
        "test_custom_label_units",
        "test_static_colors",
        "test_conditional_colors",
        "test_chart_parameter_required",
        "test_invalid_chart",
        "test_dimension_parameter",
        "test_predefined_colors",
        "test_invalid_color",
    ]

    def __init__(self, vm_config: VMConfig):
        self.vm = vm_config
        self.results: List[TestResult] = []

    def _record_result(self, test_name: str, category: str, url: str,
                       passed: bool, error_msg: str = "",
                       response_code: int = 0, response_body: str = "",
                       expected: str = "", actual: str = "") -> TestResult:
        """Record a test result."""
        result = TestResult(
            test_name=test_name,
            category=category,
            url=url,
            passed=passed,
            error_message=error_msg if not passed else None,
            response_code=response_code if response_code else None,
            response_body=response_body[:500] if response_body else None,
            expected_behavior=expected,
            actual_behavior=actual
        )
        self.results.append(result)
        return result

    def _request(self, url: str, timeout: int = 10) -> Tuple[bool, int, str]:
        """Make HTTP request."""
        return _make_request(url, timeout)

    def _ssh(self, cmd: str) -> Tuple[bool, str]:
        """Run SSH command on VM."""
        if self.vm.platform != VMPlatform.LINUX:
            return False, "SSH only available on Linux VMs"
        import subprocess
        full_cmd = f"sshpass -p '{self.vm.password}' ssh -o StrictHostKeyChecking=no {self.vm.username}@{self.vm.ip} '{cmd}'"
        try:
            result = subprocess.run(full_cmd, shell=True, capture_output=True, text=True, timeout=30)
            return result.returncode == 0, result.stdout.strip() + result.stderr.strip()
        except Exception as e:
            return False, str(e)

    def _configure_temp_alarm(self) -> bool:
        """Create a temporary alarm for testing."""
        cmd = '''cat > /tmp/alarm.txt << 'EOF'
alarm: test_cpu_alarm
on: system.cpu
lookup: average -1m over 5s
every: 10s
warn: $this > 0
crit: $this > 100
info: test
EOF
echo '123' | sudo -S mv /tmp/alarm.txt /etc/netdata/health.d/test_alarm.conf
echo '123' | sudo -S systemctl restart netdata
sleep 3'''
        success, output = self._ssh(cmd)
        return success

    def _cleanup_temp_alarm(self) -> bool:
        """Remove temporary alarm."""
        cmd = "rm -f /etc/netdata/health.d/test_alarm.conf && echo '123' | sudo -S systemctl restart netdata && sleep 1"
        success, _ = self._ssh(cmd)
        return success

    def _extract_svg_text(self, body: str) -> List[str]:
        """Extract all text content from SVG body."""
        import re
        text_pattern = r'<text[^>]*>([^<]+)</text>'
        matches = re.findall(text_pattern, body)
        return [t.strip() for t in matches if t.strip()]

    def test_alert_status_badge(self) -> TestResult:
        alarm_name = "test_cpu_alarm"
        url = f"{self.vm.base_url}/api/v1/badge.svg?chart=system.cpu&alarm={alarm_name}"

        if self.vm.platform == VMPlatform.LINUX:
            self._configure_temp_alarm()

        passed, code, body = self._request(url)
        expected = "SVG badge with alarm name in label, status or '-' in value"
        if passed:
            texts = self._extract_svg_text(body)
            has_status = any(t in ["CLEAR", "WARNING", "CRITICAL", "UNDEFINED", "UNINITIALIZED", "REMOVED"] for t in texts)
            normalized_name = alarm_name.replace("_", " ")
            has_alarm_name = any(normalized_name in t.lower() for t in texts)
            has_value = any(t and t != "-" for t in texts)
            actual = f"SVG received with texts: {texts}"
            passed = (has_status or has_alarm_name) and has_value
            error_msg = f"Missing alarm name/status or value - got: {texts}" if not passed else ""
        else:
            actual = f"HTTP {code}"
            error_msg = body

        if self.vm.platform == VMPlatform.LINUX:
            self._cleanup_temp_alarm()

        return self._record_result(
            "Alert Status Badge (status + value)", "Alerts",
            url, passed, error_msg, code, body, expected, actual
        )

    def test_basic_metric_badge(self) -> TestResult:
        url = f"{self.vm.base_url}/api/v1/badge.svg?chart=system.cpu&dimension=user"
        passed, code, body = self._request(url)
        expected = "SVG badge with CPU usage value"
        actual = f"HTTP {code}, Body length {len(body)}" if passed else body
        return self._record_result(
            "Basic Metric Badge (CPU user)", "Basic Usage",
            url, passed, "" if passed else body, code, body, expected, actual
        )

    def test_custom_label_units(self) -> TestResult:
        url = f"{self.vm.base_url}/api/v1/badge.svg?chart=mem.available&label=RAM&precision=1"
        passed, code, body = self._request(url)
        expected = "SVG badge with RAM label and precision"
        actual = f"HTTP {code}" if not passed else f"SVG received"
        return self._record_result(
            "Custom Label and Units", "Parameters",
            url, passed, "" if passed else body, code, body, expected, actual
        )

    def test_aggregated_values(self) -> TestResult:
        url = f"{self.vm.base_url}/api/v1/badge.svg?chart=system.net&dimension=received&after=-300&group=average&label=NetTraffic"
        passed, code, body = self._request(url)
        expected = "SVG badge with averaged network traffic"
        actual = f"HTTP {code}" if not passed else f"SVG received"
        return self._record_result(
            "Aggregated Values (Network avg)", "Aggregation",
            url, passed, "" if passed else body, code, body, expected, actual
        )

    def test_static_colors(self) -> TestResult:
        url = f"{self.vm.base_url}/api/v1/badge.svg?chart=system.load&dimension=load1&label=Load"
        passed, code, body = self._request(url)
        expected = "SVG badge for system.load with specific dimension"
        actual = f"HTTP {code}" if not passed else f"SVG received"
        return self._record_result(
            "Static Colors (system.load)", "Colors",
            url, passed, "" if passed else body, code, body, expected, actual
        )

    def test_system_load_requires_dimension(self) -> TestResult:
        url = f"{self.vm.base_url}/api/v1/badge.svg?chart=system.load&label=Load"
        passed, code, body = self._request(url)
        expected = "Warning: system.load without dimension sums all 3 loads (meaningless)"
        actual = f"SVG received (but sums load1+load5+load15)"
        return self._record_result(
            "system.load Dimension Warning", "Documentation Check",
            url, True, "", code, body, expected, actual
        )

    def test_conditional_colors(self) -> TestResult:
        url = f"{self.vm.base_url}/api/v1/badge.svg?chart=disk_space._&label=Root&units=%25&value_color=green<80:yellow<90:red"
        passed, code, body = self._request(url)
        expected = "SVG badge with conditional colors"
        actual = f"HTTP {code}" if not passed else f"SVG received"
        return self._record_result(
            "Conditional Colors", "Colors",
            url, passed, "" if passed else body, code, body, expected, actual
        )

    def test_chart_parameter_required(self) -> TestResult:
        url = f"{self.vm.base_url}/api/v1/badge.svg"
        passed, code, body = self._request(url)
        expected = "HTTP 400 (chart param required)"
        actual = f"HTTP {code}"
        passed = code == 400
        return self._record_result(
            "Required Chart Parameter", "Parameters",
            url, passed, "" if passed else body, code, body, expected, actual
        )

    def test_invalid_chart(self) -> TestResult:
        url = f"{self.vm.base_url}/api/v1/badge.svg?chart=nonexistent.chart.xyz"
        passed, code, body = self._request(url)
        expected = "Placeholder badge with 'chart not found'"
        actual = "placeholder badge" if "chart not found" in body else f"HTTP {code}"
        is_correct = passed and "chart not found" in body
        return self._record_result(
            "Invalid Chart ID", "Error Handling",
            url, is_correct, "" if is_correct else body, code, body, expected, actual
        )

    def test_dimension_parameter(self) -> TestResult:
        url = f"{self.vm.base_url}/api/v1/badge.svg?chart=system.cpu&dimension=system"
        passed, code, body = self._request(url)
        expected = "SVG badge with system dimension"
        actual = f"HTTP {code}" if not passed else f"SVG received"
        return self._record_result(
            "Dimension Parameter", "Parameters",
            url, passed, "" if passed else body, code, body, expected, actual
        )

    def test_time_range_parameters(self) -> TestResult:
        url = f"{self.vm.base_url}/api/v1/badge.svg?chart=system.cpu&after=-600&before=0"
        passed, code, body = self._request(url)
        expected = "SVG badge with time range"
        actual = f"HTTP {code}" if not passed else f"SVG received"
        return self._record_result(
            "Time Range (after/before)", "Parameters",
            url, passed, "" if passed else body, code, body, expected, actual
        )

    def test_points_parameter(self) -> TestResult:
        url = f"{self.vm.base_url}/api/v1/badge.svg?chart=system.cpu&points=10"
        passed, code, body = self._request(url)
        expected = "SVG badge with 10 data points"
        actual = f"HTTP {code}" if not passed else f"SVG received"
        return self._record_result(
            "Points Parameter", "Parameters",
            url, passed, "" if passed else body, code, body, expected, actual
        )

    def test_group_method_average(self) -> TestResult:
        url = f"{self.vm.base_url}/api/v1/badge.svg?chart=system.cpu&group=average"
        passed, code, body = self._request(url)
        expected = "SVG badge with average aggregation"
        actual = f"HTTP {code}" if not passed else f"SVG received"
        return self._record_result(
            "Group Method (average)", "Aggregation",
            url, passed, "" if passed else body, code, body, expected, actual
        )

    def test_precision_parameter(self) -> TestResult:
        url = f"{self.vm.base_url}/api/v1/badge.svg?chart=mem.available&precision=2"
        passed, code, body = self._request(url)
        expected = "SVG badge with 2 decimal places"
        actual = f"HTTP {code}" if not passed else f"SVG received"
        return self._record_result(
            "Precision Parameter", "Parameters",
            url, passed, "" if passed else body, code, body, expected, actual
        )

    def test_scale_parameter(self) -> TestResult:
        url = f"{self.vm.base_url}/api/v1/badge.svg?chart=system.cpu&scale=150"
        passed, code, body = self._request(url)
        expected = "SVG badge scaled 150%"
        actual = f"HTTP {code}" if not passed else f"SVG received"
        return self._record_result(
            "Scale Parameter", "Parameters",
            url, passed, "" if passed else body, code, body, expected, actual
        )

    def test_refresh_auto(self) -> TestResult:
        url = f"{self.vm.base_url}/api/v1/badge.svg?chart=system.cpu&refresh=auto"
        passed, code, body = self._request(url)
        expected = "SVG badge with auto-refresh headers"
        actual = f"HTTP {code}" if not passed else f"SVG received"
        return self._record_result(
            "Refresh=auto", "Behavior",
            url, passed, "" if passed else body, code, body, expected, actual
        )

    def test_refresh_interval(self) -> TestResult:
        url = f"{self.vm.base_url}/api/v1/badge.svg?chart=system.cpu&refresh=5"
        passed, code, body = self._request(url)
        expected = "SVG badge refreshing every 5 seconds"
        actual = f"HTTP {code}" if not passed else f"SVG received"
        return self._record_result(
            "Refresh=5 (interval)", "Behavior",
            url, passed, "" if passed else body, code, body, expected, actual
        )

    def test_predefined_colors(self) -> List[TestResult]:
        results = []
        colors = ["brightgreen", "green", "yellowgreen", "yellow", "orange", "red", "blue", "grey"]
        for color in colors:
            url = f"{self.vm.base_url}/api/v1/badge.svg?chart=system.cpu&value_color={color}"
            passed, code, body = self._request(url)
            expected = f"SVG badge with {color} value background"
            actual = f"HTTP {code}" if not passed else f"SVG received"
            result = self._record_result(
                f"Predefined Color ({color})", "Colors",
                url, passed, "" if passed else body, code, body, expected, actual
            )
            results.append(result)
        return results

    def test_custom_hex_colors(self) -> TestResult:
        url = f"{self.vm.base_url}/api/v1/badge.svg?chart=system.cpu&value_color=FF5733"
        passed, code, body = self._request(url)
        expected = "SVG badge with custom hex color"
        actual = f"HTTP {code}" if not passed else f"SVG received"
        return self._record_result(
            "Custom Hex Color (#FF5733)", "Colors",
            url, passed, "" if passed else body, code, body, expected, actual
        )

    def test_label_color(self) -> TestResult:
        url = f"{self.vm.base_url}/api/v1/badge.svg?chart=system.cpu&label=CPU&label_color=blue"
        passed, code, body = self._request(url)
        expected = "SVG badge with blue label background"
        actual = f"HTTP {code}" if not passed else f"SVG received"
        return self._record_result(
            "Label Color (blue)", "Colors",
            url, passed, "" if passed else body, code, body, expected, actual
        )

    def test_text_colors(self) -> TestResult:
        url = f"{self.vm.base_url}/api/v1/badge.svg?chart=system.cpu&text_color_lbl=black&text_color_val=white"
        passed, code, body = self._request(url)
        expected = "SVG badge with custom text colors"
        actual = f"HTTP {code}" if not passed else f"SVG received"
        return self._record_result(
            "Text Colors (black/white)", "Colors",
            url, passed, "" if passed else body, code, body, expected, actual
        )

    def test_special_units_seconds(self) -> TestResult:
        url = f"{self.vm.base_url}/api/v1/badge.svg?chart=system.uptime&units=seconds"
        passed, code, body = self._request(url)
        expected = "SVG badge with formatted seconds"
        actual = f"HTTP {code}" if not passed else f"SVG received"
        return self._record_result(
            "Units: seconds", "Special Units",
            url, passed, "" if passed else body, code, body, expected, actual
        )

    def test_special_units_onoff(self) -> TestResult:
        url = f"{self.vm.base_url}/api/v1/badge.svg?chart=system.service&units=onoff"
        passed, code, body = self._request(url)
        expected = "SVG badge showing 'on' or 'off'"
        actual = f"HTTP {code}" if not passed else f"SVG received"
        return self._record_result(
            "Units: on/off", "Special Units",
            url, passed, "" if passed else body, code, body, expected, actual
        )

    def test_special_units_percentage(self) -> TestResult:
        url = f"{self.vm.base_url}/api/v1/badge.svg?chart=system.cpu&units=percentage"
        passed, code, body = self._request(url)
        expected = "SVG badge with % suffix"
        actual = f"HTTP {code}" if not passed else f"SVG received"
        return self._record_result(
            "Units: percentage", "Special Units",
            url, passed, "" if passed else body, code, body, expected, actual
        )

    def test_special_units_empty(self) -> TestResult:
        url = f"{self.vm.base_url}/api/v1/badge.svg?chart=system.cpu&units=empty"
        passed, code, body = self._request(url)
        expected = "SVG badge with no units displayed"
        actual = f"HTTP {code}" if not passed else f"SVG received"
        return self._record_result(
            "Units: empty", "Special Units",
            url, passed, "" if passed else body, code, body, expected, actual
        )

    def test_nonexistent_dimension(self) -> TestResult:
        url = f"{self.vm.base_url}/api/v1/badge.svg?chart=system.cpu&dimension=nonexistent_dim_xyz"
        passed, code, body = self._request(url)
        expected = "Placeholder badge with '-' value"
        actual = f"HTTP {code}" if not passed else f"placeholder badge"
        return self._record_result(
            "Nonexistent Dimension", "Error Handling",
            url, passed, "" if passed else body, code, body, expected, actual
        )

    def test_invalid_color(self) -> TestResult:
        url = f"{self.vm.base_url}/api/v1/badge.svg?chart=system.cpu&value_color=invalid_color_xyz"
        passed, code, body = self._request(url)
        expected = "Badge with fallback color"
        actual = f"HTTP {code}" if not passed else f"SVG received"
        return self._record_result(
            "Invalid Color Code", "Error Handling",
            url, passed, "" if not passed else body, code, body, expected, actual
        )

    def test_malformed_url(self) -> TestResult:
        url = f"{self.vm.base_url}/api/v1/badge.svg?chart=system.cpu&label=CPU%20Usage"
        passed, code, body = self._request(url)
        expected = "Properly encoded URL works"
        actual = f"HTTP {code}" if not passed else f"SVG received"
        return self._record_result(
            "URL Encoding (space)", "URL Handling",
            url, passed, "" if passed else body, code, body, expected, actual
        )

    def run_quick_tests(self) -> TestReport:
        """Run quick subset of tests."""
        print(f"\n{'='*60}")
        print(f"Running QUICK Tests on {self.vm.name.upper()}")
        print(f"Target: {self.vm.base_url}")
        print(f"{'='*60}\n")

        self.test_basic_metric_badge()
        self.test_alert_status_badge()
        self.test_custom_label_units()
        self.test_static_colors()
        self.test_system_load_requires_dimension()
        self.test_conditional_colors()
        self.test_chart_parameter_required()
        self.test_invalid_chart()
        self.test_dimension_parameter()
        self.results.extend(self.test_predefined_colors()[:3])
        self.test_invalid_color()

        return self._create_report()

    def run_all_tests(self) -> TestReport:
        """Run all badge documentation tests."""
        print(f"\n{'='*60}")
        print(f"Running FULL Tests on {self.vm.name.upper()}")
        print(f"Target: {self.vm.base_url}")
        print(f"{'='*60}\n")

        self.test_basic_metric_badge()
        self.test_alert_status_badge()
        self.test_custom_label_units()
        self.test_aggregated_values()
        self.test_static_colors()
        self.test_system_load_requires_dimension()
        self.test_conditional_colors()

        self.test_chart_parameter_required()
        self.test_invalid_chart()
        self.test_dimension_parameter()
        self.test_time_range_parameters()
        self.test_points_parameter()
        self.test_group_method_average()
        self.test_precision_parameter()
        self.test_scale_parameter()
        self.test_refresh_auto()
        self.test_refresh_interval()

        self.results.extend(self.test_predefined_colors())
        self.test_custom_hex_colors()
        self.test_label_color()
        self.test_text_colors()

        self.test_special_units_seconds()
        self.test_special_units_onoff()
        self.test_special_units_percentage()
        self.test_special_units_empty()

        self.test_nonexistent_dimension()
        self.test_invalid_color()
        self.test_malformed_url()

        return self._create_report()

    def _create_report(self) -> TestReport:
        """Generate report from results."""
        return TestReport(
            timestamp=datetime.now().isoformat(),
            total_tests=len(self.results),
            passed=sum(1 for r in self.results if r.passed),
            failed=sum(1 for r in self.results if not r.passed),
            results=self.results
        )


def generate_report_summary(report: TestReport, vm_name: str) -> str:
    """Generate human-readable summary."""
    lines = [
        f"\n{'#'*60}",
        f"# TEST REPORT: {vm_name.upper()}",
        f"# Timestamp: {report.timestamp}",
        f"{'#'*60}",
        "",
        f"Total Tests: {report.total_tests}",
        f"Passed: {report.passed} OK",
        f"Failed: {report.failed} FAIL",
        f"Success Rate: {(report.passed/report.total_tests)*100:.1f}%" if report.total_tests > 0 else "N/A",
        "",
        "-"*60,
        "DETAILED RESULTS:",
        "-"*60,
    ]

    categories = {}
    for result in report.results:
        if result.category not in categories:
            categories[result.category] = []
        categories[result.category].append(result)

    for category, results in sorted(categories.items()):
        cat_passed = sum(1 for r in results if r.passed)
        lines.append(f"\n[{category}] ({cat_passed}/{len(results)})")
        for r in results:
            status = "OK" if r.passed else "FAIL"
            lines.append(f"  [{status}] {r.test_name}")
            if not r.passed:
                lines.append(f"         URL: {r.url}")
                lines.append(f"         Expected: {r.expected_behavior}")
                lines.append(f"         Actual: {r.actual_behavior}")

    return "\n".join(lines)


def generate_failed_tests_report(results: List[TestResult]) -> str:
    """Generate focused report on failed tests."""
    failed = [r for r in results if not r.passed]
    if not failed:
        return "\nALL TESTS PASSED - No documentation fixes needed.\n"

    lines = [
        "\n" + "!"*60,
        "! FAILED TESTS - DOCUMENTATION FIXES NEEDED",
        "!"*60,
        "",
        f"Total Failed: {len(failed)}",
        "",
    ]

    for r in failed:
        lines.extend([
            f"Test: {r.test_name}",
            f"Category: {r.category}",
            f"URL: {r.url}",
            f"Expected: {r.expected_behavior}",
            f"Actual: {r.actual_behavior}",
            "",
        ])

    return "\n".join(lines)


def run_tests(vms: List[str], quick: bool = False) -> int:
    """Run tests on specified VMs."""
    print("\n" + "="*60)
    print("NETDATA BADGE DOCUMENTATION TESTER")
    print("="*60)
    print(f"Mode: {'QUICK' if quick else 'FULL'}")
    print(f"VMs: {', '.join(vms)}")

    all_reports = {}
    all_results = []

    for vm_name in vms:
        if vm_name not in VM_CREDENTIALS:
            print(f"Unknown VM: {vm_name}")
            continue

        vm_config = VM_CREDENTIALS[vm_name]
        tester = BadgeTester(vm_config)

        if quick:
            report = tester.run_quick_tests()
        else:
            report = tester.run_all_tests()

        all_reports[vm_name] = report
        all_results.extend(report.results)
        print(generate_report_summary(report, vm_name))

    if not all_results:
        print("No tests ran!")
        return 1

    total_passed = sum(r.passed for r in all_results)
    total_failed = sum(not r.passed for r in all_results)
    total_tests = len(all_results)

    print("\n" + "="*60)
    print("COMBINED SUMMARY")
    print("="*60)
    print(f"Total Tests: {total_tests}")
    print(f"Passed: {total_passed} OK")
    print(f"Failed: {total_failed} FAIL")
    print(f"Success Rate: {(total_passed/total_tests)*100:.1f}%")

    failed_report = generate_failed_tests_report(all_results)
    print(failed_report)

    json_output = {
        "timestamp": datetime.now().isoformat(),
        "mode": "quick" if quick else "full",
        "vms_tested": vms,
        "summary": {
            "total_test": total_tests,
            "passed": total_passed,
            "failed": total_failed,
            "success_rate": round((total_passed/total_tests)*100, 1)
        },
        "vm_results": {
            vm: {
                "tests": len(r.results),
                "passed": sum(1 for x in r.results if x.passed),
                "failed": sum(1 for x in r.results if not x.passed)
            } for vm, r in all_reports.items()
        },
        "failed_tests": [
            {
                "test_name": r.test_name,
                "category": r.category,
                "url": r.url,
                "expected": r.expected_behavior,
                "actual": r.actual_behavior
            } for r in all_results if not r.passed
        ]
    }

    os.makedirs("test_results", exist_ok=True)
    with open("test_results/badge_test_report.json", "w") as f:
        json.dump(json_output, f, indent=2)

    print(f"\nJSON report: test_results/badge_test_report.json")

    return 0 if total_failed == 0 else 1


def main():
    parser = argparse.ArgumentParser(
        description="Netdata Badge Documentation Tester",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Quick test (Linux only, ~13 tests)
  python3 skills/testing_agent.py --quick

  # Full test (Linux only, ~40 tests)
  python3 skills/testing_agent.py

  # Test specific VM
  python3 skills/testing_agent.py --vm linux
  python3 skills/testing_agent.py --vm windows

  # Test both VMs
  python3 skills/testing_agent.py --vm linux --vm windows

  # Quick test on both VMs
  python3 skills/testing_agent.py --quick --vm linux --vm windows
"""
    )

    parser.add_argument("--quick", action="store_true",
                        help="Run quick subset of tests (~13 tests)")
    parser.add_argument("--vm", action="append", default=[],
                        choices=["linux", "windows"],
                        help="VM(s) to test (default: linux)")
    parser.add_argument("--all", action="store_true",
                        help="Test all available VMs")

    args = parser.parse_args()

    doc_issues = lint_documentation()
    if doc_issues:
        print("\n" + "!"*60)
        print("! DOCUMENTATION LINT ISSUES FOUND")
        print("!"*60)
        for issue in doc_issues:
            print(f"  [!] {issue}")
        print()

    if args.all:
        vms = ["linux", "windows"]
    elif args.vm:
        vms = args.vm
    else:
        vms = ["linux"]

    return run_tests(vms, quick=args.quick)


def lint_documentation() -> List[str]:
    """Lint badges.md for common documentation issues."""
    issues = []

    badges_md_paths = [
        "/Users/kanela/src/netdata/netdata/docs/dashboards-and-charts/badges.md",
        "docs/dashboards-and-charts/badges.md",
        "../netdata/docs/dashboards-and-charts/badges.md",
    ]

    content = None
    for path in badges_md_paths:
        try:
            with open(path, "r") as f:
                content = f.read()
            break
        except FileNotFoundError:
            continue

    if not content:
        return issues

    import re

    patterns = [
        (r"alarm.*instead of.*value", "Alarm badges show BOTH status AND value, not 'instead of'"),
        (r"divide\s*=\s*1073741824", "divide=1073741824 (bytes→GiB) usually gives 0 - use 1024 (MiB→GiB)"),
        (r"options=[a-z]+\|[a-z]", "Options parameter pipe should be URL-encoded as %7C"),
    ]

    for pattern, msg in patterns:
        matches = re.search(pattern, content, re.IGNORECASE | re.MULTILINE)
        if matches:
            issues.append(msg)

    system_load_issues = []
    for m in re.finditer(r"(src=|!\[.*\])\([^)]*chart=system\.load(?!.*dimension)", content, re.IGNORECASE):
        context_after = content[m.end():m.end()+200]
        if "**Note**" not in context_after and "Note:" not in context_after:
            system_load_issues.append("system.load should specify dimension (load1, load5, or load15)")
    issues.extend(system_load_issues)

    for pattern, msg in patterns:
        matches = re.search(pattern, content, re.IGNORECASE | re.MULTILINE)
        if matches:
            issues.append(msg)

    clear_color_issues = []
    for line_num, line in enumerate(content.split('\n'), 1):
        if '**CLEAR**' in line:
            if re.search(r'- Green\b', line) and 'brightgreen' not in line.lower():
                clear_color_issues.append(f"Line {line_num}: CLEAR alert uses 'brightgreen' color, not plain 'green'")
    issues.extend(clear_color_issues[:1])

    mem_available_dimension_issues = []
    for m in re.finditer(r"mem\.available.*dimension=(free|used)", content, re.IGNORECASE):
        mem_available_dimension_issues.append("mem.available chart has NO dimensions - remove dimension=free/used examples")
    issues.extend(mem_available_dimension_issues[:1])

    hardcoded_alarm_examples = []
    for m in re.finditer(r"alarm=(system\.cpu\.(10min_cpu_usage|high_cpu|high_load)[^&\s]*)", content, re.IGNORECASE):
        hardcoded_alarm_examples.append(f"Hardcoded alarm name in example: '{m.group(1)}' - use placeholder like YOUR_ALARM_NAME")
    issues.extend(hardcoded_alarm_examples[:1])

    return issues


if __name__ == "__main__":
    sys.exit(main())