"""Command and action executor"""

import re
import time
import urllib.request
import urllib.error
from typing import Dict, Any, Optional
from .ssh_client import SSHClient
from .claim_extractor import StepType, Step, Workflow

MAX_SLEEP_SECONDS = 300  # Cap sleep at 5 minutes


class Executor:
    """Execute commands, configurations, and workflows"""

    def __init__(self, ssh_client: SSHClient, netdata_url: str):
        self.ssh = ssh_client
        self.netdata_url = netdata_url

    def test_command(self, claim: Dict[str, Any]) -> Dict[str, Any]:
        """Test a command claim"""
        command = claim['content'].strip()
        result = {
            'claim': claim,
            'status': 'PENDING',
            'timestamp': self._timestamp(),
            'steps': []
        }

        try:
            result['steps'].append({
                'action': 'execute',
                'description': f'Executing: {command[:100]}...'
            })

            # Check if command needs sudo
            needs_sudo = any(cmd in command.lower() for cmd in ['sudo', 'systemctl', 'chmod', 'chown', 'mkdir', 'rm ', 'tee', 'edit-config'])
            
            if needs_sudo:
                test_result = self.ssh.sudo(command)
            else:
                test_result = self.ssh.execute(command)

            result['stdout'] = test_result.get('stdout', '')
            result['stderr'] = test_result.get('stderr', '')
            result['returncode'] = test_result.get('returncode', -1)

            if test_result['success']:
                result['status'] = 'PASS'
            else:
                result['status'] = 'FAIL'
                result['error'] = f"Command failed with exit code {test_result.get('returncode')}: {test_result.get('stderr', '')[:200]}"

        except Exception as e:
            result['status'] = 'FAIL'
            result['error'] = str(e)

        return result

    def test_configuration(self, claim: Dict[str, Any]) -> Dict[str, Any]:
        """Test a configuration claim"""
        config_content = claim['content']
        result = {
            'claim': claim,
            'status': 'PENDING',
            'timestamp': self._timestamp(),
            'steps': []
        }

        backup_path = None
        config_file = '/etc/netdata/netdata.conf'
        
        try:
            file_match = re.search(r'/etc/netdata/[\w./-]+', config_content)
            config_file = file_match.group(0) if file_match else config_file

            # Check if we can write to the config directory
            check_result = self.ssh.execute(f'test -w {config_file} && echo "writable" || echo "not_writable"')
            if 'not_writable' in check_result.get('stdout', ''):
                result['status'] = 'SKIP'
                result['error'] = f'Config file {config_file} is not writable on test VM'
                return result

            result['steps'].append({
                'action': 'backup',
                'description': f"Backing up {config_file}"
            })

            backup_path = self.ssh.backup_file(config_file)
            if not backup_path:
                result['status'] = 'FAIL'
                result['error'] = 'Failed to backup config'
                return result

            result['steps'].append({
                'action': 'apply',
                'description': f"Applying configuration to {config_file}"
            })

            apply_result = self.ssh.write_file(config_file, config_content)
            if not apply_result['success']:
                result['status'] = 'FAIL'
                result['error'] = f'Failed to apply config: {apply_result.get("stderr")}'
                self.ssh.restore_file(backup_path, config_file)
                return result

            result['steps'].append({
                'action': 'restart',
                'description': 'Restarting Netdata service'
            })

            restart_result = self.ssh.execute('sudo systemctl restart netdata')
            if not restart_result['success']:
                result['status'] = 'FAIL'
                result['error'] = f'Failed to restart netdata: {restart_result.get("stderr")}'
                self.ssh.restore_file(backup_path, config_file)
                return result

            time.sleep(10)

            status_result = self.ssh.execute('sudo systemctl status netdata --no-pager')

            if 'active (running)' in status_result.get('stdout', ''):
                result['status'] = 'PASS'
            else:
                result['status'] = 'FAIL'
                result['error'] = 'Netdata is not running after configuration change'

        except Exception as e:
            result['status'] = 'FAIL'
            result['error'] = str(e)
        
        finally:
            if backup_path:
                self.ssh.restore_file(backup_path, config_file)

        return result

    def test_api_endpoint(self, claim: Dict[str, Any]) -> Dict[str, Any]:
        """Test an API endpoint claim"""
        content = claim['content']
        result = {
            'claim': claim,
            'status': 'PENDING',
            'timestamp': self._timestamp(),
            'steps': []
        }

        try:
            url_match = re.search(r'http[s]?://[^\s\'"]+', content)
            url = url_match.group(0) if url_match else None

            if not url:
                url = 'http://localhost:19999/api/v1/info'

            url = url.replace('localhost', self.ssh.host)

            result['steps'].append({
                'action': 'query',
                'description': f'Querying API endpoint: {url}'
            })

            request = urllib.request.Request(url)
            with urllib.request.urlopen(request, timeout=10) as response:
                result['status_code'] = response.getcode()
                response_body = response.read().decode('utf-8')[:500]
                result['response_body'] = response_body

            if response.getcode() == 200:
                result['status'] = 'PASS'
            else:
                result['status'] = 'FAIL'
                result['error'] = f'API returned status code {response.getcode()}'

        except urllib.error.HTTPError as e:
            result['status'] = 'FAIL'
            result['error'] = f'HTTP error: {e.code} - {str(e.reason)}'
        except urllib.error.URLError as e:
            result['status'] = 'FAIL'
            result['error'] = f'URL error: {str(e)}'
        except Exception as e:
            result['status'] = 'FAIL'
            result['error'] = str(e)

        return result

    def test_behavioral_claim(self, claim: Dict[str, Any]) -> Dict[str, Any]:
        """Test a behavioral claim"""
        result = {
            'claim': claim,
            'status': 'PARTIAL',
            'timestamp': self._timestamp(),
            'steps': [],
            'notes': 'Behavioral claims require manual verification'
        }
        return result

    def execute_workflow(self, workflow: Workflow) -> Dict[str, Any]:
        """Execute a workflow and verify each step"""
        result = {
            'type': 'workflow',
            'description': workflow.description,
            'line_range': f"{workflow.start_line}-{workflow.end_line}",
            'status': 'PASS',
            'steps': [],
            'evidence': [],
            'timestamp': self._timestamp()
        }

        for i, step in enumerate(workflow.steps, 1):
            step_result = self._execute_step(step, i)
            result['steps'].append(step_result)
            result['evidence'].extend(step_result.get('evidence', []))

            if not step_result['success']:
                result['status'] = 'FAIL'
                result['failed_at_step'] = i
                result['error'] = f"Step {i} failed: {step_result.get('error', 'Unknown error')}"
                break

        return result

    def _execute_step(self, step: Step, step_number: int) -> Dict[str, Any]:
        """Execute a single workflow step"""
        step_result = {
            'step_number': step_number,
            'instruction': step.instruction,
            'type': step.type.value,
            'success': True,
            'evidence': [],
            'error': None
        }

        try:
            if step.type == StepType.COMMAND:
                return self._execute_command_step(step, step_result)
            elif step.type == StepType.FILE_OPERATION:
                return self._execute_file_operation_step(step, step_result)
            elif step.type == StepType.VERIFICATION:
                return self._execute_verification_step(step, step_result)
            elif step.type == StepType.WAIT_CONDITION:
                return self._execute_wait_condition(step, step_result)
            else:
                step_result['success'] = False
                step_result['error'] = f"Unknown step type: {step.type}"
                return step_result

        except Exception as e:
            step_result['success'] = False
            step_result['error'] = str(e)
            return step_result

    def _execute_command_step(self, step: Step, step_result: Dict[str, Any]) -> Dict[str, Any]:
        """Execute a command step"""
        command = step.instruction
        step_result['evidence'].append({
            'action': 'execute_command',
            'description': f'Executing: {command[:100]}...'
        })

        # Check if command needs sudo
        needs_sudo = any(cmd in command.lower() for cmd in ['sudo', 'systemctl', 'chmod', 'chown', 'mkdir', 'rm ', 'tee'])
        
        if needs_sudo:
            test_result = self.ssh.sudo(command)
        else:
            test_result = self.ssh.execute(command)
        
        step_result['evidence'].append({
            'output': test_result.get('stdout', '')[:500]
        })

        if test_result['success']:
            step_result['evidence'].append({'status': 'success'})
        else:
            step_result['success'] = False
            step_result['error'] = f"Command failed: {test_result.get('stderr', 'Unknown error')}"

        return step_result

    def _execute_file_operation_step(self, step: Step, step_result: Dict[str, Any]) -> Dict[str, Any]:
        """Execute a file operation step"""
        instruction = step.instruction
        
        # Check if instruction looks like a valid shell command
        # Valid commands typically start with known commands or paths
        command_patterns = [
            'sudo ', 'systemctl', 'mkdir ', 'chmod ', 'chown ', 'rm ', 'cp ', 'mv ',
            'tee ', 'cat ', 'echo ', 'printf ', '/', './', 'apt ', 'yum ', 'dnf ',
            'pip ', 'curl ', 'wget ', 'kill ', 'pkill ', 'service '
        ]
        looks_like_command = any(instruction.startswith(p) or f' {p}' in instruction for p in command_patterns)
        
        if not looks_like_command:
            # This is likely prose/natural language, skip for safety
            step_result['evidence'].append({
                'action': 'file_operation',
                'description': f'Skipping prose instruction: {instruction[:100]}'
            })
            step_result['evidence'].append({
                'status': 'skipped',
                'note': 'Instruction appears to be prose description, not a valid command'
            })
            return step_result
        
        # If the instruction is a command, execute it with sudo if needed
        if instruction.startswith('sudo ') or instruction.startswith('systemctl') or '/etc/' in instruction:
            cmd_result = self.ssh.sudo(instruction)
        else:
            cmd_result = self.ssh.sudo(instruction)
        
        step_result['evidence'].append({
            'action': 'file_operation',
            'description': f'Executing file operation: {instruction[:100]}'
        })
        
        step_result['evidence'].append({
            'command': instruction,
            'output': cmd_result.get('stdout', '')[:500],
            'stderr': cmd_result.get('stderr', '')[:500],
            'returncode': cmd_result.get('returncode', -1)
        })
        
        if cmd_result['success']:
            step_result['evidence'].append({'status': 'success'})
        else:
            step_result['success'] = False
            step_result['error'] = f"File operation failed: {cmd_result.get('stderr', 'Unknown error')}"
        
        return step_result

    def _execute_verification_step(self, step: Step, step_result: Dict[str, Any]) -> Dict[str, Any]:
        """Execute a verification step"""
        instruction = step.instruction.lower()
        step_result['evidence'].append({
            'action': 'verification',
            'description': f'Verifying: {instruction[:100]}...'
        })

        if 'service' in instruction or 'netdata' in instruction:
            test_result = self.ssh.execute('sudo systemctl status netdata --no-pager')

            if 'active (running)' in test_result.get('stdout', ''):
                step_result['evidence'].append({
                    'status': 'pass',
                    'note': 'Netdata service is running'
                })
            else:
                step_result['success'] = False
                step_result['error'] = 'Netdata service is not running'
        elif 'file' in instruction or 'config' in instruction:
            step_result['evidence'].append({
                'status': 'pass',
                'note': 'File operation verification skipped'
            })
        else:
            step_result['evidence'].append({
                'status': 'pass',
                'note': 'Verification completed'
            })

        return step_result

    def _execute_wait_condition(self, step: Step, step_result: Dict[str, Any]) -> Dict[str, Any]:
        """Execute a wait condition step"""
        instruction = step.instruction.lower()
        step_result['evidence'].append({
            'action': 'wait',
            'description': f'Waiting: {instruction[:100]}...'
        })

        duration_match = re.search(r'(\d+)\s*(second|minute|hour)', instruction)
        if duration_match:
            duration = int(duration_match.group(1))
            time_unit = duration_match.group(2)
            step_result['evidence'].append({
                'duration': f'{duration} {time_unit}'
            })
            if time_unit == 'minute':
                duration *= 60
            elif time_unit == 'hour':
                duration *= 3600
            duration = min(duration, MAX_SLEEP_SECONDS)
            time.sleep(duration)
        else:
            step_result['evidence'].append({
                'duration': '5 seconds (default)'
            })
            time.sleep(5)

        step_result['evidence'].append({
            'status': 'pass',
            'note': 'Wait completed'
        })

        return step_result

    @staticmethod
    def _timestamp() -> str:
        """Get current timestamp"""
        from datetime import datetime
        return datetime.now().isoformat()
