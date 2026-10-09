#!/usr/bin/env python3
"""Validate installed Java artifacts using owned workloads and an isolated prefix on a JVM-free Linux host."""
import argparse
import concurrent.futures
import hashlib
import json
import math
import contextlib
import io
import os
from pathlib import Path
import shlex
import socket
import subprocess
import sys
import time
import urllib.parse
import urllib.request
import uuid

CONTEXTS = {"java." + name for name in ("jvm_memory_used", "http_requests", "http_request_duration",
            "pool_connections", "pool_pending_requests", "pool_connection_limit")}


def run(args, timeout=60, check=True, secrets=()):
    args = list(map(str, args))
    def redact(value):
        for secret in secrets:
            value = value.replace(secret, "[REDACTED]")
        return value
    display = redact(shlex.join(args))
    print("+ " + display, file=sys.stderr, flush=True)
    result = subprocess.run(args, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=timeout)
    if check and result.returncode:
        print(f"Operation failed: cwd={os.getcwd()} status={result.returncode}: {redact(result.stderr)}", file=sys.stderr)
        raise subprocess.CalledProcessError(result.returncode, display)
    return result


def free_port():
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


class NativeLab:
    def __init__(self, args):
        self.args = args
        self.prefix = args.prefix.resolve()
        self.output = args.output.resolve()
        self.output.mkdir(parents=True, exist_ok=False)
        self.token = uuid.uuid4().hex[:12]
        self.marker = "Netdata Java native validation " + self.token
        self.units = []
        self.active = set()
        self.work = self.prefix / ("validation-" + self.token)
        self.plugins = self.prefix / "usr/libexec/netdata/plugins.d"
        self.java = self.prefix / "usr/share/netdata/java/runtime/bin/java"
        self.port = free_port()
        self.ports = {}
        for name in ("checkout", "inventory", "blocked", "root-owned"):
            candidate = free_port()
            while candidate == self.port or candidate in self.ports.values():
                candidate = free_port()
            self.ports[name] = candidate
        self.url = "http://127.0.0.1:" + str(self.port)

    def save(self, name, data):
        (self.output / (name + ".json")).write_text(json.dumps(data, indent=2) + "\n")

    def write(self, path, content, mode="644"):
        # Only generated configuration and the owned protocol proxy use this writer.
        staging = self.output / ("generated-" + path.name)
        staging.write_text(content)
        run(["sudo", "-n", "install", "-m", mode, staging, path])

    def unit(self, name, command, user="root", private_tmp=False):
        unit = "nd-java-check-" + self.token + "-" + name + ".service"
        existing = run(["sudo", "-n", "systemctl", "show", unit, "--property=LoadState", "--value"], check=False)
        if existing.stdout.strip() != "not-found":
            if unit not in self.units or unit in self.active:
                raise RuntimeError("Unit already exists or ownership is unknown: " + unit)
            description = run(["sudo", "-n", "systemctl", "show", unit, "--property=Description", "--value"]).stdout.strip()
            if description != self.marker:
                raise RuntimeError("Unit ownership changed: " + unit)
        if unit not in self.units:
            self.units.append(unit)
        options = ["sudo", "-n", "systemd-run", "--unit", unit, "--description", self.marker,
                   "--property=Type=exec", "--property=RuntimeMaxSec=900", "--property=TimeoutStopSec=30",
                   "--property=CPUQuota=200%", "--property=MemoryMax=768M", "--property=TasksMax=256",
                   "--property=User=root", "--property=Group=root"]
        if private_tmp:
            options += ["--property=PrivateTmp=yes"]
        if name != "daemon":
            options += ["--property=NoNewPrivileges=yes"]
        if user != "root":
            # Numeric fixture identities intentionally have no NSS account.
            command = ["/usr/bin/setpriv", "--reuid=" + str(user), "--regid=" + str(user),
                       "--clear-groups", "--no-new-privs", *command]
        run([*options, *command])
        self.active.add(unit)
        self.save("ownership", {"marker": self.marker, "units": self.units, "prefix": str(self.prefix)})
        return unit

    def stop(self, unit):
        if unit not in self.active:
            return
        description = run(["sudo", "-n", "systemctl", "show", unit, "--property=Description", "--value"]).stdout.strip()
        if description != self.marker:
            raise RuntimeError("Unit description differs; refusing to stop " + unit)
        run(["sudo", "-n", "systemctl", "stop", unit])
        self.active.remove(unit)
        run(["sudo", "-n", "systemctl", "reset-failed", unit], check=False)

    def setup(self):
        # Inspect executable names as root; do not print arbitrary host command lines.
        scan = "from pathlib import Path\nimport os\nfor p in Path('/proc').iterdir():\n if p.name.isdecimal():\n  try:\n   if Path(os.readlink(p/'exe').removesuffix(' (deleted)')).name == 'java': print(p.name)\n  except OSError: pass\n"
        existing = run(["sudo", "-n", "python3", "-c", scan]).stdout.strip()
        if existing:
            raise RuntimeError("Existing Java PIDs found; refusing automatic attachment: " + existing.replace("\n", ", "))
        for artifact in (self.plugins / "java.plugin", self.plugins / "java-helper", self.plugins / "ndsudo", self.java):
            if not artifact.is_file():
                raise RuntimeError("Missing installed artifact: " + str(artifact))
        run(["sudo", "-n", "-u", "netdata", "test", "-x", self.plugins / "java.plugin"])
        for part in (self.prefix, *self.prefix.parents):
            stat = part.stat()
            if stat.st_uid != 0 or stat.st_mode & 0o022:
                raise RuntimeError("Install prefix ancestry must be root-owned and not writable: " + str(part))
        run(["id", "netdata"])
        run(["sudo", "-n", "install", "-d", "-m", "755", self.work, self.work / "plugins", self.work / "config", self.work / "config/java"])
        for name in ("cache", "varlib", "log", "recording"):
            run(["sudo", "-n", "install", "-d", "-o", "netdata", "-g", "netdata", "-m", "700", self.work / name])
        proxy = "#!/bin/bash\nset -o pipefail\n" + shlex.quote(str(self.plugins / "java.plugin")) + ' "$@" | tee ' + shlex.quote(str(self.work / "recording/protocol.out")) + "\nexit $?\n"
        self.write(self.work / "plugins/java.plugin", proxy, "755")
        config = f"""[global]
    run as user = netdata
    hostname = java-native-test
[directories]
    config = {self.work / 'config'}
    stock config = {self.prefix / 'usr/lib/netdata/conf.d'}
    cache = {self.work / 'cache'}
    lib = {self.work / 'varlib'}
    log = {self.work / 'log'}
    home = {self.work / 'varlib'}
    plugins = {self.work / 'plugins'}
[logs]
    daemon = stderr
    collector = stderr
    health = stderr
    access = none
    aclk = none
[db]
    mode = ram
[plugins]
    enable running new plugins = no
    java = yes
    proc = no
    diskspace = no
    cgroups = no
    tc = no
    idlejitter = no
    statsd = no
[web]
    bind to = 127.0.0.1
    default port = {self.port}
    allow connections from = localhost 127.*
    allow dashboard from = localhost 127.*
    allow management from = localhost 127.*
    bearer token protection = no
[cloud]
    enabled = no
"""
        self.write(self.work / "config/netdata.conf", config)
        self.write(self.work / "config/.opt-out-from-anonymous-statistics", "")
        self.write(self.work / "config/java.conf", "enabled: yes\ndefault_run: no\nmodules:\n  java: yes\n")
        self.job_config()
        for name in self.ports:
            self.application(name)
        self.start_daemon()
        self.save("environment", {"environment": "native Linux systemd units", "prefix": str(self.prefix),
            "url": self.url, "fixture_uids": {"checkout": 10001, "inventory": 10002, "blocked": 10001, "root-owned": 0},
            "inventory_private_tmp": True, "authenticated_dyncfg_ui_verified": False})

    def job_config(self, rename=False, exclude=False):
        config = {"jobs": [{"name": "java", "update_every": 1}]}
        if rename:
            config["jobs"][0]["application_names"] = {"checkout": "Checkout API"}
        if exclude:
            config["jobs"][0]["exclude_applications"] = ["inventory"]
        # JSON is valid YAML and avoids introducing a PyYAML dependency.
        self.write(self.work / "config/java/java.conf", json.dumps(config))

    def application(self, name):
        args = [self.java, "-Xms128m", "-Xmx256m"]
        if name == "blocked":
            args.append("-XX:+DisableAttachMechanism")
        args += ["-jar", self.args.app.resolve(), "--spring.application.name=" + name, "--server.port=" + str(self.ports[name])]
        uid = {"inventory": "10002", "root-owned": "root"}.get(name, "10001")
        self.unit(name, args, uid, private_tmp=name == "inventory")
        self.wait(lambda: self.http("http://127.0.0.1:" + str(self.ports[name]) + "/work"), "application " + name)

    @staticmethod
    def http(url):
        with urllib.request.urlopen(url, timeout=10) as response:
            return response.read()

    def api(self, path):
        return json.loads(self.http(self.url + path))

    def wait(self, predicate, description, seconds=100):
        deadline = time.monotonic() + seconds
        last = None
        while time.monotonic() < deadline:
            try:
                value = predicate()
                if value:
                    return value
            except (OSError, ValueError, KeyError) as error:
                last = str(error)
            time.sleep(1)
        raise RuntimeError(description + " deadline exceeded: " + str(last))

    def start_daemon(self):
        self.daemon = self.unit("daemon", [self.args.netdata.resolve(), "-D", "-u", "netdata", "-c", self.work / "config/netdata.conf"])
        self.wait(lambda: self.api("/api/v1/info"), "Netdata readiness")

    def restart_daemon(self):
        self.stop(self.daemon)
        self.start_daemon()
        self.wait_collecting()

    def rows(self):
        result = self.api("/api/v3/function?function=java%3Aapplications")
        columns = sorted(result["columns"], key=lambda c: result["columns"][c]["index"])
        return [dict(zip(columns, row)) for row in result["data"]]

    def wait_collecting(self):
        return self.wait(lambda: (rows if sum(r["Status"] == "Collecting" and r["Application"] in
            {"checkout", "Checkout API", "inventory"} for r in rows) == 2 else None) if (rows := self.rows()) else None,
            "two collecting applications")

    def state(self):
        data = json.loads(run(["sudo", "-n", "cat", self.work / "varlib/java/state.json"]).stdout)
        return {"port": data["port"], "attempts": {key: {"process": value["process"], "status": value["status"],
                    "credential_fingerprint": hashlib.sha256(value["token"].encode()).hexdigest()}
                for key, value in data["attempts"].items()}}

    def workload(self, name):
        result = run(["sudo", "-n", self.java, "-Xms16m", "-Xmx64m", "-cp", self.args.load.resolve(),
                      "HttpLoad", "http://127.0.0.1:" + str(self.ports[name]), "15", "100"], timeout=100)
        value = json.loads(result.stdout)
        assert value["requests"] == 1500 and value["errors"] == 0, value
        assert value["statuses"] == {"200": 1350, "503": 150}, value
        return value

    def capture(self, name, applications=("checkout", "inventory")):
        charts = {key: c for key, c in self.api("/api/v1/charts")["charts"].items() if c["context"].startswith("java.")}
        samples = {key: self.api("/api/v1/data?" + urllib.parse.urlencode({"chart": key, "after": -30,
            "points": 30, "format": "json", "options": "seconds"})) for key in charts}
        stamp = time.time()
        for app in applications:
            selected = {key: c for key, c in charts.items() if c["chart_labels"].get("application") == app}
            assert {c["context"] for c in selected.values()} == CONTEXTS, (app, selected)
            for key in selected:
                sample = samples[key]
                for column in range(1, len(sample["labels"])):
                    values = [(row[0], row[column]) for row in sample["data"] if row[column] is not None]
                    assert values and 0 <= stamp - max(t for t, _ in values) <= 15, key
                    assert all(math.isfinite(value) and value >= 0 for _, value in values), key
                chart = selected[key]
                if chart["context"] == "java.http_request_duration":
                    assert chart["chart_type"] == "heatmap" and chart["units"] == "observations/s", key
            if name == "initial":
                rates = {}
                for key, chart in selected.items():
                    if chart["context"] == "java.http_requests":
                        sample = samples[key]
                        for column, status in enumerate(sample["labels"][1:], 1):
                            rates[status] = max(rates.get(status, 0), max(
                                (row[column] for row in sample["data"] if row[column] is not None), default=0))
                assert set(rates) == {"200", "503"} and all(value > 0 for value in rates.values()), (app, rates)
        value = {"rows": self.rows(), "charts": charts, "samples": samples, "time": stamp, "state": self.state()}
        self.save(name, value)
        return value

    def checks(self):
        self.wait(lambda: any(r["Application"] == "blocked" and r["Status"] == "Blocked" for r in self.rows()), "blocked JVM")
        self.wait(lambda: any(r["Application"] == "root-owned" and r["Status"] == "Unsupported" for r in self.rows()), "unsupported root JVM")
        with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
            self.save("initial-workloads", dict(zip(("checkout", "inventory"), pool.map(self.workload, ("checkout", "inventory")))))
        self.wait_collecting()
        time.sleep(6)
        initial = self.capture("initial")
        initial_state = initial["state"]
        self.restart_daemon()
        time.sleep(6)
        restarted = self.capture("daemon-restarted")
        assert initial_state == restarted["state"], "Daemon restart changed durable attachment state"
        self.stop(self.daemon)
        self.job_config(rename=True)
        self.start_daemon()
        self.wait_collecting()
        time.sleep(6)
        renamed = self.capture("renamed")
        assert set(initial["charts"]) == set(renamed["charts"])
        assert all(c["chart_labels"].get("application_name") == "Checkout API" for c in renamed["charts"].values()
                   if c["chart_labels"].get("application") == "checkout")
        assert initial_state == renamed["state"]
        self.stop(self.daemon)
        self.job_config(exclude=True)
        self.start_daemon()
        self.wait(lambda: any(r["Application"] == "inventory" and r["Status"] == "Excluded" for r in self.rows()), "excluded inventory")
        time.sleep(6)
        excluded = self.capture("excluded", ("checkout",))
        assert not any(c["chart_labels"].get("application") == "inventory" for c in excluded["charts"].values())
        assert initial_state == excluded["state"]
        self.stop(self.daemon)
        self.job_config()
        self.start_daemon()
        self.wait_collecting()
        before = next(r["Instance"] for r in initial["rows"] if r["Application"] == "checkout")
        self.stop("nd-java-check-" + self.token + "-checkout.service")
        self.application("checkout")
        self.wait(lambda: any(r["Application"] == "checkout" and r["Instance"] != before for r in self.rows()), "new process identity")
        self.save("restart-workload", self.workload("checkout"))
        self.wait_collecting()
        time.sleep(6)
        final = self.capture("app-restarted")
        assert before not in {r["Instance"] for r in final["rows"]}
        self.save("checks", {"passed": True, "native_vm_verified": True, "private_tmp_verified": True,
            "daemon_restart_preserves_state_and_receiver": True, "root_jvm_unsupported": True,
            "disabled_attach_blocked": True, "rename_preserves_chart_identity": True,
            "exclusion_removes_charts": True, "application_restart_changes_identity": True,
            "authenticated_dyncfg_ui_verified": False, "cumulative_http_count_equivalence_verified": False})

    def close(self):
        errors = []
        for unit in reversed(self.units):
            try:
                logs = run(["sudo", "-n", "journalctl", "-u", unit, "--no-pager", "-n", "150"], check=False)
                (self.output / (unit + ".log")).write_text(logs.stdout)
                self.stop(unit)
            except Exception as error:
                errors.append(str(error))
        if errors:
            raise RuntimeError("Owned-unit cleanup failed: " + "; ".join(errors))


def self_test():
    literal = "space ; $(false) `false`"
    assert run([sys.executable, "-c", "import sys; print(sys.argv[1])", literal]).stdout.strip() == literal
    assert run([sys.executable, "-c", "raise SystemExit(23)"], check=False).returncode == 23
    captured = io.StringIO()
    with contextlib.redirect_stderr(captured):
        try:
            run([sys.executable, "-c", "import sys; print(sys.argv[1], file=sys.stderr); sys.exit(23)",
                 "synthetic-secret"], secrets=("synthetic-secret",))
        except subprocess.CalledProcessError as error:
            assert error.returncode == 23 and "synthetic-secret" not in str(error)
        else:
            raise AssertionError("Nonzero status not preserved")
    assert "synthetic-secret" not in captured.getvalue() and "[REDACTED]" in captured.getvalue()
    from types import SimpleNamespace
    from unittest.mock import patch
    lab = NativeLab.__new__(NativeLab)
    lab.active = {"owned.service"}
    lab.marker = "expected description"
    with patch(__name__ + ".run", return_value=SimpleNamespace(stdout="unrelated description")) as mocked:
        try:
            lab.stop("owned.service")
        except RuntimeError:
            pass
        else:
            raise AssertionError("Mismatched unit ownership was accepted")
        assert mocked.call_count == 1
    print("Command-wrapper and unit-ownership checks passed")


def main():
    if sys.argv[1:] == ["--self-test"]:
        self_test()
        return
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("prefix", "netdata", "app", "load", "output"):
        parser.add_argument("--" + name, required=True, type=Path)
    parser.add_argument("--hold-seconds", type=int, default=0, choices=range(301), metavar="0..300")
    lab = NativeLab(parser.parse_args())
    try:
        lab.setup()
        lab.checks()
        print("Native plugin checks passed", flush=True)
        deadline = time.monotonic() + lab.args.hold_seconds
        while time.monotonic() < deadline and not (lab.output / "continue").exists():
            time.sleep(1)
    finally:
        lab.close()


if __name__ == "__main__":
    try:
        main()
    except subprocess.CalledProcessError as error:
        sys.exit(error.returncode if error.returncode > 0 else 128 - error.returncode)
