#!/usr/bin/env python3
"""
Normalization utilities for comparing AI-generated outputs with originals.
Handles alert definitions and other Netdata configuration formats.
"""

import re
from typing import Dict, Any, List, Optional
from dataclasses import dataclass


@dataclass
class NormalizedAlert:
    """Normalized representation of a Netdata alert."""
    template: str           # Alert template name
    on: str                 # Chart/context this applies to
    class_name: str         # Alert classification
    type_name: str          # Alert type
    component: str          # Component name
    lookup: Optional[str]   # Lookup expression
    calc: Optional[str]     # Calculation expression
    every: Optional[str]    # Evaluation frequency
    units: Optional[str]    # Display units
    warn: Optional[str]     # Warning condition
    crit: Optional[str]     # Critical condition
    delay: Optional[str]    # Delay settings
    summary: Optional[str]  # Summary template
    info: Optional[str]     # Info template
    to: Optional[str]       # Notification target
    options: List[str]      # Alert options
    host_labels: Optional[str]
    chart_labels: Optional[str]


def normalize_duration(duration: str) -> str:
    """
    Normalize duration strings to canonical form.

    Examples:
        1m -> 60s
        1h -> 3600s
        1d -> 86400s
        60s -> 60s
    """
    if not duration:
        return ""

    duration = duration.strip().lower()

    # Already in seconds
    if duration.endswith('s') and duration[:-1].isdigit():
        return duration

    multipliers = {
        's': 1,
        'm': 60,
        'h': 3600,
        'd': 86400,
        'w': 604800,
    }

    # Handle compound durations like "1h30m"
    total_seconds = 0
    current_num = ""

    for char in duration:
        if char.isdigit() or char == '.':
            current_num += char
        elif char in multipliers and current_num:
            total_seconds += float(current_num) * multipliers[char]
            current_num = ""

    if total_seconds > 0:
        return f"{int(total_seconds)}s"

    return duration


def normalize_expression(expr: str) -> str:
    """
    Normalize mathematical expressions.

    - Remove extra whitespace
    - Normalize operators
    - Sort commutative operations where possible
    """
    if not expr:
        return ""

    # Remove extra whitespace
    expr = " ".join(expr.split())

    # Normalize comparison operators
    expr = re.sub(r'\s*([<>=!]+)\s*', r' \1 ', expr)

    # Normalize arithmetic operators
    expr = re.sub(r'\s*([+\-*/])\s*', r' \1 ', expr)

    # Clean up multiple spaces
    expr = " ".join(expr.split())

    return expr.strip()


def normalize_whitespace(text: str) -> str:
    """Normalize whitespace in text."""
    if not text:
        return ""
    return " ".join(text.split())


def parse_alert_config(config_text: str) -> Dict[str, Any]:
    """
    Parse a Netdata alert configuration block into a dictionary.

    Args:
        config_text: Raw alert configuration text

    Returns:
        Dictionary with normalized alert fields
    """
    result = {}
    current_key = None
    current_value = []

    for line in config_text.strip().split('\n'):
        line = line.strip()

        # Skip empty lines and comments
        if not line or line.startswith('#'):
            continue

        # Check if this is a key: value line
        if ':' in line and not line.startswith(' '):
            # Save previous key-value if exists
            if current_key:
                result[current_key] = normalize_whitespace(' '.join(current_value))

            # Parse new key-value
            key, _, value = line.partition(':')
            current_key = key.strip().lower().replace(' ', '_')
            current_value = [value.strip()] if value.strip() else []
        else:
            # Continuation of previous value
            if current_key:
                current_value.append(line)

    # Don't forget the last key-value
    if current_key:
        result[current_key] = normalize_whitespace(' '.join(current_value))

    return result


def normalize_alert(config_text: str) -> Dict[str, str]:
    """
    Fully normalize an alert configuration for comparison.

    Args:
        config_text: Raw alert configuration text

    Returns:
        Dictionary with normalized, comparable values
    """
    parsed = parse_alert_config(config_text)
    normalized = {}

    # Fields that need duration normalization
    duration_fields = ['every', 'delay', 'after']

    # Fields that need expression normalization
    expression_fields = ['lookup', 'calc', 'warn', 'crit']

    for key, value in parsed.items():
        if key in duration_fields:
            normalized[key] = normalize_duration(value)
        elif key in expression_fields:
            normalized[key] = normalize_expression(value)
        else:
            normalized[key] = normalize_whitespace(value)

    return normalized


def alerts_equal(alert1: str, alert2: str) -> tuple[bool, List[str]]:
    """
    Compare two alert configurations for equality.

    Args:
        alert1: First alert configuration
        alert2: Second alert configuration

    Returns:
        Tuple of (are_equal, list_of_differences)
    """
    norm1 = normalize_alert(alert1)
    norm2 = normalize_alert(alert2)

    differences = []

    # Get all keys from both
    all_keys = set(norm1.keys()) | set(norm2.keys())

    for key in sorted(all_keys):
        val1 = norm1.get(key, "<missing>")
        val2 = norm2.get(key, "<missing>")

        if val1 != val2:
            differences.append(f"{key}: '{val1}' != '{val2}'")

    return len(differences) == 0, differences


if __name__ == "__main__":
    # Test normalization
    test_alert = """
 template: disk_space_usage
       on: disk.space
    class: Utilization
     type: System
component: Disk
   lookup: average -1m unaligned of avail
    units: %
     calc: $avail * 100 / ($avail + $used)
    every: 1m
     warn: $this > 80
     crit: $this > 90
    delay: down 15m multiplier 1.5 max 1h
  summary: Disk space usage on ${label:mount_point}
     info: Disk space utilization on ${label:mount_point}
       to: sysadmin
    """

    print("Parsing alert configuration...")
    normalized = normalize_alert(test_alert)

    for key, value in sorted(normalized.items()):
        print(f"  {key}: {value}")
