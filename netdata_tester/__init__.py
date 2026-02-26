"""
Netdata Documentation Testing Agent

A CLI tool that validates Netdata documentation claims against a live Netdata Agent.
"""

__version__ = "1.1.0"
__author__ = "Netdata Team"

from .cli import main
from .parser import DocumentationParser, ClaimType
from .executors import TestExecutor
from .managers import ConfigManager
from .reporters import TestReporter

__all__ = [
    "main",
    "DocumentationParser",
    "ClaimType", 
    "TestExecutor",
    "ConfigManager",
    "TestReporter"
]
