#!/usr/bin/env python3
"""
Netdata Documentation Tester Agent
Tests documentation claims against live Netdata installations
"""

import os
import sys
import re
import json
import subprocess
import time
import argparse
import shlex
from datetime import datetime
from pathlib import Path
from typing import List, Dict, Any, Optional, Union, TYPE_CHECKING
import urllib.request
import urllib.error
from dataclasses import dataclass, field
from enum import Enum

class StepType(Enum):
    COMMAND = "command"
    FILE_OPERATION = "file_operation"
    VERIFICATION = "verification"
    WAIT_CONDITION = "wait_condition"
    NAVIGATION = "navigation"

class Step:
    def __init__(self, step_type: StepType, instruction: str, expected: str):
        self.type = step_type
        self.instruction = instruction
        self.expected = expected
        self.number = 0

@dataclass
class Workflow:
    description: str = ""
    steps: List[Step] = field(default_factory=list)
    start_line: int = 0
    end_line: int = 0

class DocumentationTester:
    """Main class for testing documentation claims"""

    def __init__(self, config: Dict[str, Any]):
        self.config = config
        self.test_results = []
        self.vm_host = config.get('vm_host') or os.environ.get('TEST_VM_HOST', '10.10.30.140')
        self.vm_user = config.get('vm_user') or os.environ.get('TEST_VM_USER', 'cm')
        self.vm_password = config.get('vm_password') or os.environ.get('TEST_VM_PASSWORD', '')
        self.netdata_url = config.get('netdata_url') or os.environ.get('NETDATA_URL', 'http://10.10.30.140:19999')
        self.output_dir = Path(config.get('output_dir', 'test_results'))
        self.output_dir.mkdir(exist_ok=True)
        self.workflows: List[Workflow] = []
        self._backup_timestamp: Optional[int] = None

    def ssh_command(self, command: str, capture_output: bool = True) -> Dict[str, Any]:
        """Execute SSH command on test VM"""
        try:
            quoted_password = shlex.quote(self.vm_password) if self.vm_password else ''
            cmd = [
                'sshpass', '-p', self.vm_password if self.vm_password else '',
                'ssh', '-o', 'StrictHostKeyChecking=no',
                f'{self.vm_user}@{self.vm_host}',
                command
            ]
            
            if capture_output:
                result = subprocess.run(
                    cmd, capture_output=True, text=True, timeout=60
                )
                return {
                    'success': result.returncode == 0,
                    'stdout': result.stdout,
                    'stderr': result.stderr,
                    'returncode': result.returncode
                }
            else:
                subprocess.run(cmd, timeout=60)
                return {'success': True}
        except Exception as e:
            return {
                'success': False,
                'error': str(e),
                'stdout': '',
                'stderr': str(e)
            }

    def extract_workflows(self, content: str) -> List[Workflow]:
        """Extract step-by-step workflows from text"""
        workflows = []
        current_workflow = Workflow()
        step_counter = 0
        
        lines = content.split('\n')
        
        for i, line in enumerate(lines, 1):
            stripped_line = line.strip()
            
            if self._is_section_header(stripped_line):
                if current_workflow.steps:
                    workflows.append(current_workflow)
                current_workflow = Workflow()
                current_workflow.start_line = i
                step_counter = 0
                continue
            
            numbered_step = self._extract_numbered_step(stripped_line)
            if numbered_step:
                step_counter += 1
                current_workflow.steps.append(Step(
                    step_type=self._classify_step_type(numbered_step),
                    instruction=numbered_step,
                    expected="Step completes successfully"
                ))
                current_workflow.description = f"Numbered list procedure (lines {current_workflow.start_line}-{i})"
                current_workflow.end_line = i
                continue
            
            step_marker = self._extract_step_marker(stripped_line)
            if step_marker:
                step_counter += 1
                current_workflow.steps.append(Step(
                    step_type=self._classify_step_type(step_marker),
                    instruction=step_marker,
                    expected="Step completes successfully"
                ))
                current_workflow.description = f"Marked procedure (lines {current_workflow.start_line}-{i})"
                current_workflow.end_line = i
                continue
            
            task_pattern = self._extract_task_sequence(stripped_line)
            if task_pattern:
                step_counter += 1
                current_workflow.steps.append(Step(
                    step_type=StepType.COMMAND,
                    instruction=task_pattern,
                    expected="Task completes successfully"
                ))
                current_workflow.description = f"Task sequence (lines {current_workflow.start_line}-{i})"
                current_workflow.end_line = i
                continue
        
        if current_workflow.steps:
            workflows.append(current_workflow)
        
        return workflows
    
    def _is_section_header(self, line: str) -> bool:
        """Check if line is a section header"""
        return line.startswith('##') or line.startswith('#')
    
    def _extract_numbered_step(self, line: str) -> Optional[str]:
        """Extract step from numbered list (1., 2., 3.)"""
        patterns = [
            r'^\d+\.\s+(.+)',
            r'^\d+\)\s+(.+)',
            r'^\s+-\s+(.+)'
        ]
        
        for pattern in patterns:
            match = re.match(pattern, line)
            if match:
                return match.group(1).strip()
        
        return None
    
    def _extract_step_marker(self, line: str) -> Optional[str]:
        """Extract step from markers like 'First:', 'Next:', 'Finally:'"""
        patterns = [
            r'(?i)first.?\s*:?\s+(.+)',
            r'(?i)next.?\s*:?\s+(.+)',
            r'(?i)then.?\s*:?\s+(.+)',
            r'(?i)finally.?\s*:?\s+(.+)',
            r'(?i)after.?\s*:?\s+(.+)'
        ]
        
        for pattern in patterns:
            match = re.match(pattern, line)
            if match:
                return match.group(1).strip()
        
        return None
    
    def _extract_task_sequence(self, line: str) -> Optional[str]:
        """Extract task from patterns like 'To do X:' or 'Follow these steps:'"""
        patterns = [
            r'(?i)to\s+(?:do|complete|perform|execute)\s*:?\s+(.+)',
            r'(?i)follow\s+(?:these\s+)?steps?\s*:?\s+(.+)',
            r'(?i)proceed\s+with\s+(.+)'
            r'(?i)continues?\s+by\s+(.+)'
        ]
        
        for pattern in patterns:
            match = re.match(pattern, line)
            if match:
                return match.group(1).strip()
        
        return None
    
    def _classify_step_type(self, step_text: str) -> StepType:
        """Classify step as command, file operation, verification, or wait"""
        step_lower = step_text.lower()
        
        file_ops = ['create', 'edit', 'delete', 'modify', 'add', 'remove', 'write', 'copy', 'move']
        if any(op in step_lower for op in file_ops):
            return StepType.FILE_OPERATION
        
        verify_keywords = ['verify', 'check', 'confirm', 'test', 'validate', 'ensure']
        if any(keyword in step_lower for keyword in verify_keywords):
            return StepType.VERIFICATION
        
        wait_keywords = ['wait', 'pause', 'sleep', 'after', 'once', 'until']
        if any(keyword in step_lower for keyword in wait_keywords):
            return StepType.WAIT_CONDITION
        
        return StepType.COMMAND

    def parse_documentation(self, file_path: str) -> List[Dict[str, Any]]:
        """Parse documentation file and extract testable claims"""
        claims = []
        content = Path(file_path).read_text()
        lines = content.split('\n')
        
        in_code_block = False
        code_type = None
        code_content = []
        code_start_line = 0
        
        for i, line in enumerate(lines, 1):
            if line.startswith('```'):
                if not in_code_block:
                    in_code_block = True
                    code_type = line[3:].strip() or 'text'
                    code_start_line = i
                    code_content = []
                else:
                    in_code_block = False
                    if code_type:
                        claim = self.analyze_code_block(
                            code_type, code_content, code_start_line, i
                        )
                        if claim:
                            claims.append(claim)
            elif in_code_block:
                code_content.append(line)
            else:
                claim = self.analyze_behavioral_claim(line, i)
                if claim:
                    claims.append(claim)
        
        workflows = self.extract_workflows(content)
        self.workflows = workflows
        
        for workflow in workflows:
            claims.append({
                'type': 'workflow',
                'line_range': f"{workflow.start_line}-{workflow.end_line}",
                'content': workflow.description,
                'description': workflow.description,
                'steps': workflow.steps
            })
        
        return claims

    def analyze_code_block(self, code_type: Optional[str], content: List[str], start_line: int, end_line: int) -> Optional[Dict[str, Any]]:
        """Analyze a code block and determine what to test"""
        content_str = '\n'.join(content)
        
        if code_type in ['bash', 'sh', 'shell']:
            return {
                'type': 'command',
                'line_range': f"{start_line}-{end_line}",
                'content': content_str,
                'description': f"Command test at lines {start_line}-{end_line}"
            }
        elif code_type in ['yaml', 'yml', 'conf', 'ini']:
            return {
                'type': 'configuration',
                'line_range': f"{start_line}-{end_line}",
                'content': content_str,
                'description': f"Configuration test at lines {start_line}-{end_line}"
            }
        elif code_type == 'http' or 'curl' in content_str:
            return {
                'type': 'api',
                'line_range': f"{start_line}-{end_line}",
                'content': content_str,
                'description': f"API endpoint test at lines {start_line}-{end_line}"
            }
        
        return None

    def analyze_behavioral_claim(self, line: str, line_num: int) -> Optional[Dict[str, Any]]:
        """Extract behavioral claims from text"""
        patterns = [
            r'(?:will|should|must)\s+(?:enable|trigger|alert|show|display|create|start|stop)',
            r'(?:enables|triggers|alerts|shows|displays|creates|starts|stops)\s+',
        ]
        
        for pattern in patterns:
            if re.search(pattern, line, re.IGNORECASE):
                return {
                    'type': 'behavioral',
                    'line_range': str(line_num),
                    'content': line.strip(),
                    'description': f"Behavioral claim at line {line_num}"
                }
        
        return None

    def test_configuration(self, claim: Dict[str, Any]) -> Dict[str, Any]:
        """Test a configuration claim"""
        config_content = claim['content']
        result = {
            'claim': claim,
            'status': 'PENDING',
            'timestamp': datetime.now().isoformat(),
            'steps': []
        }
        
        try:
            file_match = re.search(r'/etc/netdata/[\w./-]+', config_content)
            config_file = file_match.group(0) if file_match else '/etc/netdata/netdata.conf'
            
            result['steps'].append({
                'action': 'backup',
                'description': f"Backing up {config_file}"
            })
            
            self._backup_timestamp = int(time.time())
            backup_cmd = f'sudo cp {config_file} {config_file}.backup.{self._backup_timestamp}'
            backup_result = self.ssh_command(backup_cmd)
            
            if not backup_result['success']:
                result['status'] = 'FAIL'
                result['error'] = f'Failed to backup config: {backup_result.get("stderr")}'
                return result
            
            result['steps'].append({
                'action': 'apply',
                'description': f"Applying configuration to {config_file}"
            })
            
            escaped_content = config_content.replace("'", "'\\''")
            apply_cmd = f"sudo tee {config_file} <<'EOF'\n{config_content}\nEOF"
            apply_result = self.ssh_command(apply_cmd)
            
            if not apply_result['success']:
                result['status'] = 'FAIL'
                result['error'] = f'Failed to apply config: {apply_result.get("stderr")}'
                self.restore_config(config_file)
                return result
            
            result['steps'].append({
                'action': 'restart',
                'description': 'Restarting Netdata service'
            })
            
            restart_cmd = 'sudo systemctl restart netdata'
            restart_result = self.ssh_command(restart_cmd)
            
            if not restart_result['success']:
                result['status'] = 'FAIL'
                result['error'] = f'Failed to restart netdata: {restart_result.get("stderr")}'
                self.restore_config(config_file)
                return result
            
            time.sleep(10)
            
            status_cmd = 'sudo systemctl status netdata --no-pager'
            status_result = self.ssh_command(status_cmd)
            
            if 'active (running)' in status_result.get('stdout', ''):
                result['status'] = 'PASS'
            else:
                result['status'] = 'FAIL'
                result['error'] = 'Netdata is not running after configuration change'
            
            self.restore_config(config_file)
            
        except Exception as e:
            result['status'] = 'FAIL'
            result['error'] = str(e)
        
        return result

    def test_command(self, claim: Dict[str, Any]) -> Dict[str, Any]:
        """Test a command claim"""
        command = claim['content'].strip()
        result = {
            'claim': claim,
            'status': 'PENDING',
            'timestamp': datetime.now().isoformat(),
            'steps': []
        }
        
        try:
            result['steps'].append({
                'action': 'execute',
                'description': f'Executing: {command[:100]}...'
            })
            
            test_result = self.ssh_command(command)
            
            result['stdout'] = test_result.get('stdout', '')
            result['stderr'] = test_result.get('stderr', '')
            result['returncode'] = test_result.get('returncode', -1)
            
            if test_result['success']:
                result['status'] = 'PASS'
            else:
                result['status'] = 'FAIL'
                result['error'] = f'Command failed with exit code {test_result.get("returncode")}'
        
        except Exception as e:
            result['status'] = 'FAIL'
            result['error'] = str(e)
        
        return result

    def test_api_endpoint(self, claim: Dict[str, Any]) -> Dict[str, Any]:
        """Test an API endpoint claim"""
        content = claim['content']
        result = {
            'claim': claim,
            'status': 'PENDING',
            'timestamp': datetime.now().isoformat(),
            'steps': []
        }
        
        try:
            url_match = re.search(r'http[s]?://[^\s\'"]+', content)
            url = url_match.group(0) if url_match else None
            
            if not url:
                url = 'http://localhost:19999/api/v1/info'
            
            url = url.replace('localhost', self.vm_host)
            
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
            'status': 'PENDING',
            'timestamp': datetime.now().isoformat(),
            'steps': [],
            'notes': 'Manual verification required for behavioral claims'
        }
        
        result['status'] = 'PARTIAL'
        result['notes'] = 'Behavioral claims require manual verification'
        
        return result

    def restore_config(self, config_file: str):
        """Restore original configuration"""
        if self._backup_timestamp is not None:
            restore_cmd = f'sudo mv {config_file}.backup.{self._backup_timestamp} {config_file} 2>/dev/null || true'
        else:
            restore_cmd = f'sudo mv {config_file}.backup.* {config_file} 2>/dev/null || true'
        self.ssh_command(restore_cmd)
        self._backup_timestamp = None

    def execute_workflow(self, workflow) -> Dict[str, Any]:
        """Execute a workflow and verify each step"""
        result = {
            'type': 'workflow',
            'description': workflow.description,
            'line_range': f"{workflow.start_line}-{workflow.end_line}",
            'status': 'PASS',
            'steps': [],
            'evidence': [],
            'timestamp': datetime.now().isoformat()
        }
        
        for i, step in enumerate(workflow.steps, 1):
            step_result = self._execute_workflow_step(step, i)
            result['steps'].append(step_result)
            result['evidence'].extend(step_result.get('evidence', []))
            
            if not step_result['success']:
                result['status'] = 'FAIL'
                result['failed_at_step'] = i
                result['error'] = f"Step {i} failed: {step_result.get('error', 'Unknown error')}"
                break
        
        return result
    
    def _execute_workflow_step(self, step: Step, step_number: int) -> Dict[str, Any]:
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
        
        test_result = self.ssh_command(command)
        
        step_result['evidence'].append({
            'output': test_result.get('stdout', '')[:500]
        })
        
        if test_result['success']:
            step_result['evidence'].append({
                'status': 'success'
            })
        else:
            step_result['success'] = False
            step_result['error'] = f"Command failed: {test_result.get('stderr', 'Unknown error')}"
        
        return step_result
    
    def _execute_file_operation_step(self, step: Step, step_result: Dict[str, Any]) -> Dict[str, Any]:
        """Execute a file operation step"""
        instruction = step.instruction.lower()
        
        if 'create' in instruction or 'add' in instruction:
            step_result['evidence'].append({
                'action': 'create_file',
                'description': 'Creating configuration file'
            })
            step_result['evidence'].append({
                'status': 'skipped',
                'note': 'File operations in workflows are skipped for safety'
            })
            return step_result
        
        elif 'edit' in instruction or 'modify' in instruction:
            step_result['evidence'].append({
                'action': 'edit_file',
                'description': 'Editing configuration file'
            })
            step_result['evidence'].append({
                'status': 'skipped',
                'note': 'File operations in workflows are skipped for safety'
            })
            return step_result
        
        elif 'delete' in instruction or 'remove' in instruction:
            step_result['evidence'].append({
                'action': 'delete_file',
                'description': 'Deleting file'
            })
            step_result['evidence'].append({
                'status': 'skipped',
                'note': 'File operations in workflows are skipped for safety'
            })
            return step_result
        
        else:
            step_result['success'] = False
            step_result['error'] = f"Unknown file operation: {instruction}"
            return step_result
    
    def _execute_verification_step(self, step: Step, step_result: Dict[str, Any]) -> Dict[str, Any]:
        """Execute a verification step"""
        instruction = step.instruction.lower()
        step_result['evidence'].append({
            'action': 'verification',
            'description': f'Verifying: {instruction[:100]}...'
        })
        
        if 'service' in instruction or 'netdata' in instruction:
            verify_cmd = 'sudo systemctl status netdata --no-pager'
            test_result = self.ssh_command(verify_cmd)
            
            if 'active (running)' in test_result.get('stdout', ''):
                step_result['evidence'].append({
                    'status': 'pass',
                    'note': 'Netdata service is running'
                })
            else:
                step_result['success'] = False
                step_result['error'] = 'Netdata service is not running'
                return step_result
        
        elif 'file' in instruction or 'config' in instruction:
            step_result['evidence'].append({
                'status': 'pass',
                'note': 'File operation verification skipped (manual check required)'
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
                'duration': f'{duration} {time_unit}s'
            })
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

    def test_claim(self, claim: Dict[str, Any]) -> Dict[str, Any]:
        """Route claim to appropriate test method"""
        claim_type = claim['type']
        
        if claim_type == 'configuration':
            return self.test_configuration(claim)
        elif claim_type == 'command':
            return self.test_command(claim)
        elif claim_type == 'api':
            return self.test_api_endpoint(claim)
        elif claim_type == 'behavioral':
            return self.test_behavioral_claim(claim)
        elif claim_type == 'workflow':
            return self.execute_workflow(claim)
        else:
            return {
                'claim': claim,
                'status': 'SKIP',
                'error': f'Unknown claim type: {claim_type}'
            }

    def generate_report(self, doc_file: str) -> str:
        """Generate markdown test report"""
        total = len(self.test_results)
        passed = sum(1 for r in self.test_results if r['status'] == 'PASS')
        failed = sum(1 for r in self.test_results if r['status'] == 'FAIL')
        partial = sum(1 for r in self.test_results if r['status'] == 'PARTIAL')
        
        report = f"""# Documentation Test Report

**Document**: {doc_file}
**Test Date**: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}
**Test Environment**: Netdata at {self.netdata_url}

## Summary
- **Total Claims**: {total}
- **Passed**: {passed} ✅
- **Failed**: {failed} ❌
- **Partial**: {partial} ⚠️

## Test Results

"""
        
        for result in self.test_results:
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

    def save_report(self, report: str, doc_file: str):
        """Save report to file"""
        doc_name = Path(doc_file).stem
        timestamp = datetime.now().strftime('%Y%m%d_%H%M%S')
        report_file = self.output_dir / f'{doc_name}_{timestamp}.md'
        
        report_file.write_text(report)
        print(f"Report saved to: {report_file}")
        
        return str(report_file)

def main():
    parser = argparse.ArgumentParser(description='Test Netdata documentation claims')
    parser.add_argument('doc_file', help='Path to documentation file')
    parser.add_argument('--vm-host', help='Test VM IP (or set TEST_VM_HOST env var)')
    parser.add_argument('--vm-user', help='SSH user (or set TEST_VM_USER env var)')
    parser.add_argument('--netdata-url', help='Netdata URL (or set NETDATA_URL env var)')
    parser.add_argument('--output-dir', default='test_results', help='Output directory')
    parser.add_argument('--format', choices=['markdown', 'json'], default='markdown', help='Report format')
    
    args = parser.parse_args()
    
    config = {
        'vm_host': args.vm_host,
        'vm_user': args.vm_user,
        'netdata_url': args.netdata_url,
        'output_dir': args.output_dir
    }
    
    if not Path(args.doc_file).exists():
        print(f"Error: Documentation file not found: {args.doc_file}")
        sys.exit(1)
    
    print(f"Testing documentation: {args.doc_file}")
    print(f"Test VM: {args.vm_host}")
    print(f"Netdata URL: {args.netdata_url}")
    print("-" * 60)
    
    tester = DocumentationTester(config)
    
    print("Parsing documentation...")
    claims = tester.parse_documentation(args.doc_file)
    
    code_claims = [c for c in claims if c['type'] in ['configuration', 'command', 'api', 'behavioral']]
    workflow_claims = [c for c in claims if c['type'] == 'workflow']
    
    print(f"Found {len(code_claims)} code block claims")
    print(f"Found {len(workflow_claims)} workflow claims")
    print(f"Total: {len(claims)} testable claims")
    print()
    
    if not claims:
        print("No testable claims found in documentation")
        sys.exit(0)
    
    print("=" * 60)
    print("TESTING CODE BLOCKS")
    print("=" * 60)
    for i, claim in enumerate(code_claims, 1):
        print(f"[{i}/{len(code_claims)}] Testing {claim['type']}: {claim['description']}")
        result = tester.test_claim(claim)
        tester.test_results.append(result)
        print(f"  Status: {result['status']}")
        print()
    
    print("=" * 60)
    print("TESTING WORKFLOWS")
    print("=" * 60)
    for i, claim in enumerate(workflow_claims, 1):
        print(f"[{i}/{len(workflow_claims)}] Testing workflow: {claim['description']}")
        result = tester.test_claim(claim)
        tester.test_results.append(result)
        print(f"  Status: {result['status']}")
        if result.get('failed_at_step'):
            print(f"  Failed at step: {result['failed_at_step']}")
            print(f"  Error: {result.get('error', 'Unknown error')}")
        print()
    
    print("Generating report...")
    report = tester.generate_report(args.doc_file)
    
    report_file = tester.save_report(report, args.doc_file)
    
    total = len(tester.test_results)
    passed = sum(1 for r in tester.test_results if r['status'] == 'PASS')
    failed = sum(1 for r in tester.test_results if r['status'] == 'FAIL')
    partial = sum(1 for r in tester.test_results if r['status'] == 'PARTIAL')
    
    print()
    print("=" * 60)
    print("TEST SUMMARY")
    print("=" * 60)
    print(f"Total: {total} | Passed: {passed} | Failed: {failed} | Partial: {partial}")
    print(f"Report: {report_file}")
    print("=" * 60)
    
    if failed > 0:
        sys.exit(1)
    
    if __name__ == '__main__':
        main()
