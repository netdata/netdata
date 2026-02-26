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
                        claim = self._analyze_code_block(
                            code_type, code_content, code_start_line, i
                        )
                        if claim:
                            claims.append(claim)
            elif in_code_block:
                code_content.append(line)
            else:
                claim = self._extract_behavioral_claim(line, i)
                if claim:
                    claims.append(claim)

        workflows = self._extract_workflows(content)
        for workflow in workflows:
            claims.append({
                'type': 'workflow',
                'line_range': f"{workflow.start_line}-{workflow.end_line}",
                'content': workflow.description,
                'description': workflow.description,
                'steps': workflow.steps
            })

        return claims

    def _analyze_code_block(
        self, code_type: str, content: List[str], start_line: int, end_line: int
    ) -> Optional[Dict[str, Any]]:
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

    def _extract_behavioral_claim(self, line: str, line_num: int) -> Optional[Dict[str, Any]]:
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

    def _extract_workflows(self, content: str) -> List[Workflow]:
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
                    type=self._classify_step_type(numbered_step),
                    instruction=numbered_step,
                    expected="Step completes successfully",
                    number=step_counter
                ))
                current_workflow.description = f"Numbered list procedure (lines {current_workflow.start_line}-{i})"
                current_workflow.end_line = i
                continue

            step_marker = self._extract_step_marker(stripped_line)
            if step_marker:
                step_counter += 1
                current_workflow.steps.append(Step(
                    type=self._classify_step_type(step_marker),
                    instruction=step_marker,
                    expected="Step completes successfully",
                    number=step_counter
                ))
                current_workflow.description = f"Marked procedure (lines {current_workflow.start_line}-{i})"
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
