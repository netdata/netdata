"""SSH Client for remote command execution"""

import subprocess
import os
import tempfile
import time
import shlex
from typing import Dict, Any, Optional


class SSHClient:
    """Handle SSH connections to test VM"""

    def __init__(self, host: str, user: str, password: str):
        self.host = host
        self.user = user
        self.password = password
        self._quoted_password = shlex.quote(password) if password else ''
        self._ssh_base = [
            'sshpass', '-p', password if password else '',
            'ssh', '-o', 'StrictHostKeyChecking=no',
            f'{user}@{host}'
        ]

    def _run(self, command: str, capture_output: bool = True, timeout: int = 60) -> Dict[str, Any]:
        """Execute command on remote VM via SSH"""
        cmd = self._ssh_base + [command]

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
        """Execute command with sudo on remote VM using -S flag"""
        sudo_command = f"echo {self._quoted_password} | sudo -S {command}"
        return self._run(sudo_command, capture_output, timeout)

    def file_exists(self, path: str) -> bool:
        """Check if file exists on remote VM"""
        quoted_path = shlex.quote(path)
        result = self._run(f"test -e {quoted_path} && echo 'exists' || echo 'not_found'")
        return 'exists' in result.get('stdout', '')

    def read_file(self, path: str) -> Optional[str]:
        """Read file content from remote VM"""
        quoted_path = shlex.quote(path)
        result = self._run(f"cat {quoted_path}")
        if result['success']:
            return result['stdout']
        return None

    def write_file(self, path: str, content: str) -> Dict[str, Any]:
        """Write content to file on remote VM using sudo"""
        quoted_path = shlex.quote(path)
        escaped_content = content.replace("'", "'\\''")
        cmd = self._ssh_base + [
            f"printf '%s\\n' '{escaped_content}' | sudo -S tee {quoted_path} > /dev/null"
        ]
        
        try:
            result = subprocess.run(
                cmd, capture_output=True, text=True, timeout=30
            )
            return {
                'success': result.returncode == 0,
                'stdout': result.stdout,
                'stderr': result.stderr,
                'returncode': result.returncode
            }
        except Exception as e:
            return {'success': False, 'error': str(e)}

    def backup_file(self, path: str) -> Optional[str]:
        """Create backup of remote file"""
        import time
        quoted_path = shlex.quote(path)
        timestamp = int(time.time())
        backup_path = f"{path}.backup.{timestamp}"
        result = self.sudo(f"cp {quoted_path} {shlex.quote(backup_path)}")
        if result['success']:
            return backup_path
        return None

    def restore_file(self, backup_path: str, original_path: str) -> bool:
        """Restore file from backup"""
        result = self.sudo(f"mv {shlex.quote(backup_path)} {shlex.quote(original_path)}")
        return result['success']
