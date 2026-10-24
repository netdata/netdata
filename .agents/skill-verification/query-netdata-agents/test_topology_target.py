"""Exercise only target selection extracted from the topology recipe; no requests."""
import os
from pathlib import Path
import subprocess
import unittest

GUIDE = Path(__file__).resolve().parents[3] / 'docs/netdata-ai/skills/query-netdata-agents/query-topology.md'


class TargetTests(unittest.TestCase):
    def test_target_precedence_and_port_compatibility(self):
        text = GUIDE.read_text()
        setup = text.split('agents_load_env\n', 1)[1].split("\nread -r -d '' BODY", 1)[0]
        cases = [
            ({}, '127.0.0.1:19999'),
            ({'AGENT_PORT': '20000'}, '127.0.0.1:20000'),
            ({'AGENT_HOST': 'agent.invalid'}, 'agent.invalid:19999'),
            ({'AGENT_HOST': 'agent.invalid', 'AGENT_PORT': '20000'}, 'agent.invalid:20000'),
            ({'AGENT_HOST': 'agent.invalid:20001', 'AGENT_PORT': '20000'}, 'agent.invalid:20001'),
            ({'AGENT_HOST': '[::1]:20001', 'AGENT_PORT': '20000'}, '[::1]:20001'),
            ({'AGENT_HOST': '[::1]', 'AGENT_PORT': '20000'}, '[::1]:20000'),
            ({'AGENT_URL': 'http://explicit.invalid:20002/path', 'AGENT_HOST': 'other.invalid',
              'AGENT_PORT': '20000'}, 'explicit.invalid:20002'),
        ]
        for values, expected in cases:
            with self.subTest(values=values):
                result = subprocess.run(['bash', '-eu', '-c', setup + '\nprintf "%s" "$AGENT_TARGET"'],
                                        env={'PATH': os.environ['PATH'], **values},
                                        text=True, capture_output=True, timeout=5)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(result.stdout, expected)


if __name__ == '__main__':
    unittest.main()
