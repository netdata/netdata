"""VM Connection Configuration - loads from environment or .env file"""

import os
from pathlib import Path

def load_env_file():
    """Load environment variables from .env file if present"""
    env_file = Path(__file__).parent / '.env'
    if env_file.exists():
        with open(env_file) as f:
            for line in f:
                line = line.strip()
                if line and not line.startswith('#') and '=' in line:
                    key, value = line.split('=', 1)
                    os.environ.setdefault(key.strip(), value.strip())

load_env_file()

VM_HOST = os.environ.get('TEST_VM_HOST', '')
VM_USER = os.environ.get('TEST_VM_USER', '')
VM_PASSWORD = os.environ.get('TEST_VM_PASSWORD', '')
NETDATA_URL = os.environ.get('NETDATA_URL', '')
OUTPUT_DIR = os.environ.get('OUTPUT_DIR', 'test_results')
