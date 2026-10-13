#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""Run a disposable Unix Agent to verify native check health transitions.

Supply built Agent and scripts.d binaries (scripts.d requires scripts_native_dev).
The plugin must resolve a usable nd-run helper (see NATIVE.md). No installed Agent is contacted or modified.
"""
import argparse
import json
import os
import pathlib
import shlex
import shutil
import socket
import subprocess
import sys
import tempfile
import time
import urllib.request


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--agent", required=True, type=pathlib.Path)
    parser.add_argument("--plugin", required=True, type=pathlib.Path)
    parser.add_argument("--mode", choices=("oneshot", "persistent"), default="oneshot")
    args = parser.parse_args()
    agent, plugin = args.agent.resolve(), args.plugin.resolve()
    if os.name != "posix" or not pathlib.Path("/bin/bash").is_file():
        parser.error("this smoke test requires Unix and /bin/bash")
    for binary in (agent, plugin):
        if not binary.is_file() or not os.access(binary, os.X_OK):
            parser.error(f"not executable: {binary}")
    true_command = shutil.which("true")
    if not true_command:
        parser.error("a true executable is required to suppress notifications")

    # Keep paths short enough for the Agent's Unix-domain spawn-server sockets.
    root = pathlib.Path(tempfile.mkdtemp(prefix="nd-native-", dir="/tmp"))
    repo = pathlib.Path(__file__).resolve().parents[5]
    for name in (
        "etc/health.d", "etc/scripts.d", "stock/scripts.d", "log", "cache",
        "lib", "web", "plugins", "run", "share",
    ):
        (root / name).mkdir(parents=True)
    (root / "etc/health.d/native_script.conf").write_bytes(
        (repo / "src/go/plugin/scripts.d/development/native_script.conf").read_bytes()
    )
    (root / "etc/scripts.d.conf").write_text(
        "enabled: yes\ndefault_run: yes\nmodules:\n  native: yes\n  nagios: no\n"
    )
    helper = repo / "src/go/plugin/scripts.d/lib/native.sh"
    failure_action = "nd_fail; return" if args.mode == "persistent" else "return 1"
    script = (
        "#!/bin/bash\nset -eu\n"
        f"source {shlex.quote(str(helper))}\n"
        "collect_snapshot() {\n"
        f"    state=$(cat {shlex.quote(str(root / 'state'))})\n"
        "    if [[ $state == collection_failed ]]; then\n"
        f"        printf '%s\\n' failed >> {shlex.quote(str(root / 'failed-attempts'))}\n"
        f"        {failure_action}\n"
        "    fi\n"
        "    nd_begin\n"
        "    if [[ $state != omitted ]]; then\n"
        "        nd_check probe Probe queue\n"
        "        nd_check_sample \"$ND_FAMILY\" \"$state\" queue 'mail\\'\n"
        "    fi\n"
        "    nd_end\n"
        "}\n"
    )
    if args.mode == "persistent":
        script += (
            "[[ $1 == serve ]]\n"
            f"printf '%s\\n' \"$$\" >> {shlex.quote(str(root / 'launches'))}\n"
            "nd_ready\ncount=0\nwhile nd_next; do\n"
            "    count=$((count+1))\n"
            f"    printf '%s' \"$count\" > {shlex.quote(str(root / 'exchanges'))}\n"
            "    collect_snapshot\ndone\n"
        )
    else:
        script += "[[ $1 == collect ]]\ncollect_snapshot\n"
    (root / "collect.sh").write_text(script)
    (root / "etc/scripts.d/native.conf").write_text(
        "jobs:\n  - name: health_probe\n"
        f"    command: [/bin/bash, {root}/collect.sh]\n"
        f"    mode: {args.mode}\n"
        "    update_every: 1\n    timeout: 3\n"
    )
    (root / "plugins/scripts.d.plugin").symlink_to(plugin)
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", 0))
        port = listener.getsockname()[1]
    (root / "netdata.conf").write_text(f"""[global]
 hostname = native-script-validation
 crash reports = off
 update every = 1
[db]
 mode = ram
[directories]
 config = {root}/etc
 stock config = {root}/stock
 stock data = {root}/share
 log = {root}/log
 cache = {root}/cache
 lib = {root}/lib
 web = {root}/web
 plugins = {root}/plugins
[web]
 bind to = 127.0.0.1:{port}
[health]
 enabled = yes
 enable stock health configuration = no
 script to execute on alarm = {true_command}
[plugins]
 enable running new plugins = no
 scripts.d = yes
 statsd = no
 proc = no
 macos = no
 timex = no
 idlejitter = no
 diskspace = no
[cloud]
 enabled = no
[registry]
 enabled = no
""")

    def set_state(state):
        (root / "next.state").write_text(state if state is not None else "omitted")
        (root / "next.state").replace(root / "state")

    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))

    def api(path):
        with opener.open(f"http://127.0.0.1:{port}/api/v1/{path}", timeout=2) as response:
            return json.load(response)

    set_state("critical")
    print(f"artifacts={root}", flush=True)
    command = [str(agent), "-D", "-c", str(root / "netdata.conf"), "-P", str(root / "netdata.pid")]
    print("+ " + shlex.join(command), file=sys.stderr, flush=True)
    with (root / "agent.log").open("w") as log:
        proc = subprocess.Popen(
            command, stdout=log, stderr=log,
            env={**os.environ, "DO_NOT_TRACK": "1", "NETDATA_RUN_DIR": str(root / "run")},
        )
        try:
            (root / "owned-pid").write_text(str(proc.pid))
            initial_launches = None
            for state, wanted in (
                ("critical", "CRITICAL"), ("unknown", "UNDEFINED"),
                ("warning", "WARNING"), ("ok", "CLEAR"), ("critical", "CRITICAL"),
            ):
                set_state(state)
                deadline, seen = time.monotonic() + 45, None
                while time.monotonic() < deadline:
                    if proc.poll() is not None:
                        raise RuntimeError(f"Agent exited: {proc.returncode}")
                    try:
                        alerts = api("alarms?all").get("alarms", {}).values()
                        seen = [a.get("status") for a in alerts if a.get("name") == "native_script_check"]
                        if wanted in seen:
                            break
                    except (OSError, ValueError):
                        pass
                    time.sleep(1)
                else:
                    raise AssertionError(f"{state}: expected {wanted}, last={seen}")
                print(f"{state} -> {wanted}", flush=True)
                if args.mode == "persistent":
                    launches = (root / "launches").read_text().splitlines()
                    # Initial discovery can replace the file-reader job with the
                    # watched configuration before the first health evaluation.
                    if initial_launches is None:
                        initial_launches = launches
                    else:
                        assert launches == initial_launches, launches

            failure_start = int(time.time())
            set_state("collection_failed")
            deadline = time.monotonic() + 45
            while time.monotonic() < deadline:
                if proc.poll() is not None:
                    raise RuntimeError(f"Agent exited: {proc.returncode}")
                checks = [a for a in api("alarms?all").get("alarms", {}).values()
                          if a.get("name") == "native_script_check"]
                states = [a.get("status") for a in checks]
                assert not {"CLEAR", "OK"}.intersection(states), states
                transitions = [e.get("status") for e in api("alarm_log?after=0")
                               if e.get("name") == "native_script_check" and e.get("when", 0) >= failure_start]
                assert not {"CLEAR", "OK"}.intersection(transitions), transitions
                failures = root / "failed-attempts"
                attempts = len(failures.read_text().splitlines()) if failures.exists() else 0
                # Observe a real health evaluation after multiple failed attempts.
                # Retained CRITICAL or stale/undefined health is acceptable.
                if attempts >= 2 and any(a.get("last_updated", 0) > failure_start for a in checks):
                    print(f"collection failed across health evaluation ({attempts} attempts): no false CLEAR", flush=True)
                    break
                time.sleep(1)
            else:
                raise AssertionError(f"no health evaluation during repeated failures: attempts={attempts}, states={states}")

            resumed_at = int(time.time())
            set_state("critical")
            deadline = time.monotonic() + 45
            while time.monotonic() < deadline:
                checks = [a for a in api("alarms?all").get("alarms", {}).values()
                          if a.get("name") == "native_script_check"]
                if any(a.get("status") == "CRITICAL" and a.get("last_updated", 0) > resumed_at for a in checks):
                    print("collection resumed -> CRITICAL", flush=True)
                    break
                time.sleep(1)
            else:
                raise AssertionError("no CRITICAL evaluation after collection resumed")

            charts = [v for v in api("charts")["charts"].values() if v.get("context") == "native_script.check_state"]
            assert len(charts) == 1, charts
            assert charts[0]["chart_labels"]["queue"] == "mail/", charts[0]["chart_labels"]
            print("Label normalization: PASS", flush=True)
            if args.mode == "persistent":
                launches = (root / "launches").read_text().splitlines()
                assert launches == initial_launches, launches
                assert int((root / "exchanges").read_text()) >= 5
                print("Persistent process retained across health transitions: PASS", flush=True)
            removal_start = int(time.time())
            set_state(None)
            deadline = time.monotonic() + 90
            while time.monotonic() < deadline:
                entries = api("alarm_log?after=0")
                states = [
                    e.get("status") for e in entries
                    if e.get("name") == "native_script_check" and e.get("when", 0) >= removal_start
                ]
                if "REMOVED" in states:
                    assert "CLEAR" not in states, states
                    print("critical -> omitted -> REMOVED (no false CLEAR)", flush=True)
                    break
                time.sleep(1)
            else:
                raise AssertionError(f"no REMOVED transition: {states}")
        finally:
            if proc.poll() is None:
                proc.terminate()
                try:
                    proc.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    proc.kill()
                    proc.wait()
            print("Agent stopped", flush=True)


if __name__ == "__main__":
    main()
