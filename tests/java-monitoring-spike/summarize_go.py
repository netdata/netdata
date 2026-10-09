#!/usr/bin/env python3
"""Replay historical spike or current native-plugin container regression evidence."""
import argparse
import json
import math
from pathlib import Path


def summarize(root):
    def read(name):
        return json.loads((root / (name + ".json")).read_text())

    initial, renamed, excluded, restarted, stale, final = (
        read(name) for name in ("initial", "renamed", "excluded", "job-restarted", "stale", "app-restarted"))
    checks = read("checks")
    native = checks.get("evidence_version") == 2
    if native:
        assert checks == {"passed": True, "evidence_version": 2,
                          "state_preserved_across_reconfiguration": True}
        assert len(initial["journal"]) == 3
        assert initial["journal"] == renamed["journal"] == restarted["journal"]
    else:
        assert checks == {"passed": True, "initial_attempts": 3, "final_attempts": 4}
    prefix = "java." if native else "java_spike."
    assert read("test-response")["status"] == 200
    assert set(initial["charts"]) == set(renamed["charts"])
    for chart in renamed["charts"].values():
        if chart["chart_labels"]["application"] == "checkout":
            assert chart["chart_labels"]["application_name"] == "Checkout API"
    identities = lambda phase: {r["Application"]: r["Instance"] for r in phase["rows"]}
    assert identities(initial) == identities(restarted)
    assert identities(initial)["checkout"] != identities(final)["checkout"]
    assert identities(initial)["inventory"] == identities(final)["inventory"]
    for phase, status in ((excluded, "Excluded"), (stale, "No fresh data")):
        row = next(r for r in phase["rows"] if r["Application"] == "inventory")
        assert row["Status"] == status
        assert row["HTTP"] == "Not observed"
        assert not any(c["chart_labels"]["application"] == "inventory" for c in phase["charts"].values())
    blocked = next(r for r in final["rows"] if r["Application"] == "blocked")
    assert blocked["Status"] == "Blocked"
    assert all(blocked[k] == "Not observed" for k in ("JVM", "HTTP", "Pools"))

    expected = {prefix + x for x in ("jvm_memory_used", "http_requests", "http_request_duration",
                                           "pool_connections", "pool_pending_requests", "pool_connection_limit")}
    apps = {}
    for name in ("checkout", "inventory"):
        active_id = identities(final)[name]
        charts = {key: c for key, c in final["charts"].items() if c["chart_labels"]["instance"] == active_id}
        assert {c["context"] for c in charts.values()} == expected
        for key, c in charts.items():
            sample = final["samples"][key]
            values = [v for row in sample["data"] for v in row[1:] if v is not None]
            assert values and all(math.isfinite(v) and v >= 0 for v in values)
            for index in range(1, len(sample["labels"])):
                latest = max(row[0] for row in sample["data"] if row[index] is not None)
                assert 0 <= final["time"] - latest <= 10
            if c["context"] == prefix + "http_request_duration":
                assert c["chart_type"] == "heatmap"
                assert c["units"] == "observations/s"
            if c["context"] == prefix + "pool_connection_limit":
                assert all(v == 10 for v in values)
        rates = {}
        for key, c in initial["charts"].items():
            if c["context"] == prefix + "http_requests" and c["chart_labels"]["application"] == name:
                sample = initial["samples"][key]
                for index, status in enumerate(sample["labels"][1:], 1):
                    values = [row[index] for row in sample["data"] if row[index] is not None]
                    rates[status] = round(max(values), 3)
        assert set(rates) == {"200", "503"} and all(v > 0 for v in rates.values())
        apps[name] = {"stored_contexts": sorted(expected), "active_charts": len(charts), "peak_request_rates": rates}

    for workload in [*read("initial-workloads").values(), read("restart-workload")]:
        assert workload["requests"] == 1500 and workload["errors"] == 0
        assert workload["statuses"] == {"200": 1350, "503": 150}
    if native:
        return {"evidence_version": 2, "environment": "owned container with same-rootfs fixture JVMs",
                "applications": apps, "client_http_counts_per_workload": {"200": 1350, "503": 150},
                "stored_cumulative_http_count_equivalence_verified": False,
                "rename_preserves_chart_identity": True, "job_restart_preserves_process_identity": True,
                "durable_state_preserved_across_reconfiguration": True,
                "exclusion_removes_active_charts": True, "stale_source_removes_coverage_and_active_charts": True,
                "application_restart_changes_identity": True, "all_final_dimensions_have_samples_within_seconds": 10,
                "dyncfg_preflight_passed": True, "dyncfg_transport": "owned local plugins.d input pipe",
                "authenticated_form_rendering_verified": False, "native_vm_verified": False}

    # Compare full cumulative counters, independently of Netdata's sampled rates.
    totals = {}
    observations = {}
    for line in (root / "state/metrics.jsonl").read_text().splitlines():
        for resource in json.loads(line)["resourceMetrics"]:
            attrs = {a["key"]: a["value"].get("stringValue") for a in resource["resource"]["attributes"]}
            instance = attrs.get("service.instance.id")
            for scope in resource.get("scopeMetrics", []):
                for metric in scope.get("metrics", []):
                    if metric["name"] != "http.server.request.duration":
                        continue
                    for point in metric["histogram"]["dataPoints"]:
                        status = next(a["value"]["intValue"] for a in point["attributes"] if a["key"] == "http.response.status_code")
                        key = (instance, status)
                        totals[key] = max(totals.get(key, 0), int(point["count"]))
                        observations.setdefault(key, []).append((int(point["timeUnixNano"]) / 1e9, int(point["count"])))
    for name in ("checkout", "inventory"):
        identity = identities(initial)[name]
        assert {status: totals[(identity, status)] for status in ("200", "503")} == {"200": 1350, "503": 150}
    identity = identities(final)["checkout"]
    # Automatic attachment can precede the restarted application's readiness GET.
    # The harness waits five seconds before the workload. Count a readiness request
    # only when several one-count exports precede the workload's first error status.
    first_error = min(stamp for stamp, _ in observations[(identity, "503")])
    early = [(stamp, count) for stamp, count in observations[(identity, "200")] if stamp <= first_error - 2]
    readiness = 0
    if early:
        assert all(count == 1 for _, count in early)
        assert max(stamp for stamp, _ in early) - min(stamp for stamp, _ in early) >= 2
        readiness = 1
    assert totals[(identity, "200")] == 1350 + readiness
    assert totals[(identity, "503")] == 150
    for workload in [*read("initial-workloads").values(), read("restart-workload")]:
        assert workload["requests"] == 1500 and workload["errors"] == 0
        assert workload["statuses"] == {"200": 1350, "503": 150}

    return {"applications": apps, "http_counts_per_workload": {"200": 1350, "503": 150},
            "restarted_application_extra_readiness_requests": readiness,
            "attempts": {"initial": 3, "final": 4}, "rename_preserves_chart_identity": True,
            "job_restart_preserves_process_identity": True, "exclusion_removes_active_charts": True,
            "stale_source_removes_coverage_and_active_charts": True, "application_restart_changes_identity": True,
            "all_final_dimensions_have_samples_within_seconds": 10,
            "dyncfg_preflight_passed": True, "dyncfg_transport": "owned local plugins.d input pipe",
            "authenticated_form_rendering_verified": False}


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("directory", type=Path)
    args = parser.parse_args()
    print(json.dumps(summarize(args.directory), indent=2))
