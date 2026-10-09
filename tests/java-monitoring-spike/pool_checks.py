#!/usr/bin/env python3
"""Real-source oracle for late Hikari discovery, concurrency, tracker preservation and removal."""
import argparse
import concurrent.futures
import json
from pathlib import Path
import subprocess
import sys
import time
import urllib.request

from run import Lab, ROOT, request, wait_http
from summarize import attributes, points

PREFIX = "netdata.spike.hikari."


def samples(directory, service, since, until):
    result = {}
    for line in (directory / "recording/metrics.json").read_text().splitlines(keepends=True):
        if not line.endswith("\n"):
            continue
        for resource in json.loads(line).get("resourceMetrics", []):
            if attributes(resource["resource"]["attributes"]).get("service.name") != service:
                continue
            for scope in resource.get("scopeMetrics", []):
                for metric in scope.get("metrics", []):
                    for point in points(metric):
                        stamp = int(point["timeUnixNano"]) / 1e9
                        if since < stamp <= until:
                            result.setdefault(metric["name"], {}).setdefault(stamp, []).append(point)
    return result


def verify(directory):
    runs = json.loads((directory / "pool-checks.json").read_text())
    assert runs, "no lifecycle runs recorded"
    required_phases = {"idle_after_attach", "discovered", "saturated", "released", "idle_discovered", "closed",
                       "recreated", "repeated_recreation", "all_closed"}
    report = {}
    for service, run in runs.items():
        assert set(run["phases"]) == required_phases, service + ": incomplete lifecycle phases"
        assert run["attach"]["status"] == 0
        assert run["before"]["tracker_acquired"] == run["before"]["tracker_used"] == 2
        identities = {}
        for phase, check in run["phases"].items():
            # Exclude collection in flight while the fixture operation finished.
            observed = samples(directory, service, check["since"] + 1, check["until"])
            assert len(observed.get("jvm.memory.used", {})) >= 2, phase + ": no fresh telemetry"
            expected = check["expected"]
            actual = {key for key in observed if key.startswith(PREFIX)}
            if not expected:
                assert not actual, phase + ": removed or undiscovered pools emitted metrics"
            else:
                assert actual == {PREFIX + suffix for suffix in ("connections", "pending_requests", "limit")}
                for suffix in ("connections", "pending_requests", "limit"):
                    for stamp, batch in observed[PREFIX + suffix].items():
                        values = {}
                        for point in batch:
                            attrs = attributes(point["attributes"])
                            key = (attrs["pool.name"], attrs.get("state"))
                            assert key not in values, phase + ": duplicate pool observation"
                            values[key] = int(point["asInt"])
                            identities.setdefault(phase, {}).setdefault(attrs["pool.name"], set()).add(attrs["pool.id"])
                        wanted = {}
                        for pool, fields in expected.items():
                            if suffix == "connections":
                                wanted[(pool, "active")] = fields["active"]
                                wanted[(pool, "idle")] = fields["idle"]
                            else:
                                wanted[(pool, None)] = fields[suffix]
                        assert values == wanted, f"{service}/{phase}/{suffix}: {values} != {wanted}"
            oracle = check["oracle"]
            assert oracle["tracker_preserved"] and oracle["hikari_mbeans"] == 0
            if "controlled" in expected:
                expected_pool = expected["controlled"]
                assert oracle["active"] == expected_pool["active"] and oracle["idle"] == expected_pool["idle"]
                assert oracle["pending"] == expected_pool["pending_requests"] and oracle["limit"] == expected_pool["limit"]
        old = identities["discovered"]["controlled"]
        new = identities["recreated"]["controlled"]
        assert len(old) == len(new) == 1 and old.isdisjoint(new)
        final = run["phases"]["all_closed"]["oracle"]
        assert final["tracker_acquired"] == final["tracker_used"] == 43, final
        assert final["trackers_created"] == final["trackers_closed"] == 8, final
        # Exact stable nonzero pool values must also survive the actual Netdata ingestion path.
        charts = json.loads((directory / (service + "-saturated-charts.json")).read_text())
        data = json.loads((directory / (service + "-saturated-data.json")).read_text())
        stored = set()
        interval = run["phases"]["saturated"]
        for key, item in data.items():
            if item["context"].startswith("otel." + PREFIX):
                chart = charts[key]
                for row in item["response"].get("data", []):
                    if interval["since"] + 1 < row[0] <= interval["until"]:
                        for value in row[1:]:
                            if value is not None:
                                stored.add((item["context"], chart["chart_labels"].get("state"), value))
        assert ("otel." + PREFIX + "pending_requests", None, 1) in stored, stored
        assert ("otel." + PREFIX + "limit", None, 2) in stored, stored
        assert ("otel." + PREFIX + "connections", "active", 2) in stored, stored
        assert ("otel." + PREFIX + "connections", "idle", 0) in stored, stored
        report[service] = {"phases_verified": list(run["phases"]), "attach_seconds": run["attach"]["seconds"],
                           "tracker_acquisitions": final["tracker_acquired"], "tracker_closes": final["trackers_closed"],
                           "stored_saturated_values": sorted(stored, key=str)}
    return report


def experiment(lab, image, runs):
    service = "pool-oracle-" + image.rsplit(":", 1)[-1]
    cid = lab.create(service, ["--network", lab.network, "-p", "127.0.0.1::8080", image,
                              "-cp", "/lab/lifecycle.jar:/lab/lib/*", "org.netdata.spike.PoolLifecycle"])
    lab.start(cid)
    url = lab.base_url(cid, 8080)
    wait_http(url + "/health")

    def state():
        return json.loads(request(url + "/state")[1])

    def action(path):
        with urllib.request.urlopen(urllib.request.Request(url + path, data=b"", method="POST"), timeout=10) as response:
            return json.load(response)

    run = {"before": state(), "phases": {}}
    runs[service] = run
    run["attach"] = lab.probe(cid, "agent", "/lab/otel.jar", lab.agent_options(service, extension=True))
    lab.save("pool-checks.json", runs)
    if run["attach"]["status"]:
        raise RuntimeError("extension attachment failed")

    def phase(name, expected):
        since = time.time()
        # This pinned Netdata image stores these charts every ten seconds.
        # Hold the nonzero oracle state across multiple storage intervals.
        time.sleep(25 if name == "saturated" else 5)
        oracle = state()
        until = time.time()
        run["phases"][name] = {"since": since, "until": until, "oracle": oracle, "expected": expected,
                               "coverage": lab.capture(service, name)}
        lab.save("pool-checks.json", runs)
        print(service + ": recorded " + name, file=sys.stderr, flush=True)

    def pool(active=0, idle=2, pending=0, limit=2):
        return {"active": active, "idle": idle, "pending_requests": pending, "limit": limit}

    phase("idle_after_attach", {})
    with concurrent.futures.ThreadPoolExecutor(max_workers=16) as clients:
        list(clients.map(action, ["/borrow"] * 16))
    phase("discovered", {"controlled": pool()})
    action("/hold")
    deadline = time.monotonic() + 5
    while state()["pending"] != 1:
        if time.monotonic() > deadline:
            raise RuntimeError("controlled waiter did not block")
        time.sleep(.1)
    phase("saturated", {"controlled": pool(active=2, idle=0, pending=1)})
    action("/release")
    phase("released", {"controlled": pool()})
    with concurrent.futures.ThreadPoolExecutor(max_workers=16) as clients:
        list(clients.map(action, ["/idle-use"] * 16))
    phase("idle_discovered", {"controlled": pool(), "idle-until-used": pool(idle=1, limit=1)})
    action("/close")
    phase("closed", {"idle-until-used": pool(idle=1, limit=1)})
    action("/recreate")
    action("/borrow")
    phase("recreated", {"controlled": pool(idle=3, limit=3), "idle-until-used": pool(idle=1, limit=1)})
    for _ in range(5):
        action("/close")
        action("/recreate")
        action("/borrow")
    phase("repeated_recreation", {"controlled": pool(idle=3, limit=3), "idle-until-used": pool(idle=1, limit=1)})
    action("/idle-close")
    action("/close")
    phase("all_closed", {})
    lab.docker("stop", "--time", "10", cid)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--verify", type=Path)
    parser.add_argument("--output", type=Path)
    parser.add_argument("--images", nargs="+", default=["netdata-java-spike:21"])
    args = parser.parse_args()
    if args.verify:
        print(json.dumps(verify(args.verify), indent=2))
        return
    output = (args.output or ROOT.parents[1] / ".local/java-monitoring-spike" /
              ("pool-" + time.strftime("%Y%m%d-%H%M%S"))).resolve()
    output.mkdir(parents=True, exist_ok=False)
    lab = Lab(output)
    try:
        lab.setup()
        runs = {}
        for image in args.images:
            experiment(lab, image, runs)
        report = verify(output)
        lab.save("pool-verification.json", report)
        print(json.dumps(report, indent=2))
    finally:
        lab.close()


if __name__ == "__main__":
    try:
        main()
    except subprocess.CalledProcessError as error:
        print(error.stderr, file=sys.stderr)
        sys.exit(error.returncode)
