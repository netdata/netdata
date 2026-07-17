#!/usr/bin/env python3

import sys
import unittest
from pathlib import Path

INTEGRATIONS_DIR = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(INTEGRATIONS_DIR))

import gen_integrations


class MetricsTemplateTest(unittest.TestCase):
    def test_metric_description_column_is_safe_and_aligned(self):
        entry = {
            "metrics": {
                "folding": {"title": "Metrics", "enabled": False},
                "description": "",
                "availability": ["Linux"],
                "scopes": [
                    {
                        "name": "system",
                        "description": "",
                        "labels": [],
                        "metrics": [
                            {
                                "name": "test.metric",
                                "description": "First | second\n<strong>tag</strong>",
                                "dimensions": [{"name": "value"}],
                                "unit": "events/s",
                                "availability": [],
                            },
                        ],
                    },
                ],
            },
        }

        rendered = (
            gen_integrations.get_jinja_env()
            .get_template("metrics.md")
            .render(entry=entry, clean=True)
        )

        self.assertIn("| Metric | Description | Dimensions | Unit | Linux |", rendered)
        self.assertIn("|:------|:------------|:----------|:----|:---:|", rendered)
        self.assertIn(
            "| test.metric | First / second &lt;strong&gt;tag&lt;/strong&gt; | value | events/s | • |",
            rendered,
        )


if __name__ == "__main__":
    unittest.main()
