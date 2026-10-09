#!/usr/bin/env python3
"""Exercise the experimental Go plugin in an owned, isolated PID namespace."""
import argparse
import concurrent.futures
import json
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
        function = f'config javaspike:collector:java {action}'
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
            'for p in /proc/[0-9]*/exe; do if [ "$(readlink "$p")" = /lab/javaspike.bin ]; then '
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
        return [dict(zip(columns, row)) for row in data["data"]]

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
        self.anchor = self.create("anchor", ["--network", self.network, "--entrypoint", "sleep",
                                            "netdata-java-spike:plain-jre", "infinity"])
        self.start(self.anchor)
        self.apps = {}
        for name in ("checkout", "inventory", "blocked"):
            self.apps[name] = self.application(name, disabled=name == "blocked")
        (self.output / "java-spike-run").write_text(self.token + "\n")
        (self.output / "netdata.conf").write_text("""[global]
  run as user = root
[db]
  mode = ram
[plugins]
  enable running new plugins = no
  javaspike = yes
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
        (self.output / "javaspike.conf").write_text("enabled: yes\ndefault_run: no\nmodules:\n  java: yes\n")
        (self.output / "java.conf").write_text("jobs:\n  - name: java\n")
        self.netdata = self.create("monitor", ["--network", self.network, "--network-alias", "java-monitor",
            "--pid", "container:" + self.anchor, "--cap-add", "SYS_PTRACE", "-p", "127.0.0.1::19999",
            "-e", "DO_NOT_TRACK=1", "-e", "NETDATA_DISABLE_CLOUD=1",
            "-e", "SCOUT_LAB_SCOPE=owned-fixture-pid-namespace", "-e", "JAVASPIKE_RUN=" + self.token,
            "-e", "JAVASPIKE_ENDPOINT=http://java-monitor:4318", "netdata-java-spike:go"])
        for source, dest in (("java-spike-run", "/lab/java-spike-run"), ("netdata.conf", "/etc/netdata/netdata.conf"),
                             ("javaspike.conf", "/etc/netdata/javaspike.conf"), ("java.conf", "/etc/netdata/javaspike/java.conf")):
            self.docker("cp", str(self.output / source), self.netdata + ":" + dest)
        self.start(self.netdata)
        self.netdata_url = self.base_url(self.netdata, 19999)
        wait_http(self.netdata_url + "/api/v1/info")
        self.save("live.json", {"url": self.netdata_url, "run": self.token, "monitor": self.netdata})

    def application(self, name, disabled=False):
        args = ["--network", self.network, "--network-alias", name, "--pid", "container:" + self.anchor,
                "-p", "127.0.0.1::8080", "--entrypoint", "java", "netdata-java-spike:plain-jre", "-Xms128m", "-Xmx256m"]
        if disabled:
            args.append("-XX:+DisableAttachMechanism")
        cid = self.create(name, [*args, "-jar", "/app/app.jar", "--spring.application.name=" + name])
        self.start(cid)
        wait_http(self.base_url(cid, 8080) + "/work")
        return cid

    def workload(self, name, seconds=15):
        result = self.docker("exec", self.netdata, "/lab/jdk/bin/java", "-Xms16m", "-Xmx64m", "-cp", "/lab",
                             "HttpLoad", "http://" + name + ":8080", str(seconds), "100", timeout=seconds+80)
        return json.loads(result.stdout)

    def journal(self):
        return [json.loads(line) for line in self.docker("exec", self.netdata, "cat", "/state/attempts.jsonl").stdout.splitlines()]

    def capture(self, stage):
        charts = self.api("/api/v1/charts")["charts"]
        charts = {key: chart for key, chart in charts.items() if chart["context"].startswith("java_spike.")}
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
        initial_attempts = len([a for a in self.journal() if a["Status"] == "Unknown"])
        self.save("test-response.json", self.config("test", {"name": "java", "update_every": 1}))
        assert len([a for a in self.journal() if a["Status"] == "Unknown"]) == initial_attempts
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
        assert len([a for a in self.journal() if a["Status"] == "Unknown"]) == initial_attempts
        self.save("exclude-response.json", self.config("update", {"name": "java", "update_every": 1,
                                               "exclude_applications": ["inventory"]}))
        self.wait_rows(lambda rows: any(r["Application"] == "inventory" and r["Status"] == "Excluded" for r in rows))
        time.sleep(12)
        excluded = self.capture("excluded")
        assert not any(c["chart_labels"].get("application") == "inventory" for c in excluded["charts"].values())
        self.config("update", {"name": "java", "update_every": 1})
        self.wait_rows(lambda rows: sum(r["Status"] == "Collecting" for r in rows) == 2)
        assert len([a for a in self.journal() if a["Status"] == "Unknown"]) == initial_attempts
        self.save("restart-response.json", self.config("restart", {}))
        self.wait_rows(lambda rows: sum(r["Status"] == "Collecting" for r in rows) == 2)
        assert len([a for a in self.journal() if a["Status"] == "Unknown"]) == initial_attempts
        self.capture("job-restarted")
        # Freeze only the task-owned inventory container to stop source exports.
        self.docker("pause", self.apps["inventory"])
        try:
            self.wait_rows(lambda rows: any(r["Application"] == "inventory" and r["Status"] == "No fresh data" for r in rows))
            time.sleep(12)
            stale = self.capture("stale")
            assert not any(c["chart_labels"].get("application") == "inventory" for c in stale["charts"].values())
        finally:
            self.docker("unpause", self.apps["inventory"])
        self.wait_rows(lambda rows: sum(r["Status"] == "Collecting" for r in rows) == 2)
        # Restart only this owned application's container; identity must change.
        self.docker("restart", "--time", "10", self.apps["checkout"])
        wait_http(self.base_url(self.apps["checkout"], 8080) + "/work")
        self.wait_rows(lambda rows: any(r["Application"] == "checkout" and r["Instance"] not in before for r in rows))
        time.sleep(5)
        self.save("restart-workload.json", self.workload("checkout"))
        self.wait_rows(lambda rows: sum(r["Status"] == "Collecting" for r in rows) == 2)
        time.sleep(6)
        self.capture("app-restarted")
        self.save("checks.json", {"passed": True, "initial_attempts": initial_attempts,
                                   "final_attempts": len([a for a in self.journal() if a["Status"] == "Unknown"])})
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
