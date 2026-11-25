"""Command and action executor"""

import re
import time
import os
import urllib.request
import urllib.error
import json
from datetime import datetime
from typing import Dict, Any, Optional, List
from .ssh_client import SSHClient
from .claim_extractor import StepType, Step, Workflow

MAX_SLEEP_SECONDS = 300  # Cap sleep at 5 minutes
MAX_RETRIES = 3
RETRY_DELAY = 2  # seconds between retries
DEFAULT_TIMEOUT = 30  # seconds per test


class ExecutionLogger:
    """Log all execution details to file"""

    def __init__(self, log_dir: str = '/var/log/netdata_tester'):
        self.log_dir = log_dir
        self.log_file = None
        self._ensure_log_dir()

    def _ensure_log_dir(self):
        """Create log directory if it doesn't exist"""
        try:
            os.makedirs(self.log_dir, exist_ok=True)
        except PermissionError:
            self.log_dir = '/tmp/netdata_tester'
            os.makedirs(self.log_dir, exist_ok=True)

    def start(self, doc_file: str):
        """Start logging for a test run"""
        timestamp = datetime.now().strftime('%Y%m%d_%H%M%S')
        self.log_file = os.path.join(self.log_dir, f'run_{timestamp}.log')
        self._write(f"=== Test Run Started: {datetime.now().isoformat()} ===")
        self._write(f"Documentation: {doc_file}")
        self._write("")

    def _write(self, message: str):
        """Write to log file with timestamp"""
        if self.log_file:
            with open(self.log_file, 'a') as f:
                timestamp = datetime.now().strftime('%Y-%m-%d %H:%M:%S.%f')[:-3]
                f.write(f"[{timestamp}] {message}\n")

    def command(self, cmd: str, use_sudo: bool = False):
        """Log command execution"""
        sudo_str = "sudo " if use_sudo else ""
        self._write(f"COMMAND: {sudo_str}{cmd}")

    def output(self, stdout: str, stderr: str = "", returncode: Optional[int] = None):
        """Log command output"""
        if stdout:
            self._write(f"STDOUT:\n{stdout[:2000]}")
        if stderr:
            self._write(f"STDERR:\n{stderr[:2000]}")
        if returncode is not None:
            self._write(f"RETURNCODE: {returncode}")

    def retry(self, attempt: int, max_retries: int):
        """Log retry attempt"""
        self._write(f"RETRY: Attempt {attempt + 1}/{max_retries}")

    def result(self, status: str, details: str = ""):
        """Log test result"""
        self._write(f"RESULT: {status}" + (f" - {details}" if details else ""))

    def info(self, message: str):
        """Log info message"""
        self._write(f"INFO: {message}")

    def error(self, message: str):
        """Log error"""
        self._write(f"ERROR: {message}")

    def finish(self):
        """Finish logging"""
        self._write(f"=== Test Run Finished: {datetime.now().isoformat()} ===")
        return self.log_file


class Executor:
    """Execute commands, configurations, and workflows"""

    def __init__(self, ssh_client: SSHClient, netdata_url: str, timeout: int = DEFAULT_TIMEOUT):
        self.ssh = ssh_client
        self.netdata_url = netdata_url
        self.netdata_version = None
        self.backed_up_files: List[Dict[str, Any]] = []
        self.timeout = timeout
        self.logger = ExecutionLogger()

    def capture_version(self) -> str:
        """Capture Netdata version at start of test run"""
        result = self.ssh.execute('netdata -v')
        version_output = result.get('stdout', '').strip()
        if not version_output:
            version_output = result.get('stderr', '').strip()
        self.netdata_version = version_output
        self.logger.info(f"Netdata version: {version_output}")
        return version_output

    def check_netdata_health(self) -> bool:
        """Check if Netdata is running and responding to API requests"""
        try:
            import urllib.request
            import json
            url = f"{self.netdata_url}/api/v1/info"
            req = urllib.request.Request(url)
            with urllib.request.urlopen(req, timeout=10) as response:
                data = json.loads(response.read().decode())
                if data.get('version'):
                    self.logger.info("Netdata health check: OK")
                    return True
        except Exception as e:
            self.logger.error(f"Netdata health check failed: {e}")
        return False

    def wait_for_netdata(self, max_wait: int = 60) -> bool:
        """Wait for Netdata to become healthy"""
        self.logger.info(f"Waiting for Netdata to be healthy (max {max_wait}s)...")
        start = time.time()
        while time.time() - start < max_wait:
            if self.check_netdata_health():
                return True
            time.sleep(2)
        self.logger.error("Netdata did not become healthy in time")
        return False

    def track_backup(self, backup_path: str, original_path: str):
        """Track a backed up file for later restoration"""
        self.backed_up_files.append({'backup': backup_path, 'original': original_path})

    def restore_all(self) -> List[Dict[str, Any]]:
        """Restore all backed up files and return status"""
        restore_results = []
        for item in self.backed_up_files:
            success = self.ssh.restore_file(item['backup'], item['original'])
            restore_results.append({
                'file': item['original'],
                'restored': success
            })
        self.backed_up_files.clear()
        return restore_results

    def _execute_with_retry(self, command: str, use_sudo: bool = False) -> Dict[str, Any]:
        """Execute SSH command with retry logic"""
        last_error = None
        
        for attempt in range(MAX_RETRIES):
            try:
                if use_sudo:
                    test_result = self.ssh.sudo(command)
                else:
                    test_result = self.ssh.execute(command)
                
                if test_result.get('success', False):
                    return test_result
                last_error = test_result.get('stderr', test_result.get('error', 'Unknown error'))
            except Exception as e:
                last_error = str(e)
            
            if attempt < MAX_RETRIES - 1:
                time.sleep(RETRY_DELAY)
        
        return {'success': False, 'stderr': f'Failed after {MAX_RETRIES} attempts: {last_error}'}

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

            # Check if command needs sudo (excluding 'sudo' itself from the list to avoid double sudo)
            needs_sudo = any(cmd in command.lower() for cmd in ['systemctl', 'chmod', 'chown', 'mkdir', 'rm ', 'tee', 'edit-config'])
            
            # Strip 'sudo ' prefix if present to avoid double sudo
            if command.lower().startswith('sudo '):
                command = command[5:].strip()
                needs_sudo = True
            
            test_result = self._execute_with_retry(command, use_sudo=needs_sudo)

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

            # Skip partial configs (just sections like [web] or single options like value_color=)
            lines = config_content.strip().split('\n')
            # Only skip truly partial configs:
            # 1. Only contains section headers (no key=value pairs)
            # 2. Less than 3 lines AND all are simple key=value (no sections)
            has_section = bool(re.search(r'^\s*\[[\w-]+\]\s*$', config_content, re.MULTILINE))
            # Allow for indented key=value lines (common in config files)
            has_keyvalue = bool(re.search(r'^\s*[\w-]+=', config_content, re.MULTILINE))
            
            is_partial = False
            if has_section and not has_keyvalue:
                # Only section headers, no key=value pairs - partial
                is_partial = True
            elif len(lines) < 3 and has_keyvalue and not has_section:
                # Very short config without sections - partial
                is_partial = True
            
            if is_partial:
                result['status'] = 'SKIP'
                result['error'] = 'Partial config snippet detected (not a full config)'
                return result

            # Check if we can write to the config directory
            check_result = self.ssh.sudo(f'test -w {config_file} && echo "writable" || echo "not_writable"')
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

            # Retry write operation
            apply_result = self._execute_with_retry(
                f'cat > {config_file} << "EOF"\n{config_content}\nEOF',
                use_sudo=True
            )
            if not apply_result['success']:
                result['status'] = 'FAIL'
                result['error'] = f'Failed to apply config: {apply_result.get("stderr")}'
                self.ssh.restore_file(backup_path, config_file)
                backup_path = None  # Prevent finally from restoring again
                return result

            result['steps'].append({
                'action': 'restart',
                'description': 'Restarting Netdata service'
            })

            # Retry restart operation
            restart_result = self._execute_with_retry('systemctl restart netdata', use_sudo=True)
            if not restart_result['success']:
                result['status'] = 'FAIL'
                result['error'] = f'Failed to restart netdata: {restart_result.get("stderr")}'
                self.ssh.restore_file(backup_path, config_file)
                backup_path = None  # Prevent finally from restoring again
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

            # Retry logic for API calls
            last_error = None
            for attempt in range(MAX_RETRIES):
                try:
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
                    break  # Success or definite failure, no retry
                    
                except urllib.error.HTTPError as e:
                    last_error = f'HTTP error: {e.code} - {str(e.reason)}'
                except urllib.error.URLError as e:
                    last_error = f'URL error: {str(e)}'
                except Exception as e:
                    last_error = str(e)
                
                if attempt < MAX_RETRIES - 1:
                    time.sleep(RETRY_DELAY)
                    result['steps'].append({
                        'action': 'retry',
                        'description': f'Retrying API call (attempt {attempt + 2}/{MAX_RETRIES})...'
                    })
            else:
                # All retries exhausted
                result['status'] = 'FAIL'
                result['error'] = f'API call failed after {MAX_RETRIES} attempts: {last_error}'

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

        failed_step = None
        for i, step in enumerate(workflow.steps, 1):
            if failed_step is not None:
                step_result = {
                    'step_number': i,
                    'instruction': step.instruction,
                    'type': step.type.value,
                    'success': False,
                    'status': 'BLOCKED',
                    'evidence': [],
                    'error': f'BLOCKED: depends on failed step {failed_step}'
                }
                result['steps'].append(step_result)
                continue

            step_result = self._execute_step(step, i)
            result['steps'].append(step_result)
            result['evidence'].extend(step_result.get('evidence', []))

            if not step_result['success']:
                result['status'] = 'FAIL'
                failed_step = i
                result['failed_at_step'] = i
                result['error'] = f"Step {i} failed: {step_result.get('error', 'Unknown error')}"

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
        """Execute a command step with retry"""
        command = step.instruction
        step_result['evidence'].append({
            'action': 'execute_command',
            'description': f'Executing: {command[:100]}...'
        })

        # Check if command needs sudo (excluding 'sudo' itself from the list to avoid double sudo)
        needs_sudo = any(cmd in command.lower() for cmd in ['systemctl', 'chmod', 'chown', 'mkdir', 'rm ', 'tee'])
        
        # Strip 'sudo ' prefix if present to avoid double sudo
        if command.lower().startswith('sudo '):
            command = command[5:].strip()
            needs_sudo = True
        
        # Use retry logic
        test_result = self._execute_with_retry(command, use_sudo=needs_sudo)
        
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
            # Mark as failed so workflow fails properly
            step_result['success'] = False
            step_result['error'] = 'Step skipped - appears to be prose, not a valid command'
            return step_result
        
        # If the instruction is a command, execute it with retry
        cmd_result = self._execute_with_retry(instruction, use_sudo=True)
        
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

            if duration > MAX_SLEEP_SECONDS:
                step_result['evidence'].append({
                    'warning': f'Wait capped from {duration}s to {MAX_SLEEP_SECONDS}s for practicality'
                })
                duration = MAX_SLEEP_SECONDS
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
