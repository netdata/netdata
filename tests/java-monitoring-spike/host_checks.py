#!/usr/bin/env python3
"""Run only newly owned Java fixtures on an explicitly authorized Linux host."""
import argparse
import json
from pathlib import Path
import re
import socket
import subprocess
import sys
import time

from run import Lab, RUNTIMES, command, metrics_summary, wait_http
from automation_checks import REQUIRED, options, verify_capture
from summarize_automation import fresh_contexts


class HostLab(Lab):
    def __init__(self, base):
        if not re.fullmatch(r"/var/tmp/nd-java-spike-[a-f0-9]{12}", str(base)):
            raise ValueError("exact task directory required")
        self.base = base
        token = base.name.rsplit("-", 1)[-1]
        super().__init__(base / "evidence", f"nd-java-spike-{token}:runtime")
        self.token = token
        self.slice = f"ndjavaspike{token}.slice"
        self.bundle = base / "bundle"
        self.java = str(self.bundle / "jdk/bin/java")
        self.units = {}
        self.mon_image = f"nd-java-spike-{token}:monitor"
        self.app_image = f"nd-java-spike-{token}:plain"

    def save_units(self):
        self.save("units.json", self.units)

    def create(self, name, args):
        return super().create(name, ["--cpus", "2", "--memory", "768m", *args])

    def system(self, *args, **kwargs):
        return command(["sudo", "-n", *args], **kwargs)

    def unit(self, role, payload, uid=None, monitor=False, caps=None, wait=False, outside=False, private_tmp=False):
        name = f"nd-java-{self.token}-{role}.service"
        description = f"Netdata Java spike {self.token} {role}"
        state = self.system("systemctl", "show", name, "--property=LoadState", "--value").stdout.strip()
        if state != "not-found":
            raise RuntimeError("refusing pre-existing unit " + name)
        self.units[name] = description
        self.save_units()
        args = ["systemd-run", "--quiet", "--unit=" + name, "--description=" + description,
                "--property=RuntimeMaxSec=900", "--property=MemoryMax=768M", "--property=CPUQuota=200%",
                "--property=TasksMax=256", "--property=NoNewPrivileges=yes"]
        if not outside:
            args += ["--slice=" + self.slice]
        if uid is not None:
            # systemd resolves User= through NSS; setpriv permits unused numeric fixture IDs.
            payload = ["/usr/bin/setpriv", f"--reuid={uid}", f"--regid={uid}", "--clear-groups",
                       "--no-new-privs", *payload]
        if private_tmp:
            args += ["--property=PrivateTmp=yes", "--property=BindReadOnlyPaths=" + str(self.bundle)]
        if caps is not None:
            args += ["--property=CapabilityBoundingSet=" + caps]
        if monitor:
            args += ["--setenv=SCOUT_LAB_SCOPE=owned-host-cgroup", "--setenv=SCOUT_RUN=" + self.token,
                     "--setenv=SCOUT_OPTIONS=" + self.scout_options]
        if wait:
            args += ["--wait", "--pipe", "--collect"]
        else:
            args += ["--property=StandardOutput=append:" + str(self.output / (role + ".stdout")),
                     "--property=StandardError=append:" + str(self.output / (role + ".stderr"))]
            for suffix in ("stdout", "stderr"):
                (self.output / (role + "." + suffix)).touch(mode=0o644)
        result = self.system(*args, *payload, timeout=100 if wait else 30)
        return name, result

    def verify_unit(self, name):
        description = self.system("systemctl", "show", name, "--property=Description", "--value").stdout.strip()
        if description != self.units[name]:
            raise RuntimeError("unit ownership mismatch: " + name)

    def build(self):
        self.docker("build", "--build-arg", "RUNTIME_IMAGE=" + RUNTIMES["21"],
                    "-t", self.fixture_image, ".", timeout=1200)
        for stage, image in (("application", self.app_image), ("monitor", self.mon_image)):
            self.docker("build", "-f", "Dockerfile.automation", "--build-arg", "ARTIFACT_IMAGE=" + self.fixture_image,
                        "--target", stage, "-t", image, ".", timeout=900)
        staging = self.create("staging", [self.mon_image])
        self.bundle.mkdir()
        self.docker("cp", staging + ":/lab/.", str(self.bundle))
        self.docker("cp", staging + ":/opt/java/openjdk", str(self.bundle / "jdk"))
        # Payloads executed by the root monitor are immutable to the fixture UIDs and ordinary users.
        self.system("chown", "-R", "root:root", str(self.bundle))
        self.system("chmod", "-R", "go-w", str(self.bundle))
        self.system("chown", "root:root", str(self.base))
        self.system("chmod", "0755", str(self.base))
        self.system("install", "-d", "-m", "0700", str(self.base / "state"))
        command([self.java, "-cp", str(self.bundle), "Scout", "--self-test"])
        self.save("java-runtime.json", {"version": command([self.java, "-version"]).stderr,
                                        "release": (self.bundle / "jdk/release").read_text()})

    def app(self, role, uid):
        cid = self.create(role, ["--network", self.network, "--cgroup-parent", self.slice,
             "--user", f"{uid}:{uid}", "-p", "127.0.0.1::8080", "--entrypoint", "java", self.app_image,
             "-Xms128m", "-Xmx256m", "-jar", "/app/app.jar"])
        self.start(cid)
        url = self.base_url(cid, 8080)
        wait_http(url + "/work")
        return cid, url

    def host_app(self, role, uid, outside=False):
        with socket.socket() as listener:
            listener.bind(("127.0.0.1", 0))
            port = listener.getsockname()[1]
        name, _ = self.unit(role, [self.java, "-Xms128m", "-Xmx256m", "-jar", str(self.bundle / "app.jar"),
                "--server.address=127.0.0.1", f"--server.port={port}"], uid=uid, outside=outside, private_tmp=True)
        url = f"http://127.0.0.1:{port}"
        wait_http(url + "/work")
        return name, url

    def events(self):
        return [line.split("\t") for line in (self.output / "scout.stdout").read_text().splitlines()
                if line.startswith("SCOUT\t")]

    def await_successes(self, count):
        until = time.monotonic() + 100
        while time.monotonic() < until:
            events = self.events()
            if any(e[2] in ("failed", "timeout") for e in events):
                raise RuntimeError("owned target attach failed; see scout logs")
            if len([e for e in events if e[2] == "acknowledged"]) == count:
                return events
            time.sleep(2)
        raise RuntimeError("native discovery deadline exceeded")

    def workload(self, role, url, seconds=25):
        _, result = self.unit(role, [self.java, "-Xms16m", "-Xmx96m", "-cp", str(self.bundle),
                "HttpLoad", url, str(seconds), "100"], uid=61005, wait=True)
        return json.loads(result.stdout.strip().splitlines()[-1])

    def process_evidence(self, event):
        pid, start = event[1].split(":")
        proc = Path("/proc") / pid
        group = (proc / "cgroup").read_text().strip()
        if not group.startswith("0::/" + self.slice + "/"):
            raise RuntimeError("candidate left owned cgroup")
        stat = (proc / "stat").read_text().rsplit(") ", 1)[1].split()
        if stat[19] != start:
            raise RuntimeError("candidate identity changed")
        status = (proc / "status").read_text()
        return {"key": event[1], "uid": event[3], "cgroup": group,
                "nspid": re.search(r"^NSpid:\s+(.+)$", status, re.M).group(1).split(),
                "pid_namespace": self.system("readlink", str(proc / "ns/pid")).stdout.strip()}

    def capture_checked(self, uid, checkpoint, workload, since=0):
        name = "java-app-" + str(uid)
        time.sleep(3)
        coverage = self.capture(name, checkpoint)
        # Initial post-attach traffic must match exactly; restart readiness can add one 200 request.
        verify_capture(self.output, name, workload["statuses"] if checkpoint == "initial" else None, checkpoint)
        event = [e for e in self.events() if e[2] == "acknowledged" and e[3] == str(uid)][-1]
        instance = self.token + "-" + event[1]
        wanted = {"otel." + metric for metric in REQUIRED - {"http.server.request.duration"}}
        wanted |= {"otel.http.server.request.duration.count", "otel.http.server.request.duration.bucket"}
        fresh = fresh_contexts(self.output, name + "-" + checkpoint, instance, since)
        if not wanted <= fresh:
            raise RuntimeError("missing current-process stored contexts: " + str(wanted - fresh))
        return {"workload": workload, "coverage": coverage, "instance": instance, "since": since,
                "fresh_contexts": sorted(fresh), "process": self.process_evidence(event)}

    def existing(self, ids=None):
        ids = ids if ids is not None else self.docker("ps", "-q").stdout.split()
        return {cid: self.docker("inspect", "--format",
                    "{{.State.Running}} {{.State.Pid}} {{.State.StartedAt}} {{.RestartCount}}", cid).stdout.strip()
                for cid in ids}

    def close(self):
        errors = []
        for name in reversed(self.units):
            try:
                state = self.system("systemctl", "show", name, "--property=LoadState", "--value").stdout.strip()
                if state == "not-found":
                    continue
                self.verify_unit(name)
                self.system("systemctl", "stop", name)
                self.system("systemctl", "reset-failed", name, check=False)
            except Exception as error:
                errors.append(str(error))
        super().close()
        group = Path("/sys/fs/cgroup") / self.slice
        remaining = [str(path.relative_to(group)) for path in group.rglob("cgroup.procs") if path.read_text().strip()]
        if remaining:
            errors.append("owned slice still contains processes: " + str(remaining))
        else:
            self.system("systemctl", "stop", self.slice, check=False)
        self.save("cleanup.json", {"errors": errors,
            "remaining_owned_containers": self.docker("ps", "-aq", "--filter",
                 "label=org.netdata.java-spike.run=" + self.token).stdout.split(),
            "slice_membership": self.system("systemctl", "show", self.slice, "--property=ActiveState", "--value").stdout.strip()})
        if errors:
            raise RuntimeError("cleanup requires attention: " + str(errors))


def experiment(lab):
    lab.setup()
    collector_ip = lab.docker("inspect", "--format",
        "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", lab.collector).stdout.strip()
    lab.scout_options = options(lab, "discovered", narrow=True).replace("http://telemetry:4318", "http://" + collector_ip + ":4318")
    host, host_url = lab.host_app("host", 61001)
    first, first_url = lab.app("container-a", 61002)
    _, second_url = lab.app("container-b", 61003)
    # Same fixture jar outside the admitted slice proves that a name match cannot authorize attachment.
    decoy, decoy_url = lab.host_app("excluded-control", 61004, outside=True)
    result = {"host_pid_namespace": Path("/proc/self/ns/pid").readlink().as_posix(),
              "ptrace_scope": Path("/proc/sys/kernel/yama/ptrace_scope").read_text().strip()}
    _, denied = lab.unit("permission-control", ["/usr/bin/env", "SCOUT_SCAN_ONCE=true", lab.java,
        "-Xmx64m", "-cp", str(lab.bundle), "Scout"], monitor=True, caps="", wait=True)
    result["permission_control"] = {"stdout": denied.stdout, "stderr": denied.stderr}
    assert "SCAN_ACCESS" in denied.stdout and "\tdiscovered\t" not in denied.stdout
    monitor, _ = lab.unit("scout", [lab.java, "-Xms16m", "-Xmx64m", "-cp", str(lab.bundle), "Scout"],
                          monitor=True, caps="CAP_SYS_PTRACE CAP_SETUID CAP_SETGID")
    lab.await_successes(3)
    result["initial"] = {}
    for uid, url in ((61001, host_url), (61002, first_url), (61003, second_url)):
        result["initial"][str(uid)] = lab.capture_checked(uid, "initial", lab.workload("load-" + str(uid), url))
        lab.save("host-results.json", result)
    assert result["initial"]["61001"]["process"]["pid_namespace"] == result["host_pid_namespace"]
    assert all(len(result["initial"][uid]["process"]["nspid"]) == 2 for uid in ("61002", "61003"))
    assert len({result["initial"][uid]["process"]["pid_namespace"] for uid in result["initial"]}) == 3
    lab.verify_unit(monitor)
    lab.system("systemctl", "restart", monitor)
    time.sleep(5)
    assert len([e for e in lab.events() if e[2] == "attempt"]) == 3
    result["monitor_restart_attempts"] = 3
    restarted_at = time.time()
    lab.verify_unit(host)
    lab.system("systemctl", "restart", host)
    lab.await_successes(4)
    wait_http(host_url + "/work")
    result["host_restart"] = lab.capture_checked(61001, "restarted", lab.workload("load-host-restarted", host_url), restarted_at)
    assert result["host_restart"]["instance"] != result["initial"]["61001"]["instance"]
    lab.save("host-results.json", result)
    # Keep the old namespace alive until comparison: namespace inode numbers can be reused after exit.
    label = lab.docker("inspect", "--format", '{{index .Config.Labels "org.netdata.java-spike.run"}}', first).stdout.strip()
    assert label == lab.token
    replaced_at = time.time()
    replacement, replacement_url = lab.app("container-a-replacement", 61002)
    lab.await_successes(5)
    replacement_event = [e for e in lab.events() if e[2] == "acknowledged" and e[3] == "61002"][-1]
    replacement_process = lab.process_evidence(replacement_event)
    assert replacement_process["pid_namespace"] != result["initial"]["61002"]["process"]["pid_namespace"]
    lab.docker("stop", "--time", "10", first)
    result["container_replacement"] = lab.capture_checked(61002, "replaced",
        lab.workload("load-container-replaced", replacement_url), replaced_at)
    assert result["container_replacement"]["instance"] != result["initial"]["61002"]["instance"]
    assert result["container_replacement"]["process"]["pid_namespace"] != result["initial"]["61002"]["process"]["pid_namespace"]
    result["replacement_container_changed"] = replacement != first
    result["replacement_mode"] = "rolling; namespace comparison while both processes alive"
    result["events"] = lab.events()
    assert {e[3] for e in result["events"] if e[2] == "attempt"} == {"61001", "61002", "61003"}
    result["excluded_control"] = {"unit_owned": decoy in lab.units,
        "workload": lab.workload("load-excluded", decoy_url, 5), "attach_attempts": 0}
    result["excluded_control"]["coverage"] = lab.capture("java-app-61004", "excluded")
    assert not result["excluded_control"]["coverage"]["metrics"]
    result["monitor_properties"] = lab.system("systemctl", "show", monitor, "--property=CapabilityBoundingSet",
        "--property=NoNewPrivileges", "--property=ControlGroup", "--property=MemoryCurrent").stdout
    lab.save("host-results.json", result)


def summarize_host(directory):
    result = json.loads((directory / "host-results.json").read_text())
    ownership = json.loads((directory / "ownership.json").read_text())
    cleanup = json.loads((directory / "cleanup.json").read_text())
    assert not cleanup["errors"] and not cleanup["remaining_owned_containers"]
    assert cleanup["slice_membership"] == "inactive"
    before = json.loads((directory / "existing-before.json").read_text())
    assert before == json.loads((directory / "existing-after.json").read_text())
    assert set(result["initial"]) == {"61001", "61002", "61003"}
    events = result["events"]
    attempts = [e for e in events if e[2] == "attempt"]
    successes = [e for e in events if e[2] == "acknowledged"]
    assert len(attempts) == len({e[1] for e in attempts}) == len(successes) == 5
    assert sorted(e[3] for e in successes) == ["61001", "61001", "61002", "61002", "61003"]
    assert result["monitor_restart_attempts"] == 3
    properties = dict(line.split("=", 1) for line in result["monitor_properties"].splitlines())
    assert set(properties["CapabilityBoundingSet"].split()) == {"cap_sys_ptrace", "cap_setuid", "cap_setgid"}
    assert properties["NoNewPrivileges"] == "yes"
    assert "SCAN_ACCESS" in result["permission_control"]["stdout"]
    assert "\tdiscovered\t" not in result["permission_control"]["stdout"]
    wanted = {"otel." + metric for metric in REQUIRED - {"http.server.request.duration"}}
    wanted |= {"otel.http.server.request.duration.count", "otel.http.server.request.duration.bucket"}
    records = [(uid, "initial", record) for uid, record in result["initial"].items()]
    records += [("61001", "restarted", result["host_restart"]), ("61002", "replaced", result["container_replacement"])]
    for uid, checkpoint, record in records:
        assert record["workload"]["requests"] == 2500 and record["workload"]["errors"] == 0
        assert record["workload"]["statuses"] == {"200": 2250, "503": 250}
        assert record["instance"] == ownership["run"] + "-" + record["process"]["key"]
        assert record["process"]["uid"] == uid
        assert record["process"]["cgroup"].startswith("0::/ndjavaspike" + ownership["run"] + ".slice/")
        checked = verify_capture(directory, "java-app-" + uid,
                                 record["workload"]["statuses"] if checkpoint == "initial" else None, checkpoint)
        if checkpoint != "initial":
            assert checked["counts"] in ({"200": 2250, "503": 250}, {"200": 2251, "503": 250})
        actual = fresh_contexts(directory, "java-app-" + uid + "-" + checkpoint, record["instance"], record["since"])
        assert wanted <= actual
    native_ns = result["host_pid_namespace"]
    assert result["initial"]["61001"]["process"]["pid_namespace"] == native_ns
    assert len({r["process"]["pid_namespace"] for r in result["initial"].values()}) == 3
    for uid in ("61002", "61003"):
        assert result["initial"][uid]["process"]["nspid"][-1] == "1"
        assert len(result["initial"][uid]["process"]["nspid"]) == 2
    assert result["host_restart"]["instance"] != result["initial"]["61001"]["instance"]
    assert result["container_replacement"]["instance"] != result["initial"]["61002"]["instance"]
    assert result["replacement_container_changed"]
    assert result["container_replacement"]["process"]["pid_namespace"] != result["initial"]["61002"]["process"]["pid_namespace"]
    assert not metrics_summary(directory / "recording/metrics.json", "java-app-61004")["metrics"]
    assert result["excluded_control"]["workload"]["errors"] == 0
    return {"initial_targets": 3, "successful_attachments": 5, "independent_initial_pid_namespaces": 3,
            "host_restart_verified": True, "container_replacement_verified": True,
            "duplicate_attempts_after_monitor_restart": 0, "excluded_matching_app_untouched": True,
            "no_capability_discovery_denied": True, "ptrace_scope": result["ptrace_scope"],
            "monitor_properties": result["monitor_properties"].splitlines(),
            "stored_contexts_verified_per_phase": sorted(wanted), "verified_phases": len(records),
            "preexisting_containers_unchanged": len(before), "owned_runtime_resources_cleaned": True}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("base", type=Path)
    parser.add_argument("--build", action="store_true")
    parser.add_argument("--verify", action="store_true", help="replay an evidence directory without host operations")
    args = parser.parse_args()
    if args.verify:
        print(json.dumps(summarize_host(args.base), indent=2))
        return
    lab = HostLab(args.base)
    lab.output.mkdir(exist_ok=True)
    if any(lab.output.iterdir()):
        raise RuntimeError("evidence directory must be empty")
    before = lab.existing()
    lab.save("existing-before.json", before)
    try:
        if not Path("/sys/fs/cgroup/cgroup.controllers").exists():
            raise RuntimeError("cgroup v2 required")
        if lab.docker("info", "--format", "{{.CgroupDriver}}").stdout.strip() != "systemd":
            raise RuntimeError("this bounded experiment requires Docker's systemd cgroup driver")
        for uid in range(61001, 61006):
            if command(["getent", "passwd", str(uid)], check=False).returncode == 0:
                raise RuntimeError("fixture numeric UID is already assigned")
        if args.build:
            lab.build()
        else:
            lab.save("java-runtime.json", {"version": command([lab.java, "-version"]).stderr,
                                            "release": (lab.bundle / "jdk/release").read_text()})
        experiment(lab)
    finally:
        lab.close()
        after = lab.existing(before.keys())
        lab.save("existing-after.json", after)
        if before != after:
            raise RuntimeError("pre-existing container state changed during experiment; investigate")


if __name__ == "__main__":
    try:
        main()
    except subprocess.CalledProcessError as error:
        print(error.stderr, file=sys.stderr)
        sys.exit(error.returncode)
