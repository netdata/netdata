"""Claim extraction from documentation"""

import re
from typing import List, Dict, Any, Optional
from dataclasses import dataclass, field
from enum import Enum


class StepType(Enum):
    COMMAND = "command"
    FILE_OPERATION = "file_operation"
    VERIFICATION = "verification"
    WAIT_CONDITION = "wait_condition"
    NAVIGATION = "navigation"


@dataclass
class Step:
    type: StepType
    instruction: str
    expected: str
    number: int = 0


@dataclass
class Workflow:
    description: str = ""
    steps: List[Step] = field(default_factory=list)
    start_line: int = 0
    end_line: int = 0


class ClaimExtractor:
    """Extract testable claims from documentation"""

    def parse_file(self, file_path: str) -> List[Dict[str, Any]]:
        """Parse documentation file and extract all testable claims"""
        from pathlib import Path
        content = Path(file_path).read_text()
        return self.parse_content(content)

    def parse_content(self, content: str) -> List[Dict[str, Any]]:
        """Parse documentation content and extract testable claims"""
        claims = []
        
        # First pass: extract all code blocks as individual claims
        code_claims = self._extract_all_code_blocks(content)
        claims.extend(code_claims)
        
        # Second pass: extract workflows with their actual commands
        workflows = self._extract_workflows_with_commands(content)
        for workflow in workflows:
            claims.append({
                'type': 'workflow',
                'line_range': f"{workflow.start_line}-{workflow.end_line}",
                'content': workflow.description,
                'description': workflow.description,
                'steps': workflow.steps
            })
        
        # Third pass: extract behavioral claims
        behavioral_claims = self._extract_behavioral_claims(content)
        claims.extend(behavioral_claims)
        
        return claims

    def _extract_all_code_blocks(self, content: str) -> List[Dict[str, Any]]:
        """Extract all code blocks as testable claims"""
        claims = []
        lines = content.split('\n')
        
        in_code_block = False
        code_type = None
        code_content = []
        code_start_line = 0
        code_end_line = 0
        
        for i, line in enumerate(lines, 1):
            if line.startswith('```'):
                if not in_code_block:
                    in_code_block = True
                    code_type = line[3:].strip() or 'text'
                    code_start_line = i
                    code_content = []
                else:
                    in_code_block = False
                    code_end_line = i
                    if code_type and code_content:
                        claim = self._create_claim_from_code_block(
                            code_type, code_content, code_start_line, code_end_line
                        )
                        if claim:
                            claims.append(claim)
            elif in_code_block:
                code_content.append(line)
        
        return claims

    def _create_claim_from_code_block(
        self, code_type: str, content: List[str], start_line: int, end_line: int
    ) -> Optional[Dict[str, Any]]:
        """Create a claim from a code block"""
        content_str = '\n'.join(content).strip()
        
        if not content_str:
            return None
        
        # Skip if it's just a URL or example output
        if content_str.startswith('http://') or content_str.startswith('https://'):
            return {
                'type': 'url',
                'line_range': f"{start_line}-{end_line}",
                'content': content_str,
                'description': f"URL reference at lines {start_line}-{end_line}"
            }
        
        if code_type in ['bash', 'sh', 'shell', 'bash+wget', 'bash+curl']:
            # Check if it's a command
            if any(cmd in content_str.lower() for cmd in ['sudo', 'systemctl', 'curl', 'wget', 'echo', 'cat', 'tee', 'mkdir', 'chmod', 'chown', 'apt', 'yum', 'dnf', 'pip']):
                return {
                    'type': 'command',
                    'line_range': f"{start_line}-{end_line}",
                    'content': content_str,
                    'description': f"Command to execute at lines {start_line}-{end_line}"
                }
            return None
            
        elif code_type in ['yaml', 'yml', 'conf', 'ini', 'text']:
            # Check if it's a config file example (INI [section], key=value, or YAML key: value)
            # Note: We don't skip blocks containing URLs here - a config may legitimately have URL values

            is_ini = re.search(r'^\[', content_str, re.MULTILINE)
            is_keyvalue = '=' in content_str
            is_yaml = re.search(r'^\s*[\w-]+:\s+\S', content_str, re.MULTILINE)

            # For text blocks, require multiple YAML-like lines to reduce false positives
            # from things like "Step 1: do this" or "Answer: something"
            if code_type == 'text':
                yaml_matches = re.findall(r'^\s*[\w-]+:\s+\S', content_str, re.MULTILINE)
                is_yaml = len(yaml_matches) >= 2

            if is_ini or is_keyvalue or is_yaml:
                # Extract file path from nearby text
                file_match = re.search(r'(?:file|path|edit|create|add to)\s+[`"]?([/\w.-]+)[`"]?', content_str, re.IGNORECASE)
                file_path = file_match.group(1) if file_match else None
                
                return {
                    'type': 'configuration',
                    'line_range': f"{start_line}-{end_line}",
                    'content': content_str,
                    'file_path': file_path,
                    'description': f"Configuration at lines {start_line}-{end_line}"
                }
            return None
        
        elif code_type == 'http' or 'curl' in content_str:
            return {
                'type': 'api',
                'line_range': f"{start_line}-{end_line}",
                'content': content_str,
                'description': f"API endpoint at lines {start_line}-{end_line}"
            }
        
        return None

    def _extract_workflows_with_commands(self, content: str) -> List[Workflow]:
        """Extract workflows and associate code blocks with steps"""
        workflows = []
        lines = content.split('\n')
        
        # Find all code blocks first
        code_blocks = []
        in_block = False
        block_start = 0
        block_type = None
        block_content = []
        
        for i, line in enumerate(lines, 1):
            if line.startswith('```'):
                if not in_block:
                    in_block = True
                    block_start = i
                    block_type = line[3:].strip()
                    block_content = []
                else:
                    in_block = False
                    if block_content and block_type in ['bash', 'sh', 'shell']:
                        code_blocks.append({
                            'start': block_start,
                            'end': i,
                            'type': block_type,
                            'content': '\n'.join(block_content).strip()
                        })
                    block_content = []
            elif in_block:
                block_content.append(line)
        
        # Now find workflow sections and match code blocks to steps
        in_workflow = False
        workflow_start = 0
        current_steps = []
        step_num = 0
        
        for i, line in enumerate(lines, 1):
            stripped = line.strip()
            
            # Detect workflow start (numbered list or step markers)
            is_numbered = re.match(r'^\d+[.)]\s+', stripped)
            is_step_marker = re.match(r'^(?:first|next|then|finally|step|to|edit|run|execute|restart|stop|start|add|create|remove|delete|configure)\b', stripped, re.IGNORECASE)
            
            if is_numbered or is_step_marker:
                # Set workflow_start when first step is detected
                if not in_workflow or workflow_start == 0:
                    in_workflow = True
                    workflow_start = i
                
                # Find associated code block
                associated_code = None
                for block in code_blocks:
                    if block['start'] > i and block['start'] - i < 20:  # Within 20 lines
                        associated_code = block['content']
                        break
                
                # Skip this step if there's no associated code block - it's just prose
                if not associated_code:
                    continue
                
                step_text = re.sub(r'^\d+[.)]\s+', '', stripped)
                
                step_type = self._classify_step_type(associated_code)
                
                step_num += 1
                current_steps.append(Step(
                    type=step_type,
                    instruction=associated_code,
                    expected="Step completes successfully",
                    number=step_num
                ))
            
            elif in_workflow and (stripped.startswith('##') or stripped == ''):
                # End of workflow
                if current_steps:
                    workflows.append(Workflow(
                        description=f"Workflow at lines {workflow_start}-{i-1}",
                        steps=current_steps,
                        start_line=workflow_start,
                        end_line=i-1
                    ))
                in_workflow = False
                current_steps = []
                step_num = 0
        
        # Handle last workflow
        if current_steps:
            workflows.append(Workflow(
                description=f"Workflow at lines {workflow_start}-{len(lines)}",
                steps=current_steps,
                start_line=workflow_start,
                end_line=len(lines)
            ))
        
        return workflows

    def _extract_behavioral_claims(self, content: str) -> List[Dict[str, Any]]:
        """Extract behavioral claims from text"""
        claims = []
        lines = content.split('\n')
        
        behavioral_patterns = [
            r'(?:will|should|must|can|may)\s+(?:enable|trigger|alert|show|display|create|start|stop|appear|make|allow)',
            r'(?:enables|triggers|alerts|shows|displays|creates|starts|stops|appears)\s+',
            r'(?:results? in|leads to|causes)\s+',
        ]
        
        for i, line in enumerate(lines, 1):
            for pattern in behavioral_patterns:
                if re.search(pattern, line, re.IGNORECASE):
                    # Check if this line is near a code block - if so, skip (it's already covered)
                    claims.append({
                        'type': 'behavioral',
                        'line_range': str(i),
                        'content': line.strip(),
                        'description': f"Behavioral claim at line {i}"
                    })
                    break
        
        return claims

    def _is_section_header(self, line: str) -> bool:
        """Check if line is a section header"""
        return line.startswith('##') or line.startswith('#')

    def _extract_numbered_step(self, line: str) -> Optional[str]:
        """Extract step from numbered list (1., 2., 3.)"""
        patterns = [
            r'^\d+\.\s+(.+)',
            r'^\d+\)\s+(.+)',
        ]

        for pattern in patterns:
            match = re.match(pattern, line)
            if match:
                return match.group(1).strip()

        return None

    def _extract_step_marker(self, line: str) -> Optional[str]:
        """Extract step from markers like 'First:', 'Next:', 'Finally:'"""
        patterns = [
            r'(?i)^(first|next|then|finally|step \d+):\s*(.+)',
            r'(?i)^(to|edit|run|execute|restart|stop|start|add|create|remove|delete|configure)\s+(.+)',
        ]

        for pattern in patterns:
            match = re.match(pattern, line)
            if match:
                return line.strip()

        return None

    def _classify_step_type(self, step_text: str) -> StepType:
        """Classify step as command, file operation, verification, or wait"""
        step_lower = step_text.lower()
        
        file_ops = ['create', 'edit', 'delete', 'modify', 'add', 'remove', 'write', 'copy', 'move', 'configure', 'install']
        if any(op in step_lower for op in file_ops):
            return StepType.FILE_OPERATION
        
        verify_keywords = ['verify', 'check', 'confirm', 'test', 'validate', 'ensure', 'make sure', 'ensure that']
        if any(keyword in step_lower for keyword in verify_keywords):
            return StepType.VERIFICATION
        
        wait_keywords = ['wait', 'pause', 'sleep', 'after', 'once', 'until', 'give it']
        if any(keyword in step_lower for keyword in wait_keywords):
            return StepType.WAIT_CONDITION
        
        return StepType.COMMAND
