#!/usr/bin/env python3
"""Replay complete automatic-attachment and balanced cost evidence; emit a compact report."""
import argparse
import json
from pathlib import Path
import statistics

from automation_checks import REQUIRED, verify_capture
from run import metrics_summary


def fresh_contexts(directory, prefix, instance, since):
    charts = json.loads((directory / (prefix + "-charts.json")).read_text())
    data = json.loads((directory / (prefix + "-data.json")).read_text())
    return {item["context"] for key, item in data.items()
            if charts[key]["chart_labels"].get("resource.attributes.service.instance.id") == instance
            and item["status"] == 200
            and any(row[0] > since and any(value is not None for value in row[1:])
                    for row in item["response"].get("data", []))}


def summarize_auto(directory):
    result = json.loads((directory / "automatic-results.json").read_text())
    run = json.loads((directory / "ownership.json").read_text())["run"]
    events = result["events"]
    attempts = [e for e in events if e[2] == "attempt"]
    assert len(attempts) == len({e[1] for e in attempts}) == 5
    assert result["monitor_restart_attempts"] == 4
    assert sorted(e[3] for e in events if e[2] == "acknowledged") == ["10001", "10001", "10002"]
    assert sorted(e[3] for e in events if e[2] == "failed") == ["10003", "10004"]
    for uid in ("10001", "10002"):
        verify_capture(directory, "java-app-" + uid, result["initial"][uid]["workload"]["statuses"], "initial")
    logs = (directory / "scout.log").read_text()
    assert "Dynamic agent loading is not enabled" in logs
    assert "Read-only file system" in logs
    for uid in ("10003", "10004"):
        assert not metrics_summary(directory / "recording/metrics.json", "java-app-" + uid)["metrics"]
    before = [e[1] for e in result["initial_events"] if e[2] == "acknowledged" and e[3] == "10001"]
    after = [e[1] for e in events if e[2] == "acknowledged" and e[3] == "10001"]
    assert len(before) == 1 and len(after) == 2 and before[0] != after[-1]
    wanted = {"otel." + metric for metric in REQUIRED - {"http.server.request.duration"}}
    wanted |= {"otel.http.server.request.duration.count", "otel.http.server.request.duration.bucket"}
    fresh = fresh_contexts(directory, "java-app-10001-restarted", run + "-" + after[-1], result["restart_at"])
    assert wanted <= fresh, wanted - fresh
    return {"automatic_attachments": 3, "distinct_successful_uids": [10001, 10002],
            "duplicate_attempts_after_monitor_restart": 0,
            "explicit_failures": ["dynamic agent loading disabled", "target /tmp read-only"],
            "fresh_contexts_after_app_restart": sorted(fresh),
            "monitor_memory": json.loads(result["monitor_stats"])["MemUsage"]}


def require_balanced(results):
    assert set(results) == {f"cost-{block}-{mode}" for block in (1, 2, 3)
                           for mode in ("baseline", "full", "narrow")}, "all nine balanced cases required"


def summarize_cost(directory):
    results = json.loads((directory / "cost-results.json").read_text())
    require_balanced(results)
    rows = {}
    for name, result in results.items():
        block, mode = name.split("-")[1:]
        assert result["mode"] == mode and result["block"] == int(block)
        for phase, count in (("pre_warmup", 3000), ("warmup", 6000), ("workload", 9000)):
            load = result[phase]
            assert load["requests"] == count and load["errors"] == 0
            assert load["statuses"] == {"200": count * 9 // 10, "503": count // 10}
        if mode != "baseline":
            assert result["attach"]["status"] == 0
            verify_capture(directory, name, {"200": 13500, "503": 1500})
        else:
            assert not json.loads((directory / (name + "-metrics.json")).read_text())["metrics"]
        ticks = result["after"]["cpu_ticks"] - result["before"]["cpu_ticks"]
        cpu_ms = ticks / result["ticks_per_second"] * 1000 / result["workload"]["requests"]
        assert abs(cpu_ms - result["cpu_ms_per_request"]) < 1e-9
        rows[name] = {"rss_mib": result["after"]["rss_mib"], "cpu_ms_per_request": cpu_ms,
                      "p95_ms": result["workload"]["p95_ms"],
                      "schedule_lag_p95_ms": result["workload"]["schedule_lag_p95_ms"],
                      "attach_seconds": result.get("attach", {}).get("seconds")}
    aggregate = {}
    for mode in ("baseline", "full", "narrow"):
        selected = [row for name, row in rows.items() if name.endswith("-" + mode)]
        aggregate[mode] = {}
        for key in selected[0]:
            values = [row[key] for row in selected if row[key] is not None]
            if values:
                aggregate[mode][key] = {"median": statistics.median(values), "min": min(values), "max": max(values)}
    return {"runs": rows, "aggregate": aggregate}


def self_test():
    for malformed in ({}, {f"cost-1-{mode}": {} for mode in ("baseline", "full", "narrow")}):
        try:
            require_balanced(malformed)
        except AssertionError:
            pass
        else:
            raise AssertionError("incomplete cost evidence accepted")
    require_balanced({f"cost-{block}-{mode}": {} for block in (1, 2, 3) for mode in ("baseline", "full", "narrow")})
    print("complete balanced-case guard checks passed")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--automatic", type=Path)
    parser.add_argument("--cost", type=Path)
    parser.add_argument("--self-test", action="store_true")
    args = parser.parse_args()
    if args.self_test:
        self_test()
    else:
        if args.automatic is None and args.cost is None:
            parser.error("select --automatic and/or --cost")
        report = {}
        if args.automatic:
            report["automatic"] = summarize_auto(args.automatic)
        if args.cost:
            report["cost"] = summarize_cost(args.cost)
        print(json.dumps(report, indent=2))
