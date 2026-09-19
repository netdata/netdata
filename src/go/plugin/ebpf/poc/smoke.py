#!/usr/bin/env python3
"""Exercise the real plugin, DynCfg protocol and Docker VM kernel without Netdata."""

import argparse
import json
import os
from pathlib import Path
import platform
import pwd
import queue
import shlex
import shutil
import subprocess
import sys
import tempfile
import threading
import time


def bpf_programs():
    result = subprocess.run(["bpftool", "-j", "prog", "show"], text=True, capture_output=True, check=True)
    return {p["id"] for p in json.loads(result.stdout)}


class Plugin:
    def __init__(self, config, varlib, binary, user):
        env = dict(os.environ, NETDATA_LIB_DIR=str(varlib))
        args = [str(binary)]
        if config is None:
            # Exercise the installation paths compiled into the executable.
            env.pop("NETDATA_USER_CONFIG_DIR", None)
            env.pop("NETDATA_STOCK_CONFIG_DIR", None)
        else:
            env.update(NETDATA_USER_CONFIG_DIR=str(config), NETDATA_STOCK_CONFIG_DIR=str(config / "empty"))
            args.extend(["-c", str(config)])
        identity = {}
        if user:
            account = pwd.getpwnam(user)
            identity = dict(user=account.pw_uid, group=account.pw_gid,
                            extra_groups=os.getgrouplist(user, account.pw_gid))
        print("+ " + shlex.join(args), file=sys.stderr)
        self.proc = subprocess.Popen(args, env=env, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                     stderr=subprocess.PIPE, text=True, bufsize=1, **identity)
        self.lines = queue.Queue()
        self.history = []
        self.errors = []
        self.sequence = 0
        self.readers = [threading.Thread(target=self.read_stdout), threading.Thread(target=self.read_stderr)]
        for reader in self.readers:
            reader.start()

    def read_stdout(self):
        for line in self.proc.stdout:
            self.lines.put(line.rstrip("\n"))
        self.lines.put(None)

    def read_stderr(self):
        for line in self.proc.stderr:
            self.errors.append(line.rstrip("\n"))

    def wait(self, predicate, timeout=30):
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            try:
                line = self.lines.get(timeout=max(0.01, deadline - time.monotonic()))
            except queue.Empty:
                break
            if line is None:
                raise RuntimeError(f"plugin exited with status {self.proc.poll()}")
            self.history.append(line)
            if predicate(line):
                return line
        raise TimeoutError("timed out waiting for plugin protocol output")

    def command(self, command, payload=None, expected=(200, 202)):
        self.sequence += 1
        uid = f"poc-{self.sequence}"
        route = f"config ebpf-poc:collector:cachestat {command}"
        if payload is None:
            wire = f'FUNCTION {uid} 15 "{route}" 0xFFFF "user=poc"\n'
        else:
            wire = (f'FUNCTION_PAYLOAD {uid} 15 "{route}" 0xFFFF "user=poc" application/json\n'
                    + json.dumps(payload) + "\nFUNCTION_PAYLOAD_END\n")
        print(f"DynCfg {command}", file=sys.stderr)
        self.proc.stdin.write(wire)
        self.proc.stdin.flush()
        header = self.wait(lambda line: line.startswith(f"FUNCTION_RESULT_BEGIN {uid} "))
        start = len(self.history)
        self.wait(lambda line: line == "FUNCTION_RESULT_END")
        body = json.loads("\n".join(self.history[start:-1]))
        code = int(shlex.split(header)[2])
        if code not in expected:
            raise RuntimeError(f"DynCfg {command}: status {code}, body {body}")
        return body

    def frame(self):
        self.wait(lambda line: line.startswith("BEGIN ") and "page_cache_events" in line)
        start = len(self.history)
        self.wait(lambda line: line == "END")
        values = {}
        for line in self.history[start:-1]:
            fields = shlex.split(line)
            if fields and fields[0] == "SET":
                values[fields[1]] = int(fields[3])
        if set(values) != {"accessed", "buffer_dirty", "added", "account_dirtied"}:
            raise AssertionError(f"unexpected framework chart dimensions: {values}")
        return values

    def stop(self):
        if self.proc.poll() is None:
            self.proc.stdin.write("QUIT\n")
            self.proc.stdin.flush()
            self.proc.stdin.close()
            try:
                self.proc.wait(timeout=15)
            except subprocess.TimeoutExpired:
                # This Popen object identifies only the plugin launched by this test.
                self.proc.kill()
                self.proc.wait()
                raise RuntimeError("plugin failed to stop within 15 seconds")
        for reader in self.readers:
            reader.join(timeout=2)
        if self.proc.returncode != 0:
            raise RuntimeError(f"plugin exited with status {self.proc.returncode}")


def wait_detached(ids):
    deadline = time.monotonic() + 5
    while time.monotonic() < deadline:
        if not ids.intersection(bpf_programs()):
            return
        time.sleep(0.1)
    raise AssertionError("POC programs remain loaded after collector cleanup")


def workload(directory):
    # Local container filesystem only: no host mount, network or global cache eviction.
    path = directory / "page-cache-workload"
    data = bytes(1024 * 1024)
    with path.open("wb") as stream:
        for _ in range(64):
            stream.write(data)
        stream.flush()
        os.fsync(stream.fileno())
    for _ in range(8):
        with path.open("rb") as stream:
            while stream.read(1024 * 1024):
                pass


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", type=Path, default=Path("/usr/local/bin/ebpf-poc.plugin"))
    parser.add_argument("--installed-config", action="store_true", help="use compiled-in stock configuration paths")
    parser.add_argument("--user", help="launch as this user to test installed setuid permissions")
    args = parser.parse_args()
    baseline = bpf_programs()
    all_owned = set()
    with tempfile.TemporaryDirectory(prefix="ebpf-poc-") as temp:
        directory = Path(temp)
        config = None
        if not args.installed_config:
            config = directory / "config"
            shutil.copytree(Path(__file__).resolve().parents[1] / "config", config)
            (config / "empty").mkdir()
        varlib = directory / "varlib"
        varlib.mkdir()
        plugin = Plugin(config, varlib, args.binary, args.user)
        try:
            discovered = plugin.wait(lambda line: line.startswith("CONFIG ") and
                                     "ebpf-poc:collector:cachestat create" in line and "single" in line)
            if args.installed_config:
                assert "file=" in discovered and "/ebpf-poc/cachestat.conf" in discovered, discovered
            schema = plugin.command("schema")
            assert "update_every" in json.dumps(schema)
            plugin.command("test", {"name": "cachestat", "update_every": 1})
            assert bpf_programs() == baseline, "preflight unexpectedly loaded BPF programs"
            plugin.command("enable")
            first = plugin.frame()
            owned = bpf_programs() - baseline
            all_owned.update(owned)
            assert len(owned) == 4, f"expected four cachestat probes, got {len(owned)} programs"
            assert any("ebpf_poc.page_cache_events" in line and line.startswith("CHART ")
                       for line in plugin.history), "no framework chart declaration"
            assert any("incremental" in line and line.startswith("DIMENSION ")
                       for line in plugin.history), "counter rate algorithm missing"
            workload(directory)
            after = plugin.frame()
            if not any(after[k] > first[k] for k in first):
                after = plugin.frame()
            assert any(after[k] > first[k] for k in first), "real kernel counters did not increase"

            # A candidate test runs alongside the incumbent without a second attachment.
            plugin.command("test", {"name": "cachestat", "update_every": 2})
            assert bpf_programs() - baseline == owned, "config test altered the running probe set"
            update_start = len(plugin.history)
            plugin.command("update", {"name": "cachestat", "update_every": 2})
            plugin.frame()
            declarations = [shlex.split(line) for line in plugin.history[update_start:]
                            if line.startswith("CHART ") and "ebpf_poc.page_cache_events" in line]
            assert any(fields[9] == "2" for fields in declarations), "chart interval was not updated"
            wait_detached(owned)
            updated = bpf_programs() - baseline
            all_owned.update(updated)
            assert len(updated) == 4 and not updated.intersection(owned), "update did not replace native handles"
            current = plugin.command("get")
            assert current["update_every"] == 2, f"updated interval missing: {current}"

            plugin.command("disable")
            wait_detached(updated)
            plugin.command("enable")
            plugin.frame()
            restarted = bpf_programs() - baseline
            all_owned.update(restarted)
            assert len(restarted) == 4, "enable did not reattach four probes"
            plugin.stop()
            wait_detached(all_owned)
            print(json.dumps({
                "result": "PASS", "kernel": platform.release(), "architecture": platform.machine(),
                "binary": str(args.binary), "user": args.user, "installed_config": args.installed_config,
                "first_counters": first, "after_workload": after,
                "probes_per_running_job": 4,
                "checks": ["real CO-RE/libbpf collection", "V2 chart and incremental dimensions",
                           "DynCfg schema/test/enable/update/get/disable/re-enable", "test leaves incumbent attached",
                           "update replaces native handles", "QUIT releases native programs"],
            }, indent=2))
        except Exception:
            print("Recent protocol output:\n" + "\n".join(plugin.history[-35:]), file=sys.stderr)
            print("Recent plugin errors:\n" + "\n".join(plugin.errors[-35:]), file=sys.stderr)
            raise
        finally:
            plugin.stop()


if __name__ == "__main__":
    main()
