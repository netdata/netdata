#!/usr/bin/env python3
"""Exercise installed java.plugin in an owned container; native-host proof is separate."""
import argparse
import concurrent.futures
import json
import hashlib
import shlex
from pathlib import Path
import time
import urllib.parse
import urllib.request
import uuid

from run import Lab, LABEL, wait_http


class GoLab(Lab):
    def api(self, path, body=None):
        req = urllib.request.Request(self.netdata_url + path)
        if body is not None:
            req.data = json.dumps(body).encode()
            req.add_header("Content-Type", "application/json")
        with urllib.request.urlopen(req, timeout=40) as response:
            return json.load(response)

    def config(self, action, body=None):
        # The unclaimed Agent deliberately rejects authenticated DynCfg HTTP calls.
        # As the lab's local administrator, drive its existing plugin input pipe.
        transaction = "lab-" + uuid.uuid4().hex
        function = f'config java:collector:java {action}'
        if action == "test":
            function += " java"
        if body is None:
            frame = f'FUNCTION {transaction} 30 "{function}" 0xffff "user=lab"\n'
        else:
            frame = f'FUNCTION_PAYLOAD {transaction} 30 "{function}" 0xffff "user=lab" application/json\n'
            frame += json.dumps(body) + '\nFUNCTION_PAYLOAD_END\n'
        path = self.output / "control.txt"
        path.write_text(frame)
        self.docker("cp", str(path), self.netdata + ":/state/control.txt")
        pid = self.docker("exec", self.netdata, "sh", "-c",
            'for p in /proc/[0-9]*/exe; do if [ "$(readlink "$p")" = /lab/java.bin ]; then '
            'p=${p%/exe}; printf "%s\\n" "${p##*/}"; fi; done').stdout.strip()
        if not pid.isdecimal():
            raise RuntimeError("expected one owned Go plugin")
        self.docker("exec", self.netdata, "sh", "-c", f"cat /state/control.txt > /proc/{pid}/fd/0")
        deadline = time.monotonic() + 35
        while time.monotonic() < deadline:
            output = self.docker("exec", self.netdata, "cat", "/state/protocol.out").stdout
            marker = "FUNCTION_RESULT_BEGIN " + transaction + " "
            if marker in output:
                response = output.split(marker, 1)[1]
                if "FUNCTION_RESULT_END" in response:
                    header, content = response.split("\n", 1)
                    result = json.loads(content.split("FUNCTION_RESULT_END", 1)[0])
                    if int(header.split()[0]) >= 300:
                        raise RuntimeError(f"DynCfg {action} failed: {result}")
                    return result
            time.sleep(.2)
        raise RuntimeError("DynCfg response deadline exceeded")

    def applications(self):
        return self.api("/api/v3/function?function=java%3Aapplications")

    def rows(self):
        data = self.applications()
        columns = sorted(data["columns"], key=lambda k: data["columns"][k]["index"])
        return [r for row in data["data"] if (r := dict(zip(columns, row)))["Application"] in
                {"checkout", "Checkout API", "inventory", "blocked"}]

    def wait_rows(self, condition, seconds=90):
        deadline = time.monotonic() + seconds
        last = None
        while time.monotonic() < deadline:
            try:
                last = self.rows()
                if condition(last):
                    return last
            except (OSError, KeyError):
                pass
            time.sleep(1)
        raise RuntimeError(f"Java inventory deadline exceeded: {last}")

    def setup(self):
        self.network = self.docker("network", "create", "--label", f"{LABEL}={self.token}",
                                   f"nd-java-{self.token}").stdout.strip()
        (self.output / "netdata.conf").write_text("""[global]
  run as user = netdata
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
[web]
  allow connections from = *
  allow dashboard from = *
  allow management from = *
  bearer token protection = no
""")
        (self.output / "java.conf").write_text("enabled: yes\ndefault_run: no\nmodules:\n  java: yes\n")
        (self.output / "java-job.conf").write_text("jobs:\n  - name: java\n")
        self.netdata = self.create("monitor", ["--network", self.network,
            "--cap-add", "SYS_PTRACE", "-p", "127.0.0.1::19999",
            "-p", "127.0.0.1::18081", "-p", "127.0.0.1::18082", "-p", "127.0.0.1::18083",
            "-e", "DO_NOT_TRACK=1", "-e", "NETDATA_DISABLE_CLOUD=1", "netdata-java-spike:go"])
        for source, dest in (("netdata.conf", "/etc/netdata/netdata.conf"),
                             ("java.conf", "/etc/netdata/java.conf"), ("java-job.conf", "/etc/netdata/java/java.conf")):
            self.docker("cp", str(self.output / source), self.netdata + ":" + dest)
        self.start(self.netdata)
        self.netdata_url = self.base_url(self.netdata, 19999)
        wait_http(self.netdata_url + "/api/v1/info")
        self.apps = {}
        for name in ("checkout", "inventory", "blocked"):
            self.application(name, disabled=name == "blocked")
        self.save("live.json", {"url": self.netdata_url, "run": self.token, "monitor": self.netdata,
                               "environment": "owned container with same-rootfs fixture JVMs"})

    def application(self, name, disabled=False):
        port = {"checkout": 18081, "inventory": 18082, "blocked": 18083}[name]
        args = ["/usr/share/netdata/java/runtime/bin/java", "-Xms128m", "-Xmx256m"]
        if disabled:
            args.append("-XX:+DisableAttachMechanism")
        args += ["-jar", "/lab/app.jar", "--spring.application.name=" + name, "--server.port=" + str(port)]
        # Capture $! immediately. Keep the supervisor alive to reap the owned JVM.
        script = (shlex.join(args) + " > /fixtures/" + name + ".log 2>&1 & "
                  "fixture_pid=$!; printf '%s\\n' \"$fixture_pid\" > /fixtures/" + name + ".pid; wait \"$fixture_pid\"")
        self.docker("exec", "-d", "--user", "10001:10001", self.netdata, "sh", "-c", script)
        wait_http(self.base_url(self.netdata, port) + "/work")
        pid = int(self.docker("exec", self.netdata, "cat", "/fixtures/" + name + ".pid").stdout)
        stat = self.docker("exec", self.netdata, "cat", f"/proc/{pid}/stat").stdout
        start_time = stat.rsplit(") ", 1)[1].split()[19]
        self.apps[name] = {"pid": pid, "start_time": start_time, "port": port}
        self.save("fixture-processes.json", self.apps)

    def signal_application(self, name, signal):
        assert signal in {"STOP", "CONT", "TERM"}
        process = self.apps[name]
        pid = process["pid"]
        stat = self.docker("exec", self.netdata, "cat", f"/proc/{pid}/stat").stdout
        argv = self.docker("exec", self.netdata, "cat", f"/proc/{pid}/cmdline").stdout.split("\0")
        if stat.rsplit(") ", 1)[1].split()[19] != process["start_time"] or "--spring.application.name=" + name not in argv:
            raise RuntimeError("Owned fixture process identity changed; refusing to signal")
        self.docker("exec", "--user", "10001:10001", self.netdata, "kill", "-" + signal, str(pid))

    def workload(self, name, seconds=15):
        result = self.docker("exec", self.netdata, "/usr/share/netdata/java/runtime/bin/java", "-Xms16m", "-Xmx64m", "-cp", "/lab",
                             "HttpLoad", "http://127.0.0.1:" + str(self.apps[name]["port"]), str(seconds), "100", timeout=seconds+80)
        return json.loads(result.stdout)

    def journal(self):
        state = json.loads(self.docker("exec", self.netdata, "cat", "/var/lib/netdata/java/state.json").stdout)
        # Never persist attachment credentials in the regression evidence.
        return {key: {"process": value["process"], "status": value["status"],
                      "credential_fingerprint": hashlib.sha256(value["token"].encode()).hexdigest()}
                for key, value in state["attempts"].items()
                if value["process"]["application"] in {"checkout", "inventory", "blocked"}}

    def capture(self, stage):
        charts = self.api("/api/v1/charts")["charts"]
        charts = {key: chart for key, chart in charts.items() if chart["context"].startswith("java.")}
        samples = {}
        for key in charts:
            samples[key] = self.api("/api/v1/data?" + urllib.parse.urlencode({"chart": key, "after": -45,
                                  "points": 45, "format": "json", "options": "seconds"}))
        result = {"rows": self.rows(), "charts": charts, "samples": samples, "journal": self.journal(), "time": time.time()}
        self.save(stage + ".json", result)
        return result

    def run_checks(self, hold):
        self.wait_rows(lambda rows: len(rows) == 3 and any(r["Status"] == "Blocked" for r in rows))
        with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
            self.save("initial-workloads.json", dict(zip(("checkout", "inventory"), pool.map(self.workload, ("checkout", "inventory")))))
        self.wait_rows(lambda rows: sum(r["Status"] == "Collecting" for r in rows) == 2)
        time.sleep(6)
        initial = self.capture("initial")
        for app in ("checkout", "inventory"):
            contexts = {c["context"] for c in initial["charts"].values() if c["chart_labels"].get("application") == app}
            assert len(contexts) == 6, (app, contexts)
        self.save("schema.json", self.config("schema"))
        self.save("config-tree.json", self.api("/api/v3/config?action=tree&path=/"))
        self.save("config-initial.json", self.config("get"))
        initial_state = self.journal()
        assert len(initial_state) == 3
        self.save("test-response.json", self.config("test", {"name": "java", "update_every": 1}))
        assert self.journal() == initial_state
        self.save("rename-response.json", self.config("update", {"name": "java", "update_every": 1,
                                               "application_names": {"checkout": "Checkout API"}}))
        self.wait_rows(lambda rows: any(r["Application"] == "Checkout API" and r["Status"] == "Collecting" for r in rows))
        time.sleep(3)
        renamed = self.capture("renamed")
        assert set(initial["charts"]) == set(renamed["charts"])
        assert all(c["chart_labels"].get("application_name") == "Checkout API"
                   for c in renamed["charts"].values() if c["chart_labels"].get("application") == "checkout")
        before = {r["Instance"] for r in initial["rows"]}
        assert before == {r["Instance"] for r in renamed["rows"]}
        assert self.journal() == initial_state
        self.save("exclude-response.json", self.config("update", {"name": "java", "update_every": 1,
                                               "exclude_applications": ["inventory"]}))
        self.wait_rows(lambda rows: any(r["Application"] == "inventory" and r["Status"] == "Excluded" for r in rows))
        time.sleep(12)
        excluded = self.capture("excluded")
        assert not any(c["chart_labels"].get("application") == "inventory" for c in excluded["charts"].values())
        self.config("update", {"name": "java", "update_every": 1})
        self.wait_rows(lambda rows: sum(r["Status"] == "Collecting" for r in rows) == 2)
        assert self.journal() == initial_state
        self.save("restart-response.json", self.config("restart", {}))
        self.wait_rows(lambda rows: sum(r["Status"] == "Collecting" for r in rows) == 2)
        assert self.journal() == initial_state
        self.capture("job-restarted")
        # Freeze only the task-owned inventory JVM to stop source exports.
        self.signal_application("inventory", "STOP")
        try:
            self.wait_rows(lambda rows: any(r["Application"] == "inventory" and r["Status"] == "No fresh data" for r in rows))
            time.sleep(12)
            stale = self.capture("stale")
            assert not any(c["chart_labels"].get("application") == "inventory" for c in stale["charts"].values())
        finally:
            self.signal_application("inventory", "CONT")
        self.wait_rows(lambda rows: sum(r["Status"] == "Collecting" for r in rows) == 2)
        # Restart only this owned JVM; identity must change.
        self.signal_application("checkout", "TERM")
        deadline = time.monotonic() + 30
        while self.docker("exec", self.netdata, "test", "-e", f"/proc/{self.apps['checkout']['pid']}", check=False).returncode == 0:
            if time.monotonic() >= deadline:
                raise RuntimeError("Owned checkout JVM did not terminate")
            time.sleep(.2)
        self.application("checkout")
        self.wait_rows(lambda rows: any(r["Application"] == "checkout" and r["Instance"] not in before for r in rows))
        time.sleep(5)
        self.save("restart-workload.json", self.workload("checkout"))
        self.wait_rows(lambda rows: sum(r["Status"] == "Collecting" for r in rows) == 2)
        time.sleep(6)
        self.capture("app-restarted")
        self.save("checks.json", {"passed": True, "evidence_version": 2,
                                   "state_preserved_across_reconfiguration": True})
        deadline = time.monotonic() + hold
        while time.monotonic() < deadline and not (self.output / "continue").exists():
            time.sleep(1)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--hold-seconds", type=int, default=0)
    args = parser.parse_args()
    args.output.mkdir(parents=True, exist_ok=False)
    lab = GoLab(args.output.resolve())
    try:
        lab.setup()
        lab.run_checks(args.hold_seconds)
    finally:
        if hasattr(lab, "netdata"):
            (lab.output / "state").mkdir(exist_ok=True)
            lab.docker("cp", lab.netdata + ":/state/.", str(lab.output / "state"), check=False)
        lab.close()


if __name__ == "__main__":
    main()
