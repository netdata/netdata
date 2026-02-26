"""SSH Client for remote command execution"""

import subprocess
from typing import Dict, Any, Optional


class SSHClient:
    """Handle SSH connections to test VM"""

    def __init__(self, host: str, user: str, password: str):
        self.host = host
        self.user = user
        self.password = password

    def execute(self, command: str, capture_output: bool = True, timeout: int = 60) -> Dict[str, Any]:
        """Execute command on remote VM via SSH"""
        try:
            cmd = [
                'sshpass', '-p', self.password,
                'ssh', '-o', 'StrictHostKeyChecking=no',
                f'{self.user}@{self.host}',
                command
            ]

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

    def execute_with_sudo(self, command: str, capture_output: bool = True) -> Dict[str, Any]:
        """Execute command with sudo on remote VM"""
        sudo_command = f"sudo {command}"
        return self.execute(sudo_command, capture_output)

    def file_exists(self, path: str) -> bool:
        """Check if file exists on remote VM"""
        result = self.execute(f"test -e {path} && echo 'exists' || echo 'not_found'")
        return 'exists' in result.get('stdout', '')

    def read_file(self, path: str) -> Optional[str]:
        """Read file content from remote VM"""
        result = self.execute(f"cat {path}")
        if result['success']:
            return result['stdout']
        return None

    def write_file(self, path: str, content: str) -> Dict[str, Any]:
        """Write content to file on remote VM"""
        escaped_content = content.replace("'", "'\\''")
        cmd = f"sudo tee {path} <<'EOF'\n{content}\nEOF"
        return self.execute(cmd)

    def backup_file(self, path: str) -> Optional[str]:
        """Create backup of remote file"""
        import time
        timestamp = int(time.time())
        backup_path = f"{path}.backup.{timestamp}"
        result = self.execute(f"sudo cp {path} {backup_path}")
        if result['success']:
            return backup_path
        return None

    def restore_file(self, backup_path: str, original_path: str) -> bool:
        """Restore file from backup"""
        result = self.execute(f"sudo mv {backup_path} {original_path}")
        return result['success']
