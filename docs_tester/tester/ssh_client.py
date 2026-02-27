"""SSH Client for remote command execution"""

import subprocess
from typing import Dict, Any, Optional


class SSHClient:
    """Handle SSH connections to test VM"""

    def __init__(self, host: str, user: str, password: str):
        self.host = host
        self.user = user
        self.password = password

    def _run(self, command: str, capture_output: bool = True, timeout: int = 60) -> Dict[str, Any]:
        """Execute command on remote VM via SSH"""
        cmd = [
            'sshpass', '-p', self.password,
            'ssh', '-o', 'StrictHostKeyChecking=no',
            f'{self.user}@{self.host}',
            command
        ]

        try:
            if capture_output:
                result = subprocess.run(
                    cmd, capture_output=True, text=True, timeout=timeout
                )
                return {
                    'success': result.returncode == 0,
                    'stdout': result.stdout,
                    'stderr': result.stderr,
                    'returncode': result.returncode
                }
            else:
                subprocess.run(cmd, timeout=timeout)
                return {'success': True}
        except Exception as e:
            return {
                'success': False,
                'error': str(e),
                'stdout': '',
                'stderr': str(e)
            }

    def execute(self, command: str, capture_output: bool = True, timeout: int = 60) -> Dict[str, Any]:
        """Execute command on remote VM via SSH"""
        return self._run(command, capture_output, timeout)

    def sudo(self, command: str, capture_output: bool = True, timeout: int = 60) -> Dict[str, Any]:
        """Execute command with sudo on remote VM using bash -c"""
        sudo_cmd = f"bash -c 'echo {self.password} | sudo -S {command}'"
        return self._run(sudo_cmd, capture_output, timeout)

    def file_exists(self, path: str) -> bool:
        """Check if file exists on remote VM"""
        result = self._run(f"test -e {path} && echo 'exists' || echo 'not_found'")
        return 'exists' in result.get('stdout', '')

    def read_file(self, path: str) -> Optional[str]:
        """Read file content from remote VM"""
        result = self._run(f"cat {path}")
        if result['success']:
            return result['stdout']
        return None

    def write_file(self, path: str, content: str) -> Dict[str, Any]:
        """Write content to file on remote VM using sudo"""
        escaped_content = content.replace("'", "'\\''")
        cmd = f"bash -c 'echo {self.password} | sudo -S tee {path} > /dev/null' <<< '{escaped_content}'"
        return self._run(cmd, capture_output=False)

    def backup_file(self, path: str) -> Optional[str]:
        """Create backup of remote file"""
        import time
        timestamp = int(time.time())
        backup_path = f"{path}.backup.{timestamp}"
        result = self.sudo(f"cp {path} {backup_path}")
        if result['success']:
            return backup_path
        return None

    def restore_file(self, backup_path: str, original_path: str) -> bool:
        """Restore file from backup"""
        result = self.sudo(f"mv {backup_path} {original_path}")
        return result['success']
