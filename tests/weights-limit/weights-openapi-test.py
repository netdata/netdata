# SPDX-License-Identifier: GPL-3.0-or-later

"""Check weights query aliases without starting an Agent or contacting a server."""

import json
from pathlib import Path
import unittest

import yaml


ROOT = Path(__file__).resolve().parents[2]
ENDPOINTS = (
    "/api/v1/weights",
    "/api/v1/metric_correlations",
    "/api/v2/weights",
    "/api/v3/weights",
)
ALIASES = {"weightsLimit": "limit", "weightsCardinalityLimit": "cardinality_limit"}


class WeightsOpenAPI(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        api = ROOT / "src/web/api"
        cls.documents = {
            "JSON": json.loads((api / "netdata-swagger.json").read_text()),
            "YAML": yaml.safe_load((api / "netdata-swagger.yaml").read_text()),
        }

    def test_alias_representations_agree(self):
        for alias in ALIASES:
            with self.subTest(alias=alias):
                self.assertEqual(
                    self.documents["JSON"]["components"]["parameters"][alias],
                    self.documents["YAML"]["components"]["parameters"][alias],
                )

    def test_optional_aliases_have_no_defaults(self):
        for representation, document in self.documents.items():
            parameters = document["components"]["parameters"]
            for endpoint in ENDPOINTS:
                with self.subTest(representation=representation, endpoint=endpoint):
                    refs = [p.get("$ref") for p in document["paths"][endpoint]["get"]["parameters"]]
                    for alias, name in ALIASES.items():
                        self.assertEqual(refs.count("#/components/parameters/" + alias), 1)
                        parameter = parameters[alias]
                        self.assertEqual(parameter["name"], name)
                        self.assertEqual(parameter["in"], "query")
                        self.assertFalse(parameter.get("required", False))
                        self.assertTrue(parameter.get("allowEmptyValue", False))
                        self.assertEqual(parameter["schema"]["type"], "integer")
                        self.assertEqual(parameter["schema"]["minimum"], 0)
                        # A default on the other alias can override an explicitly chosen limit.
                        self.assertNotIn("default", parameter["schema"])


if __name__ == "__main__":
    unittest.main()
