"""
Documentation Parser for Netdata Documentation Tester.

Extracts testable claims from markdown documentation files.
"""

import re
from dataclasses import dataclass, field
from enum import Enum
from pathlib import Path
from typing import Dict, List, Optional


class ClaimType(Enum):
    """Types of testable claims found in documentation."""
    CONFIGURATION = "configuration"
    COMMAND = "command"
    API_CALL = "api_call"
    EXPECTED_BEHAVIOR = "expected_behavior"
    UNKNOWN = "unknown"
    SKIP = "skip"


@dataclass
class TestableClaim:
    """A single testable claim extracted from documentation."""
    claim_id: str
    claim_type: ClaimType
    text: str
    file_path: str
    line_number: int
    code_block: Optional[str] = None
    expected_behavior: Optional[str] = None


class DocumentationParser:
    """Parser for extracting testable claims from documentation."""

    IMPERATIVE_PATTERNS = [
        r'\b(edit|modify|configure|change|set|add|create|run|execute|restart|enable|disable)\b',
    ]

    CODE_BLOCK_PATTERN = re.compile(r'```(\w+)?\n([\s\S]*?)```', re.MULTILINE)

    def __init__(self, verbose: bool = False):
        self.verbose = verbose

    def parse_file(self, file_path: str) -> List[TestableClaim]:
        """Parse a documentation file and extract all testable claims."""
        path = Path(file_path)
        if not path.exists():
            raise FileNotFoundError(f"Documentation file not found: {file_path}")

        content = path.read_text()
        return self.parse_content(content, str(path))

    def parse_content(self, content: str, source_name: str = "unknown") -> List[TestableClaim]:
        """Parse documentation content and extract testable claims."""
        claims = []
        lines = content.split('\n')

        for line_num, line in enumerate(lines, 1):
            line_claims = self._extract_claims_from_line(line, source_name, line_num)
            claims.extend(line_claims)

        return claims

    def _extract_claims_from_line(self, line: str, source_name: str, line_number: int) -> List[TestableClaim]:
        """Extract claims from a single line."""
        claims = []

        for pattern in self.IMPERATIVE_PATTERNS:
            if re.search(pattern, line, re.IGNORECASE):
                claim = TestableClaim(
                    claim_id=self._generate_claim_id(source_name, line_number),
                    claim_type=ClaimType.UNKNOWN,
                    text=line.strip(),
                    file_path=source_name,
                    line_number=line_number
                )
                claims.append(claim)
                break

        return claims

    def _generate_claim_id(self, source_name: str, line_number: int) -> str:
        """Generate a unique ID for a claim."""
        name = Path(source_name).stem
        return f"{name}_{line_number}"
