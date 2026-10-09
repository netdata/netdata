#!/usr/bin/env python3
"""Summarize real lab evidence; fail if a positive control or stored-sample check is missing."""
import argparse
import json
from pathlib import Path
import re


def attributes(items):
    return {item["key"]: next(iter(item["value"].values())) for item in items}


def points(metric):
    return next(metric[kind]["dataPoints"] for kind in ("gauge", "sum", "histogram") if kind in metric)


def summarize(directory):
    results = json.loads((directory / "results.json").read_text())
    output = {}
    for name, result in results.items():
        assert result["jmx"]["status"] == 0, name + ": local JMX failed"
        assert "HeapMemoryUsage=" in result["jmx"]["stdout"], name + ": missing JMX heap data"
        payload = json.loads((directory / (name + "-metrics.json")).read_text())
        metrics = payload["metrics"]
        domains = sorted({line.split()[1].split(":")[0] for line in result["jmx"]["stdout"].splitlines()
                          if line.startswith("MBEAN ")})
        record = {"java_version": result["java_version"].splitlines()[0], "mbean_domains": domains,
                  "metric_names": sorted(metrics), "netdata_charts": result["coverage"]["netdata_charts"],
                  "charts_with_samples": result["coverage"]["charts_with_samples"],
                  "workload": result["workload"],
                  "rss_mib": int(re.search(r"VmRSS:\s+(\d+)",
                      result["after_workload"]["proc_status"]).group(1)) / 1024}
        attached_late = name.startswith(("late-", "extension-"))
        if attached_late:
            assert result["attach"]["status"] == 0, name + ": agent attachment failed"
            record["attach_seconds"] = result["attach"]["seconds"]
        if not name.startswith("baseline-"):
            assert "jvm.memory.used" in metrics, name + ": missing JVM data"
            http = metrics["http.server.request.duration"]
            record["http_status_counts"] = {str(attributes(p["attributes"])["http.response.status_code"]):
                                            int(p["count"]) for p in points(http)}
            assert all(record["http_status_counts"].get(code, 0) > 0 for code in ("200", "503"))
            if attached_late:
                assert record["http_status_counts"] == result["workload"]["statuses"], name + ": HTTP count mismatch"
            assert all(sum(map(int, p["bucketCounts"])) == int(p["count"]) for p in points(http))
            record["identity_fields"] = sorted(payload["resources"][0])
            samples = json.loads((directory / (name + "-data.json")).read_text())
            stored_contexts = {value["context"] for value in samples.values()
                               if value["status"] == 200 and any(any(v is not None for v in row[1:])
                                   for row in value["response"].get("data", []))}
            assert {"otel.jvm.memory.used", "otel.http.server.request.duration.count",
                    "otel.http.server.request.duration.bucket"} <= stored_contexts, name + ": missing stored data"
            if name.startswith("startup-"):
                assert {"db.client.connections.usage", "db.client.connections.pending_requests"} <= metrics.keys()
                assert {"otel.db.client.connections.usage", "otel.db.client.connections.pending_requests"} <= stored_contexts
            if name.startswith("extension-"):
                required = {"netdata.spike.hikari.connections", "netdata.spike.hikari.pending_requests",
                            "netdata.spike.hikari.limit"}
                assert required <= metrics.keys()
                assert {"otel." + metric for metric in required} <= stored_contexts
                usage = points(metrics["netdata.spike.hikari.connections"])
                assert len(usage) == 2
                assert {attributes(p["attributes"])["state"] for p in usage} == {"active", "idle"}
                assert len({attributes(p["attributes"])["pool.id"] for p in usage}) == 1
        output[name] = record
    return output


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("directory", type=Path)
    args = parser.parse_args()
    print(json.dumps(summarize(args.directory), indent=2))
