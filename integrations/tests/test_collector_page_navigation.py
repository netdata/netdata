#!/usr/bin/env python3

import re
import sys
import unittest
from pathlib import Path

INTEGRATIONS_DIR = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(INTEGRATIONS_DIR))

from gen_doc_collector_page import _render_tech_navigation  # noqa: E402


class CollectorPageNavigationTest(unittest.TestCase):
    def test_quick_navigation_targets_generated_sections(self):
        targets = set(re.findall(r"\]\((#[^)]+)\)", _render_tech_navigation()))

        self.assertEqual(
            targets,
            {
                "#beyond-the-850-integrations",
                "#cloud-and-devops",
                "#containers-and-vms",
                "#databases",
                "#networking",
                "#operating-systems",
                "#web-servers-and-proxies",
            },
        )


if __name__ == "__main__":
    unittest.main()
