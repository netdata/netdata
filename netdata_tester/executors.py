"""
Test executors for running documentation claims.
"""

from dataclasses import dataclass, field
from datetime import datetime
from enum import Enum
from typing import Any, Dict, List, Optional

from .parser import ClaimType, TestableClaim
from .managers import ConfigManager
from .utils import VMConfig, run_ssh_command, make_request


class TestStatus(Enum):
    """Status of a test execution."""
    PASS = "PASS"
    FAIL = "FAIL"
    SKIP = "SKIP"
    ERROR = "ERROR"
    PENDING = "PENDING"


@dataclass
class TestResult:
    """Result of executing a test claim."""
    claim: TestableClaim
    status: TestStatus
    message: str = ""
    expected: str = ""
    actual: str = ""
    error_details: str = ""
    duration_ms: float = 0.0
    reproduction_steps: List[str] = field(default_factory=list)


class TestExecutor:
    """Executes test claims against a live Netdata Agent."""

    def __init__(
        self,
        vm_config: VMConfig,
        config_manager: Optional[ConfigManager] = None,
        verbose: bool = False
    ):
        self.vm = vm_config
        self.config_manager = config_manager or ConfigManager(vm_config)
        self.verbose = verbose
        self.results: List[TestResult] = []

    def execute_all(self, claims: List[TestableClaim]) -> List[TestResult]:
        """Execute all test claims."""
        results = []
        for claim in claims:
            result = self.execute_claim(claim)
            results.append(result)
            self.results.append(result)
        return results

    def execute_claim(self, claim: TestableClaim) -> TestResult:
        """Execute a single test claim against the VM."""
        start_time = datetime.now()

        result = TestResult(
            claim=claim,
            status=TestStatus.PENDING,
            expected=claim.expected_behavior or "As documented"
        )

        # Skip documentation-only claims
        if claim.claim_type == ClaimType.SKIP:
            result.status = TestStatus.SKIP
            result.message = "Documentation-only example"
            result.duration_ms = (datetime.now() - start_time).total_seconds() * 1000
            return result

        # Execute based on claim type
        try:
            if claim.claim_type == ClaimType.COMMAND:
                self._execute_command_test(claim, result)
            elif claim.claim_type == ClaimType.API_CALL:
                self._execute_api_test(claim, result)
            elif claim.claim_type == ClaimType.CONFIGURATION:
                self._execute_config_test(claim, result)
            else:
                result.status = TestStatus.SKIP
                result.message = f"Unsupported claim type: {claim.claim_type.value}"
        except Exception as e:
            result.status = TestStatus.ERROR
            result.message = f"Test execution error: {str(e)}"
            result.error_details = str(e)

        result.duration_ms = (datetime.now() - start_time).total_seconds() * 1000
        return result

    def _execute_command_test(self, claim: TestableClaim, result: TestResult):
        """Execute a command test via SSH."""
        if self.verbose:
            print(f"  🔧 Executing: {claim.text[:60]}...")
        
        cmd = claim.text
        success, stdout, stderr = run_ssh_command(self.vm, cmd, timeout=30)
        
        if success:
            result.status = TestStatus.PASS
            result.message = "Command executed successfully"
            result.actual = stdout[:200] if stdout else "No output"
        else:
            result.status = TestStatus.FAIL
            result.message = f"Command failed"
            result.actual = stderr[:200] if stderr else "Unknown error"
            result.reproduction_steps = [f"SSH to {self.vm.ssh_host}", f"Run: {cmd}"]

    def _execute_api_test(self, claim: TestableClaim, result: TestResult):
        """Execute an API test."""
        import re
        
        # Extract URL or use default
        url_match = re.search(r'http[s]?://[^\s\'"]+', claim.text)
        if url_match:
            url = url_match.group(0)
        else:
            url = f"{self.vm.base_url}/api/v1/info"
        
        if self.verbose:
            print(f"  🌐 Testing API: {url}")
        
        success, code, body = make_request(url, timeout=10)
        
        if success and code == 200:
            result.status = TestStatus.PASS
            result.message = f"API returned {code} OK"
            result.actual = f"Response: {len(body)} bytes"
        else:
            result.status = TestStatus.FAIL
            result.message = f"API returned {code}"
            result.actual = body[:200] if body else "No response"
            result.reproduction_steps = [f"curl -i {url}"]

    def _execute_config_test(self, claim: TestableClaim, result: TestResult):
        """Execute a configuration test."""
        if self.verbose:
            print(f"  ⚙️  Testing config: {claim.text[:60]}...")
        
        # Check if config file exists
        cmd = "test -f /etc/netdata/netdata.conf && echo 'EXISTS' || echo 'NOT_FOUND'"
        success, stdout, stderr = run_ssh_command(self.vm, cmd, timeout=10)
        
        if success and 'EXISTS' in stdout:
            result.status = TestStatus.PASS
            result.message = "Configuration file accessible"
            result.actual = "/etc/netdata/netdata.conf exists"
        else:
            result.status = TestStatus.FAIL
            result.message = "Configuration file not found"
            result.actual = stderr or "File not accessible"
            result.reproduction_steps = ["Check /etc/netdata/netdata.conf exists"]

    def get_summary(self) -> Dict[str, Any]:
        """Get summary of test results."""
        return {
            "total": len(self.results),
            "passed": sum(1 for r in self.results if r.status == TestStatus.PASS),
            "failed": sum(1 for r in self.results if r.status == TestStatus.FAIL),
            "skipped": sum(1 for r in self.results if r.status == TestStatus.SKIP),
            "errors": sum(1 for r in self.results if r.status == TestStatus.ERROR),
        }
