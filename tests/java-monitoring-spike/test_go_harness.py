#!/usr/bin/env python3
"""Offline checks of current regression evidence and process-control boundaries."""
import copy
import json
from pathlib import Path
from types import SimpleNamespace
import tempfile
import unittest
from unittest.mock import patch

from go_checks import GoLab
from summarize_go import summarize


class EvidenceTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        contexts = ("jvm_memory_used", "http_requests", "http_request_duration", "pool_connections",
                    "pool_pending_requests", "pool_connection_limit")
        phase = {"time": 1001, "rows": [], "charts": {}, "samples": {},
                 "journal": {name: {"status": "Attached"} for name in ("checkout", "inventory", "blocked")}}
        for app in ("checkout", "inventory"):
            phase["rows"].append({"Application": app, "Instance": app + "-old", "Status": "Collecting", "HTTP": "Observed"})
            for context in contexts:
                key = app + "." + context
                phase["charts"][key] = {"context": "java." + context,
                    "chart_labels": {"application": app, "instance": app + "-old"},
                    "chart_type": "heatmap" if context == "http_request_duration" else "line", "units": "observations/s"}
                phase["samples"][key] = {"labels": ["time", "200", "503"] if context == "http_requests" else ["time", "value"],
                                        "data": [[1000, 90, 10]] if context == "http_requests" else [[1000, 10]]}
        phase["rows"].append({"Application": "blocked", "Instance": "blocked-old", "Status": "Blocked",
                              "JVM": "Not observed", "HTTP": "Not observed", "Pools": "Not observed"})
        self.write("initial", phase)
        renamed = copy.deepcopy(phase)
        for chart in renamed["charts"].values():
            if chart["chart_labels"]["application"] == "checkout":
                chart["chart_labels"]["application_name"] = "Checkout API"
        self.write("renamed", renamed)
        self.write("job-restarted", phase)
        for name, status in (("excluded", "Excluded"), ("stale", "No fresh data")):
            inactive = copy.deepcopy(phase)
            inactive["rows"][1].update(Status=status, HTTP="Not observed")
            inactive["charts"] = {k: c for k, c in inactive["charts"].items() if c["chart_labels"]["application"] != "inventory"}
            self.write(name, inactive)
        final = copy.deepcopy(phase)
        final["rows"][0]["Instance"] = "checkout-new"
        for chart in final["charts"].values():
            if chart["chart_labels"]["application"] == "checkout":
                chart["chart_labels"]["instance"] = "checkout-new"
        self.write("app-restarted", final)
        self.write("checks", {"passed": True, "evidence_version": 2, "state_preserved_across_reconfiguration": True})
        self.write("test-response", {"status": 200})
        workload = {"requests": 1500, "errors": 0, "statuses": {"200": 1350, "503": 150}}
        self.write("initial-workloads", {"checkout": workload, "inventory": workload})
        self.write("restart-workload", workload)

    def write(self, name, value):
        (self.root / (name + ".json")).write_text(json.dumps(value))

    def test_native_evidence_needs_no_payload_recording(self):
        result = summarize(self.root)
        self.assertEqual(result["applications"]["checkout"]["active_charts"], 6)
        self.assertFalse(result["stored_cumulative_http_count_equivalence_verified"])
        self.assertFalse(result["native_vm_verified"])

    def test_stale_dimension_rejected(self):
        path = self.root / "app-restarted.json"
        final = json.loads(path.read_text())
        final["samples"]["checkout.jvm_memory_used"]["data"] = [[980, 10]]
        self.write("app-restarted", final)
        with self.assertRaises(AssertionError):
            summarize(self.root)

    def test_changed_credential_rejected(self):
        path = self.root / "job-restarted.json"
        phase = json.loads(path.read_text())
        phase["journal"]["checkout"]["credential_fingerprint"] = "changed"
        self.write("job-restarted", phase)
        with self.assertRaises(AssertionError):
            summarize(self.root)


class ControlTests(unittest.TestCase):
    def test_journal_does_not_expose_credentials(self):
        lab = GoLab(Path("/tmp"))
        lab.netdata = "owned"
        state = {"attempts": {"instance": {"token": "sensitive-token", "status": "Attached",
                                         "process": {"application": "checkout"}}}}
        with patch.object(lab, "docker", return_value=SimpleNamespace(stdout=json.dumps(state))):
            result = lab.journal()
        self.assertNotIn("sensitive-token", json.dumps(result))
        self.assertEqual(len(result["instance"]["credential_fingerprint"]), 64)

    def test_reused_pid_not_signalled(self):
        lab = GoLab(Path("/tmp"))
        lab.netdata = "owned"
        lab.apps = {"checkout": {"pid": 42, "start_time": "100"}}
        stat = "42 (java) " + " ".join(["0"] * 19 + ["200"])
        argv = "java\0--spring.application.name=checkout\0"
        with patch.object(lab, "docker", side_effect=[SimpleNamespace(stdout=stat), SimpleNamespace(stdout=argv)]) as call:
            with self.assertRaisesRegex(RuntimeError, "identity changed"):
                lab.signal_application("checkout", "TERM")
        self.assertEqual(call.call_count, 2)


if __name__ == "__main__":
    unittest.main()
