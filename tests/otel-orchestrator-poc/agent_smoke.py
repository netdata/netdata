#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""Exercise the orchestrator against an isolated, unclaimed Netdata Docker Agent."""
import argparse
import json
import pathlib
import shlex
import subprocess
import sys
import tempfile
import time
import urllib.parse
import uuid


def run(*args, input=None, quiet=False):
    if not quiet:
        print("+ " + shlex.join(args), file=sys.stderr)
    result = subprocess.run(args, input=input, text=True, capture_output=True)
    if result.returncode:
        raise RuntimeError(f"{args[0]} failed in {pathlib.Path.cwd()} with status {result.returncode}: {result.stderr}")
    if args[:2] == ("docker", "logs"):
        return result.stdout + result.stderr
    return result.stdout


def eventually(check, description, timeout=60):
    deadline = time.monotonic() + timeout
    last_error = None
    while time.monotonic() < deadline:
        try:
            result = check()
            if result:
                return result
        except (RuntimeError, ValueError) as error:
            last_error = error
        time.sleep(0.5)
    raise RuntimeError(f"timed out: {description}; last error: {last_error}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--image", default="netdata/netdata:edge")
    parser.add_argument("--binary-dir", type=pathlib.Path, default=pathlib.Path(".build/linux-amd64"))
    args = parser.parse_args()
    binary_dir = args.binary_dir.resolve(strict=True)
    manager = (binary_dir / "otel-orchestrator.plugin").resolve(strict=True)
    worker = (binary_dir / "otel-worker").resolve(strict=True)
    name = "netdata-otel-orchestrator-poc-" + uuid.uuid4().hex[:12]
    with tempfile.TemporaryDirectory(prefix="netdata-otel-orchestrator-") as directory:
        root = pathlib.Path(directory)
        root.chmod(0o755)
        (root / "otel-orchestrator.plugin").touch()
        (root / "otel-worker").touch()
        (root / "plugins").mkdir()
        wrapper = root / "plugins/otel-orchestrator.plugin"
        # Test-only permissions relay: the stock Agent requires Cloud SSO for
        # DynCfg. Grant anonymous access only to this isolated plugin's CONFIG
        # registrations. The actual distribution retains normal permissions.
        pipeline = shlex.join(["/poc/otel-orchestrator.plugin"]) + ' "$@"'
        pipeline += " | " + shlex.join(["sed", "-u", "/^CONFIG .* create /s/ 0x0000 0x0000$/ 0x0008 0x0008/"])
        wrapper.write_text("#!/bin/bash\nset -o pipefail\n"
                           "printf '%s\\n' " + shlex.quote("+ " + pipeline) + " >&2\n"
                           "if " + pipeline + "; then exit 0; else\n"
                           "  result=$?\n"
                           "  printf 'orchestrator/permissions relay failed in %s with status %s\\n' \"$PWD\" \"$result\" >&2\n"
                           "  exit \"$result\"\nfi\n")
        wrapper.chmod(0o755)
        (root / "netdata.conf").write_text('''[global]
    hostname = otel-orchestrator-poc
[directories]
    plugins = "/usr/libexec/netdata/plugins.d" "/poc/plugins"
[plugins]
    enable running new plugins = no
    otel-orchestrator = yes
    otel = yes
[health]
    enabled = no
[web]
    bind to = 127.0.0.1
[logs]
    daemon = stderr
    collector = stderr
''')
        log = root / "input.log"
        log.write_text("old-prefix-" * 150 + "\n")
        started = False

        def request(path, body=None, allowed=(200, 202), quiet=True):
            command = ["docker", "exec", "-i", name, "curl", "-sS", "--max-time", "15", "-w", "\n%{http_code}"]
            if body is not None:
                command += ["-H", "Content-Type: application/json", "--data-binary", "@-"]
            command += ["http://127.0.0.1:19999" + path]
            result = run(*command, input=None if body is None else json.dumps(body), quiet=quiet)
            content, status = result.rsplit("\n", 1)
            if int(status) not in allowed:
                raise RuntimeError(f"{path}: HTTP {status}: {content[:500]}")
            return json.loads(content)

        def config(action, id, body=None, allowed=(200, 202), **extra):
            return request("/api/v3/config?" + urllib.parse.urlencode(dict(action=action, id=id, **extra)), body, allowed=allowed, quiet=False)

        try:
            run("docker", "create", "--name", name, "--network", "none",
                "--mount", f"type=bind,src={root},dst=/poc,readonly",
                "--mount", f"type=bind,src={manager},dst=/poc/otel-orchestrator.plugin,readonly",
                "--mount", f"type=bind,src={worker},dst=/poc/otel-worker,readonly",
                "-e", "NETDATA_OTEL_POC_ENDPOINT=127.0.0.1:4317",
                "-e", "NETDATA_OTEL_POC_STATE_DIR=/var/lib/netdata/otel-orchestrator",
                "-e", "NETDATA_OTEL_CFG_METRICS_INTERVAL_SECS=1",
                "-e", "NETDATA_OTEL_CFG_LOGS_COMPRESSION_ENABLED=false",
                "--entrypoint", "/usr/sbin/netdata", args.image, "-D", "-c", "/poc/netdata.conf")
            started = True
            run("docker", "start", name)
            tree_path = "/api/v3/config?action=tree&path=/collectors/otel-orchestrator/Jobs"
            def templates_ready():
                jobs = request(tree_path)["tree"].get("/collectors/otel-orchestrator/Jobs", {})
                return jobs if "otel-orchestrator:collector:hostmetrics" in jobs else None
            templates = eventually(templates_ready, "DynCfg template registration")
            assert set(templates["otel-orchestrator:collector:hostmetrics"]["cmds"]) >= {"schema", "add", "test"}
            assert config("schema", "otel-orchestrator:collector:hostmetrics")["jsonSchema"]["properties"]["collection_interval"]
            host = "otel-orchestrator:collector:hostmetrics:host"
            logs = "otel-orchestrator:collector:filelogs:logs"
            config("test", "otel-orchestrator:collector:hostmetrics", {"name": "host", "collection_interval": "1s"}, name="host")
            config("add", "otel-orchestrator:collector:hostmetrics", {"name": "host", "collection_interval": "1s", "service_name": "agent-smoke"}, name="host")
            config("add", "otel-orchestrator:collector:filelogs", {"name": "logs", "include": ["/poc/input.log"], "service_name": "agent-logs"}, name="logs")

            def jobs_ready():
                tree = request(tree_path)["tree"]
                jobs = tree.get("/collectors/otel-orchestrator/Jobs", {})
                return all(jobs.get(id, {}).get("status") == "running" for id in (host, logs))

            eventually(jobs_ready, "daemon enable after ADD")
            def metrics_present():
                charts = request("/api/v1/charts")["charts"]
                return [chart for chart in charts.values() if chart["context"] == "otel.system.memory.usage"]
            charts = eventually(metrics_present, "OTLP metrics in Netdata charts")
            data_path = "/api/v1/data?" + urllib.parse.urlencode(dict(chart=charts[0]["id"], after=-30, points=30))
            eventually(lambda: any(isinstance(v, (float, int)) for row in request(data_path)["data"] for v in row[1:]), "numeric OTLP chart samples")
            print("PASS: host metrics stored in Netdata charts")

            def log_received():
                # Compression is disabled only in this fixture. A marker in
                # the real OTLP plugin's WAL proves delivery and storage;
                # the authenticated Logs UI is outside this unclaimed test.
                with log.open("a") as stream:
                    stream.write("netdata-orchestrator-agent-smoke-unique\n")
                return run("docker", "exec", name, "sh", "-c", "grep -alF netdata-orchestrator-agent-smoke-unique /var/log/netdata/otel/v2/logs/wal/*/*.wal", quiet=True)
            eventually(log_received, "synthetic log stored in Netdata WAL")
            print("PASS: file log stored by Netdata OTLP plugin")
            config("update", host, {"collection_interval": "0s"}, allowed=(422,))
            assert config("get", host)["collection_interval"] == "1s"
            config("update", host, {"name": "host", "collection_interval": "2s", "service_name": "agent-updated"})
            eventually(jobs_ready, "updated pipelines ready")
            assert config("get", host)["service_name"] == "agent-updated"
            run("docker", "restart", "--time", "10", name)
            eventually(jobs_ready, "saved jobs replayed after Agent restart")
            assert config("get", host)["service_name"] == "agent-updated"
            assert config("get", logs)["include"] == ["/poc/input.log"]
            print("PASS: DynCfg persisted and replayed both jobs on Agent restart")
            config("disable", host)
            run("docker", "restart", "--time", "10", name)
            def disabled_replayed():
                jobs = request(tree_path)["tree"].get("/collectors/otel-orchestrator/Jobs", {})
                return jobs.get(host, {}).get("status") == "disabled" and jobs.get(logs, {}).get("status") == "running"
            eventually(disabled_replayed, "disabled decision replayed")
            config("remove", host)
            config("remove", logs)
            eventually(lambda: set(templates_ready()) == {"otel-orchestrator:collector:hostmetrics", "otel-orchestrator:collector:filelogs"}, "jobs removed")
            print("PASS: disabled state replayed; jobs removed through DynCfg")
        except Exception:
            if started:
                print(run("docker", "logs", "--tail", "80", name), file=sys.stderr)
            raise
        finally:
            if started:
                run("docker", "rm", "-f", "-v", name)


if __name__ == "__main__":
    main()
