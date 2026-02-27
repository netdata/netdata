"""VM Connection Configuration - loads from environment or uses safe defaults"""

import os

VM_HOST = os.environ.get('TEST_VM_HOST', '')
VM_USER = os.environ.get('TEST_VM_USER', '')
VM_PASSWORD = os.environ.get('TEST_VM_PASSWORD', '')
NETDATA_URL = os.environ.get('NETDATA_URL', 'http://<vm-host-ip>:19999')
OUTPUT_DIR = os.environ.get('OUTPUT_DIR', 'test_results')
