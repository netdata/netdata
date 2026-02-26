"""
Utility functions for Netdata Documentation Tester.
"""

from dataclasses import dataclass
from typing import Optional, Tuple


@dataclass
class VMConfig:
    """Configuration for a target VM."""
    name: str = "linux"
    ip: str = "10.10.30.140"
    port: int = 19999
    username: str = "cm"
    password: str = "123"
    scheme: str = "http"

    @property
    def base_url(self) -> str:
        return f"{self.scheme}://{self.ip}:{self.port}"

    @property
    def ssh_host(self) -> str:
        return f"{self.username}@{self.ip}"


def run_ssh_command(
    vm_config: VMConfig,
    command: str,
    timeout: int = 60,
    sudo: bool = False
) -> Tuple[bool, str, str]:
    """Execute a command on the target VM via SSH."""
    import subprocess
    
    if sudo:
        command = f"echo '{vm_config.password}' | sudo -S {command}"

    full_cmd = (
        f"sshpass -p '{vm_config.password}' "
        f"ssh -o StrictHostKeyChecking=no -o ConnectTimeout=10 "
        f"{vm_config.ssh_host} '{command}'"
    )

    try:
        result = subprocess.run(
            full_cmd,
            shell=True,
            capture_output=True,
            text=True,
            timeout=timeout
        )
        return result.returncode == 0, result.stdout, result.stderr
    except subprocess.TimeoutExpired:
        return False, "", f"Command timed out after {timeout}s"
    except Exception as e:
        return False, "", str(e)


def make_request(url: str, timeout: int = 10) -> Tuple[bool, int, str]:
    """Make an HTTP request."""
    try:
        import urllib.request
        import urllib.error
        
        req = urllib.request.Request(url)
        with urllib.request.urlopen(req, timeout=timeout) as response:
            return True, response.status, response.read().decode('utf-8')
    except urllib.error.HTTPError as e:
        return False, e.code, str(e)
    except Exception as e:
        return False, 0, str(e)
