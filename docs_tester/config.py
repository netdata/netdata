"""VM Connection Configuration - loads from environment or uses safe defaults"""

import os

VM_HOST = os.environ.get('VM_HOST', '10.10.30.140')
VM_USER = os.environ.get('VM_USER', 'cm')
VM_PASSWORD = os.environ.get('VM_PASSWORD', '123')
NETDATA_URL = os.environ.get('NETDATA_URL', 'http://10.10.30.140:19999')
OUTPUT_DIR = os.environ.get('OUTPUT_DIR', 'test_results')
