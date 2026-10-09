#!/usr/bin/env python3
"""Owned-container experiment. Requires Docker and Python 3; no host JVM or Python packages."""
import argparse
import concurrent.futures
import contextlib
import hashlib
import io
import json
from pathlib import Path
import shlex
import subprocess
import sys
import time
import urllib.error
import urllib.request
import urllib.parse
import uuid

ROOT = Path(__file__).resolve().parent
LABEL = "org.netdata.java-spike.run"
COLLECTOR = "otel/opentelemetry-collector-contrib:0.162.0@sha256:39923a8e431bd1f57be82411999d389fcfe40857492e4365456d97a4c1f74be6"
NETDATA = "netdata/netdata@sha256:2dd6963cb15637748985871016af3c52d1a0cc67ea03a5b7fcfe51600e481e9f"
RUNTIMES = {
    "17": "eclipse-temurin:17-jdk@sha256:5d6042fb8cdc14d614e4e421f52e3211fc5aeadddce44cbf9de8ed37c791f824",
    "21": "eclipse-temurin:21-jdk@sha256:3e3c176ffed168beb42c607be9bc1639b466cf00261a0fb04425562c9d0c5c2b",
    "25": "eclipse-temurin:25-jdk@sha256:8c0a84ea11c8f6ed52600fc19f1040121f2a162998e9f50a5faebbbad9172dcc",
    "21-jre": "eclipse-temurin:21-jre@sha256:cff19e6215689161eb6162c11b86b0c60ddf802164f2eaf48d570f8fb79a36c5",
}


def command(args, *, timeout=120, check=True, secrets=()):
    def redact(value):
        for secret in secrets:
            value = value.replace(secret, "[REDACTED]")
        return value

    display = redact(shlex.join(map(str, args)))
    print("+ " + display, file=sys.stderr, flush=True)
    result = subprocess.run(args, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                            timeout=timeout, cwd=ROOT)
    if result.returncode:
        print(f"command failed: cwd=tests/java-monitoring-spike status={result.returncode}", file=sys.stderr)
        if check:
            raise subprocess.CalledProcessError(result.returncode, display, redact(result.stdout), redact(result.stderr))
    return result


def wrapper_test():
    value = "space ; $(false) `false`"
    assert command([sys.executable, "-c", "import sys; print(sys.argv[1])", value]).stdout.strip() == value
    assert command([sys.executable, "-c", "raise SystemExit(23)"], check=False).returncode == 23
    buffer = io.StringIO()
    with contextlib.redirect_stderr(buffer):
        command([sys.executable, "-c", "pass", "synthetic-secret"], secrets=("synthetic-secret",))
        try:
            command([sys.executable, "-c", "import sys; print(sys.argv[1], file=sys.stderr); sys.exit(23)",
                     "synthetic-secret"], secrets=("synthetic-secret",))
        except subprocess.CalledProcessError as error:
            assert error.returncode == 23
            assert "synthetic-secret" not in error.stderr and "[REDACTED]" in error.stderr
        else:
            raise AssertionError("failure status was not preserved")
    assert "synthetic-secret" not in buffer.getvalue() and "[REDACTED]" in buffer.getvalue()
    print("command wrapper checks passed")


def request(url):
    try:
        with urllib.request.urlopen(url, timeout=5) as response:
            return response.status, response.read()
    except urllib.error.HTTPError as error:
        return error.code, error.read()


def wait_http(url, seconds=90):
    deadline = time.monotonic() + seconds
    while time.monotonic() < deadline:
        try:
            if request(url)[0] == 200:
                return
        except (OSError, urllib.error.URLError):
            pass
        time.sleep(0.5)
    raise RuntimeError("fixture HTTP readiness deadline exceeded")


def load(url, seconds, concurrency=16):
    start = time.monotonic()
    deadline = start + seconds

    def worker(index):
        result = []
        number = index
        while time.monotonic() < deadline:
            before = time.monotonic()
            try:
                status, _ = request(url + f"/work?fail={'true' if number % 10 == 0 else 'false'}&holdMs=5")
            except (OSError, urllib.error.URLError):
                status = 0
            result.append((status, (time.monotonic() - before) * 1000))
            number += 1
        return result

    with concurrent.futures.ThreadPoolExecutor(max_workers=concurrency) as pool:
        samples = [sample for group in pool.map(worker, range(concurrency)) for sample in group]
    elapsed = time.monotonic() - start
    durations = sorted(duration for _, duration in samples)
    counts = {}
    for status, _ in samples:
        counts[str(status)] = counts.get(str(status), 0) + 1
    return {"seconds": elapsed, "requests": len(samples), "statuses": counts,
            "requests_per_second": len(samples) / elapsed,
            "p50_ms": durations[len(durations) // 2], "p95_ms": durations[int(len(durations) * .95)]}


def metrics_summary(path, service):
    found = {}
    resources = []
    if not path.exists():
        return {"metrics": {}, "resources": []}
    # The recorder is live; ignore only an unfinished final write, never a malformed complete record.
    for line in path.read_text().splitlines(keepends=True):
        if not line.endswith("\n"):
            continue
        for resource in json.loads(line).get("resourceMetrics", []):
            attrs = {a["key"]: a["value"] for a in resource.get("resource", {}).get("attributes", [])}
            if attrs.get("service.name", {}).get("stringValue") != service:
                continue
            if attrs not in resources:
                resources.append(attrs)
            for scope in resource.get("scopeMetrics", []):
                for metric in scope.get("metrics", []):
                    found[metric["name"]] = metric
    return {"metrics": found, "resources": resources}


def fresh_application_identity(before, after, restarted_at):
    return any(instance not in before and timestamp > restarted_at for instance, timestamp in after.items())


def lifecycle_test():
    before = {"previous-process": 90}
    assert not fresh_application_identity(before, {"previous-process": 110}, 100)
    assert not fresh_application_identity(before, {"new-process": 99}, 100)
    assert fresh_application_identity(before, {"previous-process": 110, "new-process": 115}, 100)
    print("restart identity checks passed")


class Lab:
    def __init__(self, output):
        self.output = output
        self.token = uuid.uuid4().hex[:12]
        self.containers = []
        self.network = None
        self.results = {}

    def docker(self, *args, **kwargs):
        return command(["docker", *args], **kwargs)

    def save(self, name, value):
        (self.output / name).write_text(json.dumps(value, indent=2) + "\n")

    def create(self, name, args):
        cid = self.docker("create", "--label", f"{LABEL}={self.token}", "--name",
                          f"nd-java-{self.token}-{name}", *args).stdout.strip()
        self.containers.append((name, cid))
        self.save("ownership.json", {"run": self.token, "network": self.network, "containers": self.containers})
        return cid

    def start(self, cid):
        self.docker("start", cid)

    def base_url(self, cid, port):
        address = self.docker("port", cid, f"{port}/tcp").stdout.strip().splitlines()[0]
        return "http://" + address

    def snapshot(self, cid):
        return {"proc_stat": self.docker("exec", cid, "cat", "/proc/1/stat").stdout.strip(),
                "proc_status": self.docker("exec", cid, "cat", "/proc/1/status").stdout,
                "docker_stats": self.docker("stats", "--no-stream", "--format", "{{json .}}", cid).stdout.strip()}

    def setup(self):
        self.network = self.docker("network", "create", "--label", f"{LABEL}={self.token}",
                                   f"nd-java-{self.token}").stdout.strip()
        self.netdata = self.create("netdata", ["--network", self.network, "--network-alias", "netdata",
            "-p", "127.0.0.1::19999", "-e", "DO_NOT_TRACK=1", "-e", "NETDATA_DISABLE_CLOUD=1",
            # This pinned image predates the receivers.otlp schema in the current source tree.
            "-e", "NETDATA_OTEL_CFG_ENDPOINT_PATH=0.0.0.0:4317",
            NETDATA])
        self.start(self.netdata)
        self.netdata_url = self.base_url(self.netdata, 19999)
        wait_http(self.netdata_url + "/api/v1/info")
        self.save("netdata-info.json", json.loads(request(self.netdata_url + "/api/v1/info")[1]))
        self.recording_dir = self.output / "recording"
        self.recording_dir.mkdir(mode=0o777)
        self.recording_dir.chmod(0o777)  # The recorder runs under its own container UID.
        self.collector = self.create("collector", ["--network", self.network, "--network-alias", "telemetry",
            "--mount", f"type=bind,src={self.recording_dir},dst=/evidence", COLLECTOR, "--config=/collector.yaml"])
        self.docker("cp", str(ROOT / "collector.yaml"), self.collector + ":/collector.yaml")
        self.start(self.collector)
        images = self.docker("image", "inspect", NETDATA, COLLECTOR, "netdata-java-spike:21").stdout
        self.save("images.json", [{key: image.get(key) for key in ("Id", "RepoDigests", "Architecture", "Os")}
                                   for image in json.loads(images)])

    def agent_options(self, service):
        return ";".join([f"otel.service.name={service}", "otel.exporter.otlp.endpoint=http://telemetry:4318",
            "otel.exporter.otlp.protocol=http/protobuf", "otel.traces.exporter=none", "otel.logs.exporter=none",
            "otel.metric.export.interval=1000", "otel.exporter.otlp.metrics.default.histogram.aggregation=explicit_bucket_histogram"])

    def probe(self, cid, mode, *args, user="10001:10001"):
        start = time.monotonic()
        result = self.docker("exec", "--user", user, cid, "java", "-cp", "/lab", "Probe", mode, "1", *args,
                             check=False, timeout=45)
        return {"seconds": time.monotonic() - start, "status": result.returncode,
                "stdout": result.stdout, "stderr": result.stderr}

    def capture(self, name, checkpoint=""):
        artifact_name = name + ("-" + checkpoint if checkpoint else "")
        path = self.recording_dir / "metrics.json"
        summary = metrics_summary(path, name)
        self.save(artifact_name + "-metrics.json", summary)
        charts = json.loads(request(self.netdata_url + "/api/v1/charts")[1])
        matching = {key: chart for key, chart in charts.get("charts", {}).items()
                    if chart.get("context", "").startswith("otel.")
                    and chart.get("chart_labels", {}).get("resource.attributes.service.name") == name}
        self.save(artifact_name + "-charts.json", matching)
        data = {}
        wanted = ("otel.jvm.memory.used", "otel.http.server.request.duration.count",
                  "otel.http.server.request.duration.bucket", "otel.db.client.connections.usage",
                  "otel.db.client.connections.pending_requests")
        for key, chart in matching.items():
            if chart["context"] in wanted:
                query = urllib.parse.urlencode({"chart": key, "after": -30, "points": 30, "format": "json"})
                status, body = request(self.netdata_url + "/api/v1/data?" + query)
                data[key] = {"status": status, "context": chart["context"], "response": json.loads(body)}
        self.save(artifact_name + "-data.json", data)
        if matching and not summary["metrics"]:
            raise RuntimeError("Netdata has charts but raw telemetry recording is missing")
        sample_times = [row[0] for item in data.values() for row in item["response"].get("data", [])
                        if any(value is not None for value in row[1:])]
        by_instance = {}
        for key, item in data.items():
            instance = matching[key]["chart_labels"].get("resource.attributes.service.instance.id")
            for row in item["response"].get("data", []):
                if instance and any(value is not None for value in row[1:]):
                    by_instance[instance] = max(by_instance.get(instance, 0), row[0])
        return {"metrics": sorted(summary["metrics"]), "netdata_charts": len(matching),
                "latest_sample_time": max(sample_times, default=0),
                "latest_sample_by_instance": by_instance,
                "contexts": sorted({chart["context"] for chart in matching.values()}),
                "charts_with_samples": sum(any(any(value is not None for value in row[1:])
                    for row in item["response"].get("data", [])) for item in data.values())}

    def scenario(self, mode, image, extra_flags=()):
        name = mode + "-" + image.rsplit(":", 1)[-1]
        opts = self.agent_options(name)
        flags = list(extra_flags)
        if mode == "startup":
            flags.append("-javaagent:/lab/otel.jar=" + opts)
        cid = self.create(name, ["--network", self.network, "-p", "127.0.0.1::8080", image,
                                 *flags, "-jar", "/lab/app.jar"])
        self.start(cid)
        url = self.base_url(cid, 8080)
        wait_http(url + "/work")
        result = {"warmup": load(url, 10), "before": self.snapshot(cid)}
        result["java_version"] = self.docker("exec", cid, "java", "-version").stderr
        # The HTTP readiness request and warmup both borrow from the already-created Hikari pool.
        result["jmx"] = self.probe(cid, "jmx")
        if mode == "late":
            result["attach"] = self.probe(cid, "agent", "/lab/otel.jar", opts)
        result["after_attach"] = self.snapshot(cid)
        result["workload"] = load(url, 20)
        result["after_workload"] = self.snapshot(cid)
        result["monitor_stats"] = self.docker("stats", "--no-stream", "--format", "{{json .}}",
                                              self.netdata, self.collector).stdout
        time.sleep(3)
        result["coverage"] = self.capture(name)
        if mode == "startup":
            required = {"jvm.memory.used", "http.server.request.duration", "db.client.connections.usage",
                        "db.client.connections.pending_requests"}
            missing = required - set(result["coverage"]["metrics"])
            if missing or result["coverage"]["charts_with_samples"] == 0:
                self.save(name + "-failed-control.json", result)
                raise RuntimeError(f"startup positive control failed: missing={sorted(missing)}")
        self.results[name] = result
        self.save("results.json", self.results)
        self.docker("stop", "--time", "10", cid)
        return cid, result

    def boundary_checks(self):
        results = {}
        for name, flags in (("attach-disabled", ["-XX:+DisableAttachMechanism"]),
                            ("dynamic-disabled", ["-XX:-EnableDynamicAgentLoading"])):
            cid = self.create(name, ["--network", self.network, "-p", "127.0.0.1::8080",
                                    "netdata-java-spike:21", *flags, "-jar", "/lab/app.jar"])
            self.start(cid)
            url = self.base_url(cid, 8080)
            wait_http(url + "/work")
            results[name] = {"jmx": self.probe(cid, "jmx"),
                            "agent": self.probe(cid, "agent", "/lab/otel.jar", self.agent_options(name)),
                            "app_status_after": request(url + "/work")[0]}
            self.docker("stop", "--time", "10", cid)
            self.save("boundaries.json", results)

        # A JRE-only target plus a separate JDK helper: PID/network namespaces shared, filesystem separate.
        name = "external-helper-jre"
        cid = self.create(name, ["--network", self.network, "-p", "127.0.0.1::8080", "netdata-java-spike:21-jre"])
        self.start(cid)
        url = self.base_url(cid, 8080)
        wait_http(url + "/work")
        load(url, 3)
        results[name] = {}
        for operation, user in (("jmx", "10002:10002"), ("jmx", "10001:10001"), ("agent", "10001:10001")):
            key = operation + "-" + user.split(":")[0]
            extra = ["/lab/otel.jar", self.agent_options(name)] if operation == "agent" else []
            helper = self.create(key, ["--pid", f"container:{cid}", "--network", f"container:{cid}",
                "--user", user, "--entrypoint", "java", "netdata-java-spike:21",
                "-cp", "/lab", "Probe", operation, "1", *extra])
            before = time.monotonic()
            r = self.docker("start", "--attach", helper, check=False, timeout=45)
            code = int(self.docker("inspect", "--format", "{{.State.ExitCode}}", helper).stdout)
            results[name][key] = {"status": code, "seconds": time.monotonic() - before,
                                  "stdout": r.stdout, "stderr": r.stderr}
        results[name]["workload"] = load(url, 15)
        time.sleep(3)
        results[name]["coverage"] = self.capture(name)
        self.save("boundaries.json", results)
        self.docker("stop", "--time", "10", cid)

        self.lifecycle_check(results)

    def lifecycle_check(self, results=None):
        if results is None:
            results = {}
        # Reattaching the same agent and restarting Netdata are distinct lifecycle cases.
        name = "lifecycle"
        cid = self.create(name, ["--network", self.network, "-p", "127.0.0.1::8080", "netdata-java-spike:21",
            "-javaagent:/lab/otel.jar=" + self.agent_options(name), "-jar", "/lab/app.jar"])
        self.start(cid)
        url = self.base_url(cid, 8080)
        wait_http(url + "/work")
        load(url, 5)
        results[name] = {"duplicate_attach": self.probe(cid, "agent", "/lab/otel.jar", self.agent_options(name))}
        load(url, 5)
        results[name]["before_restart"] = self.capture(name, "before")
        if results[name]["before_restart"]["charts_with_samples"] == 0:
            self.save("boundaries.json", results)
            raise RuntimeError("lifecycle precondition failed: no stored samples before restart")
        results[name]["netdata_restart_at"] = time.time()
        self.docker("restart", "--time", "10", self.netdata)
        self.netdata_url = self.base_url(self.netdata, 19999)
        wait_http(self.netdata_url + "/api/v1/info")
        load(url, 15)
        time.sleep(3)
        results[name]["after_netdata_restart"] = self.capture(name, "netdata-restarted")
        results[name]["app_restart_at"] = time.time()
        self.docker("restart", "--time", "10", cid)
        url = self.base_url(cid, 8080)
        wait_http(url + "/work")
        load(url, 15)
        time.sleep(3)
        results[name]["after_app_restart"] = self.capture(name, "app-restarted")
        self.save("boundaries.json", results)
        self.docker("stop", "--time", "10", cid)
        for stage, stamp in (("after_netdata_restart", "netdata_restart_at"), ("after_app_restart", "app_restart_at")):
            if results[name][stage]["latest_sample_time"] <= results[name][stamp]:
                raise RuntimeError("lifecycle check failed: no fresh stored samples " + stage)
        if not fresh_application_identity(results[name]["before_restart"]["latest_sample_by_instance"],
                                          results[name]["after_app_restart"]["latest_sample_by_instance"],
                                          results[name]["app_restart_at"]):
            raise RuntimeError("lifecycle check failed: no fresh samples from the restarted application identity")

    def coexistence_check(self):
        # The application already has a real tracing agent before OTel attaches.
        url = "https://github.com/DataDog/dd-trace-java/releases/download/v1.67.1/dd-java-agent.jar"
        print("+ download pinned public Datadog agent " + url, file=sys.stderr, flush=True)
        with urllib.request.urlopen(url, timeout=60) as response:
            agent = response.read(64 * 1024 * 1024)
        if hashlib.sha256(agent).hexdigest() != "beb620dc2e4783fd328a1ae23f8fd4fcb757ac23e5bb05efb04c6aa3e51a5e43":
            raise RuntimeError("Datadog agent checksum mismatch")
        path = self.output / "dd-javaagent.jar"
        path.write_bytes(agent)
        name = "coexistence"
        flags = ["-javaagent:/lab/dd.jar", "-Ddd.service=existing-datadog",
                 "-Ddd.trace.agent.url=http://127.0.0.1:9", "-Ddd.remote_config.enabled=false",
                 "-Ddd.instrumentation.telemetry.enabled=false", "-Ddd.jmxfetch.enabled=false",
                 "-Ddd.profiling.enabled=false", "-Ddd.appsec.enabled=false", "-Ddd.data.streams.enabled=false"]
        cid = self.create(name, ["--network", self.network, "-p", "127.0.0.1::8080", "netdata-java-spike:21",
                                *flags, "-jar", "/lab/app.jar"])
        self.docker("cp", str(path), cid + ":/lab/dd.jar")
        self.start(cid)
        url = self.base_url(cid, 8080)
        wait_http(url + "/work")
        result = {"warmup": load(url, 5),
                  "attach": self.probe(cid, "agent", "/lab/otel.jar", self.agent_options(name))}

        # Both JVMs use PID 1, so PID alone cannot distinguish their metrics.
        control_name = "concurrent-control"
        control = self.create(control_name, ["--network", self.network, "-p", "127.0.0.1::8080",
            "netdata-java-spike:21", "-javaagent:/lab/otel.jar=" + self.agent_options(control_name),
            "-jar", "/lab/app.jar"])
        self.start(control)
        control_url = self.base_url(control, 8080)
        wait_http(control_url + "/work")
        with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
            workloads = list(pool.map(lambda address: load(address, 15), [url, control_url]))
        result["workloads"] = workloads
        time.sleep(3)
        result["coverage"] = self.capture(name)
        result["concurrent_control"] = self.capture(control_name)
        self.save("coexistence.json", result)
        for target in (cid, control):
            self.docker("stop", "--time", "10", target)

    def close(self):
        for name, cid in reversed(self.containers):
            log = self.docker("logs", cid, check=False)
            (self.output / (name + ".log")).write_text(log.stdout + log.stderr)
            label = self.docker("inspect", "--format", '{{index .Config.Labels "' + LABEL + '"}}', cid,
                                check=False).stdout.strip()
            if label == self.token:
                self.docker("rm", "-f", "-v", cid, check=False)
        if self.network:
            self.docker("network", "rm", self.network, check=False)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--self-test", action="store_true")
    parser.add_argument("--output", type=Path)
    parser.add_argument("--images", nargs="*", default=["netdata-java-spike:21"])
    parser.add_argument("--boundaries", action="store_true")
    parser.add_argument("--build", action="store_true", help="build all four pinned fixture images before running")
    parser.add_argument("--coexistence", action="store_true")
    parser.add_argument("--lifecycle-only", action="store_true")
    args = parser.parse_args()
    if args.self_test:
        wrapper_test()
        lifecycle_test()
        return
    output = (args.output or ROOT.parents[1] / ".local/java-monitoring-spike" / time.strftime("%Y%m%d-%H%M%S")).resolve()
    output.mkdir(parents=True, exist_ok=False)
    lab = Lab(output)
    try:
        if args.build:
            for version, image in RUNTIMES.items():
                lab.docker("build", "--build-arg", "RUNTIME_IMAGE=" + image, "-t", "netdata-java-spike:" + version,
                           ".", timeout=900)
        lab.setup()
        for image in args.images:
            for mode in ("baseline", "late", "startup"):
                lab.scenario(mode, image)
        if args.boundaries:
            lab.boundary_checks()
        if args.coexistence:
            lab.coexistence_check()
        if args.lifecycle_only:
            lab.lifecycle_check()
    finally:
        lab.close()
    print(json.dumps({key: value["coverage"] for key, value in lab.results.items()}, indent=2))


if __name__ == "__main__":
    try:
        main()
    except subprocess.CalledProcessError as error:
        print(error.stderr, file=sys.stderr)
        sys.exit(error.returncode)
