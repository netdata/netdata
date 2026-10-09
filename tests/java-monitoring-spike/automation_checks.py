#!/usr/bin/env python3
"""Disposable discovery/attachment and repeated instrumentation-cost experiments."""
import argparse
import json
from pathlib import Path
import re
import subprocess
import sys
import time

from run import Lab, ROOT, metrics_summary, request, wait_http
from summarize import attributes, points

MONITOR = "netdata-java-spike:monitor"
PLAIN = "netdata-java-spike:plain-jre"
MODULES = ("runtime-telemetry", "servlet", "tomcat", "spring-webmvc", "netdata-hikari")
REQUIRED = {"jvm.memory.used", "http.server.request.duration", "netdata.spike.hikari.connections",
            "netdata.spike.hikari.pending_requests", "netdata.spike.hikari.limit"}


def options(lab, name, narrow=False):
    config = lab.agent_options(name, extension=True)
    if narrow:
        config += ";otel.instrumentation.common.default-enabled=false"
        config += "".join(f";otel.instrumentation.{module}.enabled=true" for module in MODULES)
    return config


def paced(lab, name, target, seconds, rate=300):
    client = lab.create(name, ["--network", f"container:{target}", "--user", "10005:10005",
        "--entrypoint", "java", MONITOR, "-Xms16m", "-Xmx96m", "-cp", "/lab", "HttpLoad",
        "http://127.0.0.1:8080", str(seconds), str(rate)])
    output = lab.docker("start", "--attach", client, timeout=seconds + 90)
    code = int(lab.docker("inspect", "--format", "{{.State.ExitCode}}", client).stdout)
    if code:
        raise subprocess.CalledProcessError(code, "workload " + name, output.stdout, output.stderr)
    return json.loads(output.stdout.strip().splitlines()[-1])


def process_sample(lab, cid):
    raw = lab.docker("exec", cid, "cat", "/proc/1/stat", "/proc/1/status").stdout
    stat = raw.splitlines()[0].rsplit(") ", 1)[1].split()
    return {"at": time.monotonic(), "cpu_ticks": int(stat[11]) + int(stat[12]),
            "rss_mib": int(re.search(r"VmRSS:\s+(\d+)", raw).group(1)) / 1024,
            "threads": int(re.search(r"Threads:\s+(\d+)", raw).group(1)), "raw": raw}


def verify_capture(directory, name, expected=None, checkpoint=""):
    prefix = name + ("-" + checkpoint if checkpoint else "")
    payload = json.loads((directory / (prefix + "-metrics.json")).read_text())
    metrics = payload["metrics"]
    assert REQUIRED <= metrics.keys(), (prefix, REQUIRED - metrics.keys())
    http = points(metrics["http.server.request.duration"])
    counts = {}
    for point in http:
        attrs = attributes(point["attributes"])
        assert attrs["http.route"] == "/work", (prefix, attrs)
        status = str(attrs["http.response.status_code"])
        counts[status] = counts.get(status, 0) + int(point["count"])
        assert sum(map(int, point["bucketCounts"])) == int(point["count"])
    assert set(counts) == {"200", "503"}, counts
    if expected is not None:
        assert counts == expected, (prefix, counts, expected)
    usage = points(metrics["netdata.spike.hikari.connections"])
    assert len(usage) == 2 and {attributes(p["attributes"])["state"] for p in usage} == {"active", "idle"}
    stored = json.loads((directory / (prefix + "-data.json")).read_text())
    contexts = {item["context"] for item in stored.values() if item["status"] == 200
                and any(any(v is not None for v in row[1:]) for row in item["response"].get("data", []))}
    wanted = {"otel." + metric for metric in REQUIRED - {"http.server.request.duration"}}
    wanted |= {"otel.http.server.request.duration.count", "otel.http.server.request.duration.bucket"}
    assert wanted <= contexts, (prefix, wanted - contexts)
    return {"counts": counts, "stored_contexts": sorted(contexts), "metric_names": sorted(metrics)}


def cost(lab, rounds):
    results = {}
    orders = [("baseline", "full", "narrow"), ("narrow", "baseline", "full"), ("full", "narrow", "baseline")]
    for block in range(rounds):
        for mode in orders[block % 3]:
            name = f"cost-{block + 1}-{mode}"
            cid = lab.create(name, ["--network", lab.network, "--cpus", "2", "-p", "127.0.0.1::8080",
                                   "netdata-java-spike:21"])
            lab.start(cid)
            wait_http(lab.base_url(cid, 8080) + "/work")
            result = {"mode": mode, "block": block + 1, "pre_warmup": paced(lab, name + "-pre", cid, 10)}
            if mode != "baseline":
                result["attach"] = lab.probe(cid, "agent", "/lab/otel.jar", options(lab, name, mode == "narrow"))
                assert result["attach"]["status"] == 0, result["attach"]
            result["warmup"] = paced(lab, name + "-warm", cid, 20)
            result["ticks_per_second"] = int(lab.docker("exec", cid, "getconf", "CLK_TCK").stdout)
            result["before"] = process_sample(lab, cid)
            result["workload"] = paced(lab, name + "-measure", cid, 30)
            result["after"] = process_sample(lab, cid)
            result["cpu_seconds"] = (result["after"]["cpu_ticks"] - result["before"]["cpu_ticks"]) / result["ticks_per_second"]
            result["cpu_ms_per_request"] = result["cpu_seconds"] * 1000 / result["workload"]["requests"]
            time.sleep(3)
            result["coverage"] = lab.capture(name)
            if mode != "baseline":
                expected = {code: result["warmup"]["statuses"][code] + result["workload"]["statuses"][code]
                            for code in ("200", "503")}
                result["verified"] = verify_capture(lab.output, name, expected)
            else:
                assert not result["coverage"]["metrics"]
            results[name] = result
            lab.save("cost-results.json", results)
            lab.docker("stop", "--time", "10", cid)
    return results


def events(lab, monitor):
    result = lab.docker("logs", monitor)
    return [line.split("\t") for line in result.stdout.splitlines() if line.startswith("SCOUT\t")]


def await_events(lab, monitor, predicate, seconds=90):
    deadline = time.monotonic() + seconds
    while time.monotonic() < deadline:
        found = events(lab, monitor)
        if predicate(found):
            return found
        time.sleep(2)
    raise RuntimeError("scout event deadline exceeded")


def automatic(lab):
    targets = {}
    first = None
    for uid, flags in ((10001, []), (10002, []), (10003, ["-XX:-EnableDynamicAgentLoading"]),
                       (10004, ["-Djava.io.tmpdir=/app-tmp"])):
        args = ["--network", lab.network, "--user", f"{uid}:{uid}", "-p", "127.0.0.1::8080"]
        if first:
            args += ["--pid", "container:" + first]
        if uid == 10004:
            args += ["--read-only", "--tmpfs", "/app-tmp:rw,mode=1777"]
        cid = lab.create(f"plain-{uid}", [*args, PLAIN, *flags])
        if first is None:
            first = cid
        lab.start(cid)
        wait_http(lab.base_url(cid, 8080) + "/work")
        # No agent/helper is in the image and no monitoring config was passed to the app.
        inventory = lab.docker("exec", cid, "find", "/app", "-type", "f").stdout.splitlines()
        assert inventory == ["/app/app.jar"], inventory
        targets[uid] = cid
    config = options(lab, "overridden-by-discovery", narrow=True)
    monitor = lab.create("scout", ["--network", lab.network, "--pid", "container:" + first,
        "--cap-add", "SYS_PTRACE", "-e", "SCOUT_LAB_SCOPE=owned-fixture-pid-namespace",
        "-e", "SCOUT_RUN=" + lab.token, "-e", "SCOUT_OPTIONS=" + config, MONITOR])
    lab.start(monitor)
    found = await_events(lab, monitor, lambda ev: len([e for e in ev if e[2] in ("acknowledged", "failed")]) == 4)
    assert {e[3] for e in found if e[2] == "acknowledged"} == {"10001", "10002"}, found
    assert {e[3] for e in found if e[2] == "failed"} == {"10003", "10004"}, found
    result = {"initial_events": found, "initial": {}}
    for uid in (10001, 10002):
        name = f"java-app-{uid}"
        workload = paced(lab, f"auto-load-{uid}", targets[uid], 20)
        time.sleep(3)
        coverage = lab.capture(name, "initial")
        verify_capture(lab.output, name, workload["statuses"], "initial")
        result["initial"][str(uid)] = {"workload": workload, "coverage": coverage}
    for uid in (10003, 10004):
        assert not metrics_summary(lab.recording_dir / "metrics.json", f"java-app-{uid}")["metrics"]
        assert request(lab.base_url(targets[uid], 8080) + "/work")[0] == 200
    # Restart only the monitor; its bounded, local attempt journal is preserved by this container restart.
    lab.docker("restart", "--time", "5", monitor)
    time.sleep(5)
    assert len([e for e in events(lab, monitor) if e[2] == "attempt"]) == 4
    result["monitor_restart_attempts"] = 4
    # Verify exact identity immediately before stopping this task-owned app child. PID 1 supervisor remains.
    old = next(e for e in found if e[2] == "acknowledged" and e[3] == "10001")
    pid, start = old[1].split(":")
    raw = lab.docker("exec", targets[10001], "cat", f"/proc/{pid}/stat", f"/proc/{pid}/cmdline").stdout
    stat = raw.splitlines()[0].rsplit(") ", 1)[1].split()
    assert stat[19] == start and "\x00-jar\x00/app/app.jar\x00" in raw
    restarted_at = time.time()
    lab.docker("exec", targets[10001], "kill", "-TERM", pid)
    recovered = await_events(lab, monitor, lambda ev: len([e for e in ev if e[2] == "acknowledged"]) == 3)
    latest = [e for e in recovered if e[2] == "acknowledged" and e[3] == "10001"][-1]
    assert latest[1] != old[1]
    wait_http(lab.base_url(targets[10001], 8080) + "/work")
    result["restart_workload"] = paced(lab, "auto-restarted", targets[10001], 25)
    time.sleep(3)
    result["recovered"] = lab.capture("java-app-10001", "restarted")
    verify_capture(lab.output, "java-app-10001", checkpoint="restarted")
    old_ids = result["initial"]["10001"]["coverage"]["latest_sample_by_instance"]
    new_ids = result["recovered"]["latest_sample_by_instance"]
    assert any(key not in old_ids and stamp > restarted_at for key, stamp in new_ids.items()), (old_ids, new_ids)
    result["events"] = recovered
    result["restart_at"] = restarted_at
    result["monitor_stats"] = lab.docker("stats", "--no-stream", "--format", "{{json .}}", monitor).stdout.strip()
    lab.save("automatic-results.json", result)
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--automatic", action="store_true")
    parser.add_argument("--cost-rounds", type=int, default=0)
    parser.add_argument("--build", action="store_true")
    args = parser.parse_args()
    if not args.automatic and args.cost_rounds < 1:
        parser.error("select --automatic and/or --cost-rounds")
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=False)
    lab = Lab(output)
    try:
        if args.build:
            for target, tag in (("application", PLAIN), ("monitor", MONITOR)):
                lab.docker("build", "-f", "Dockerfile.automation", "--target", target, "-t", tag, ".", timeout=900)
        lab.setup()
        lab.save("automation-images.json", json.loads(lab.docker("image", "inspect", MONITOR, PLAIN).stdout))
        if args.automatic:
            automatic(lab)
        if args.cost_rounds:
            cost(lab, args.cost_rounds)
    finally:
        lab.close()


if __name__ == "__main__":
    try:
        main()
    except subprocess.CalledProcessError as error:
        print(error.stderr, file=sys.stderr)
        sys.exit(error.returncode)
