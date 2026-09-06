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

    def test_combined_aliases_preserve_caller_inputs(self):
        cases = {
            "omitted": {},
            "limit": {"limit": 1000},
            "cardinality": {"cardinality_limit": 1000},
            "explicit-zero": {"limit": 0},
            "zero-precedence": {"limit": 1000, "cardinality_limit": 0},
            "both-positive": {"limit": 1000, "cardinality_limit": 2},
            "empty-limit": {"limit": ""},
            "empty-cardinality": {"cardinality_limit": ""},
        }
        for representation, document in self.documents.items():
            parameters = document["components"]["parameters"]
            for endpoint in ENDPOINTS:
                with self.subTest(representation=representation, endpoint=endpoint):
                    refs = [p.get("$ref") for p in document["paths"][endpoint]["get"]["parameters"]]
                    defaults = {}
                    for alias, name in ALIASES.items():
                        self.assertEqual(refs.count("#/components/parameters/" + alias), 1)
                        parameter = parameters[alias]
                        self.assertEqual(parameter["name"], name)
                        self.assertEqual(parameter["in"], "query")
                        self.assertFalse(parameter.get("required", False))
                        self.assertTrue(parameter.get("allowEmptyValue", False))
                        self.assertEqual(parameter["schema"]["type"], "integer")
                        self.assertEqual(parameter["schema"]["minimum"], 0)
                        if "default" in parameter["schema"]:
                            defaults[name] = parameter["schema"]["default"]

                    # Optional aliases must not prefill an unchosen field in API explorers.
                    # This checks the schema inputs; browser rendering is a separate integration check.
                    for case, supplied in cases.items():
                        with self.subTest(case=case):
                            populated = defaults.copy()
                            populated.update(supplied)
                            self.assertEqual(populated, supplied)


if __name__ == "__main__":
    unittest.main()
