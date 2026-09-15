#!/usr/bin/env python3
"""Compile and exercise the real nd-run helper with synthetic environments only."""

import argparse
from contextlib import contextmanager
import errno
import json
import os
from pathlib import Path
import pwd
import select
import shlex
import shutil
import signal
import subprocess
import struct
import sys
import tempfile
import unittest
from unittest.mock import patch


ROOT = Path(__file__).resolve().parents[2]
OPTIONS = None
BUILD = None
HELPER = None
FALLBACK = None
PROBE = None


def compile_binary(source, output, config):
    command = [OPTIONS.cc, "-std=gnu11", "-Wall", "-Wextra", "-Werror", "-g",
               *shlex.split(OPTIONS.cflags), "-I", str(config),
               "-I", str(ROOT / "src/collectors/utils"), str(source), "-o", str(output)]
    if source.name == "nd-run.c":
        command.append(str(ROOT / "src/collectors/utils/nd-file-reader.c"))
    if OPTIONS.capabilities:
        command.append("-lcap")
    print(shlex.join(command), file=sys.stderr)
    result = subprocess.run(command, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    if result.returncode:
        # Compiler output contains source diagnostics, never the test environment.
        sys.stderr.buffer.write(result.stderr)
        raise RuntimeError(f"compile failed in {Path.cwd()} with status {result.returncode}")


def configure(directory, user):
    directory.mkdir()
    definitions = ["#define _GNU_SOURCE 1", f"#define NETDATA_USER {json.dumps(user)}"]
    if sys.platform.startswith(("linux", "freebsd", "openbsd")) and not OPTIONS.no_setres:
        definitions += ["#define HAVE_SETRESUID 1", "#define HAVE_SETRESGID 1"]
    if OPTIONS.capabilities:
        definitions += ["#define HAVE_CAPABILITY 1"]
    (directory / "config.h").write_text("\n".join(definitions) + "\n")
    return directory


class NdRunTests(unittest.TestCase):
    def invoke(self, arguments, *, env=None, launcher=None, helper=None):
        command = [str(helper or HELPER), *map(str, arguments)]
        if launcher:
            command = [str(PROBE), launcher, *command]
        return subprocess.run(command, env={} if env is None else env, cwd=BUILD,
                              stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=10)

    def success(self, result):
        self.assertEqual(result.returncode, 0, "child failed (captured output withheld)")
        self.assertFalse(result.stderr, "unexpected diagnostic (captured output withheld)")

    def environment(self, result):
        self.success(result)
        self.assertTrue(result.stdout.endswith(b"\0"))
        entries = result.stdout[:-1].split(b"\0")
        names = [entry.partition(b"=")[0] for entry in entries]
        self.assertEqual(len(names), len(set(names)), "duplicate environment keys")
        return dict(entry.split(b"=", 1) for entry in entries)

    def expected_controlled(self, user=None):
        account = pwd.getpwnam(user or OPTIONS.user)
        return {b"USER": os.fsencode(account.pw_name), b"LOGNAME": os.fsencode(account.pw_name),
                b"HOME": os.fsencode(account.pw_dir), b"SHELL": b"/bin/sh", b"LC_ALL": b"C"}

    def check_environment(self, actual, expected):
        # Even assertion failures must not print value-bearing maps.
        self.assertTrue(actual == expected, "child environment differs (values withheld)")

    def test_environment_modes(self):
        custom = {b"AWS_SESSION_TOKEN": b"synthetic", b"CUSTOM_AUTH": b"a=b=c",
                  b"EMPTY": b"", b"SPACES": b" a b \t ", b"LINES": b"one\ntwo\n",
                  b"SHELL_TEXT": b"$(touch nd-run-must-not-exist); `false` '$HOME' \\ *",
                  b"https_proxy": b"http://proxy.invalid", b"TOOL_CONFIG": b"/synthetic/config",
                  b"USER_SUFFIX": b"keep", b"HOM": b"keep", b"HOMELESS": b"keep",
                  b"RAW_BYTES": b"\xff\xfe"}
        custom.update({f"CUSTOM_{i}".encode(): f"value={i}".encode() for i in range(128)})
        inherited = {b"PATH": b"/usr/bin:/bin", b"PWD": b"/synthetic/pwd",
                     b"TZ": b"UTC0", b"TZDIR": b"/synthetic/tz"}
        collisions = {key: b"inherited" for key in self.expected_controlled()}
        for preserve in (False, True):
            for tmpdir in (None, b"", b"/synthetic/tmp"):
                with self.subTest(preserve=preserve, tmpdir_set=tmpdir is not None):
                    env = {**custom, **inherited, **collisions}
                    if tmpdir is not None:
                        env[b"TMPDIR"] = tmpdir
                    prefix = ["--preserve-env", "--"] if preserve else []
                    actual = self.environment(self.invoke([*prefix, PROBE, "env"], env=env))
                    expected = {**inherited, **self.expected_controlled(),
                                b"TMPDIR": b"/tmp" if tmpdir is None else tmpdir}
                    if preserve:
                        expected.update(custom)
                    self.check_environment(actual, expected)
        self.assertFalse((BUILD / "nd-run-must-not-exist").exists())

    def test_empty_environment_and_fallback_account(self):
        for helper, user in ((HELPER, OPTIONS.user), (FALLBACK, "nobody")):
            for prefix in ([], ["--preserve-env", "--"]):
                with self.subTest(fallback=helper == FALLBACK, preserve=bool(prefix)):
                    actual = self.environment(self.invoke([*prefix, PROBE, "env"], helper=helper))
                    self.check_environment(actual, {**self.expected_controlled(user), b"TMPDIR": b"/tmp"})

    def test_duplicate_controlled_fields(self):
        for prefix in ([], ["--preserve-env", "--"]):
            result = self.invoke([*prefix, PROBE, "env"], launcher="launch-duplicates")
            actual = self.environment(result)
            expected = {**self.expected_controlled(), b"TMPDIR": b""}
            if prefix:
                expected.update({b"ND_AUTH": b"synthetic", b"USER_SUFFIX": b"keep",
                                 b"HOM": b"keep", b"HOMELESS": b"keep"})
            self.check_environment(actual, expected)

    def test_arguments_and_path_lookup(self):
        arguments = ["", "a b", "one\ntwo", "a=b", "$(false);*", "--preserve-env", "--"]
        expected = b"".join(os.fsencode(arg) + b"\0" for arg in arguments)
        for prefix in ([], ["--preserve-env", "--"]):
            for target in (str(PROBE), PROBE.name):
                result = self.invoke([*prefix, target, "argv", *arguments], env={"PATH": str(BUILD)})
                self.success(result)
                self.assertTrue(result.stdout == expected, "argument boundaries changed")
        # Outside the named helper modes, leading dashes remain command names.
        for name in ("--", "--unknown"):
            (BUILD / name).symlink_to(PROBE)
            self.success(self.invoke([name, "argv"], env={"PATH": str(BUILD)}))

    def test_missing_command_and_malformed_option(self):
        for arguments in ([], ["--preserve-env"], ["--preserve-env", "--"],
                          ["--preserve-env", PROBE, "exit", "0"],
                          ["--preserve-env", "synthetic-auth", PROBE]):
            result = self.invoke(arguments, env={"ND_AUTH": "synthetic-auth"})
            self.assertEqual(result.returncode, 1)
            self.assertNotIn(b"synthetic-auth", result.stdout + result.stderr)
        result = self.invoke([])
        self.assertIn(b"--preserve-env -- command", result.stdout)

    def test_exit_status_and_exec_errors(self):
        denied = BUILD / "nonexecutable"
        denied.write_text("synthetic")
        denied.chmod(0o644)
        for prefix in ([], ["--preserve-env", "--"]):
            for status in (0, 1, 42, 126, 127):
                self.assertEqual(self.invoke([*prefix, PROBE, "exit", str(status)]).returncode, status)
            for target, status in ((BUILD / "missing", 127), (denied, 126), ("", 127)):
                result = self.invoke([*prefix, target], env={"ND_AUTH": "synthetic-auth"})
                self.assertEqual(result.returncode, status)
                self.assertFalse(result.stdout)
                self.assertNotIn(b"synthetic-auth", result.stderr)

    def test_signals_and_exec_replacement(self):
        for prefix in ([], ["--preserve-env", "--"]):
            result = self.invoke([*prefix, PROBE, "signal"], launcher="launch-signals")
            self.assertEqual(result.returncode, -signal.SIGPIPE)
            with subprocess.Popen([str(HELPER), *prefix, str(PROBE), "pid"], env={},
                                  stdout=subprocess.PIPE, stderr=subprocess.PIPE) as child:
                stdout, stderr = child.communicate(timeout=10)
                self.assertEqual(child.returncode, 0)
                self.assertFalse(stderr)
                self.assertEqual(int(stdout), child.pid)

    def check_identity(self, result, user, switched, inherited_groups=None):
        self.success(result)
        lines = result.stdout.splitlines()
        account = pwd.getpwnam(user)
        ids = [int(item) for item in lines[0].split()]
        expected = ([account.pw_uid] * 2 + [account.pw_gid] * 2 if switched else
                    [os.getuid(), os.geteuid(), os.getgid(), os.getegid()])
        self.assertEqual(ids, expected)
        groups = {int(item) for item in lines[1].split()}
        expected_groups = (set(os.getgrouplist(account.pw_name, account.pw_gid)) if switched
                           else inherited_groups)
        self.assertEqual(groups, expected_groups)
        self.assertEqual(lines[2], b"-1 -1", "child regained root IDs")
        if OPTIONS.capabilities:
            self.assertIn(b"caps_empty=1", lines)
            self.assertIn(b"ambient=0", lines)

    def test_identity_groups_and_privilege_drop(self):
        baseline = self.invoke(["identity"], helper=PROBE)
        self.success(baseline)
        inherited_groups = {int(item) for item in baseline.stdout.splitlines()[1].split()}
        # macOS Python can report account memberships instead of inherited process groups.
        # Guard against reintroducing os.getgroups() as the expected process groups.
        # The correct oracle does not call the patched function.
        with patch("os.getgroups", return_value=[0x7fffffff]):
            for helper, user in ((HELPER, OPTIONS.user), (FALLBACK, "nobody")):
                for prefix in ([], ["--preserve-env", "--"]):
                    result = self.invoke([*prefix, PROBE, "identity"], helper=helper)
                    self.check_identity(result, user, os.geteuid() == 0, inherited_groups)

    @unittest.skipUnless(sys.platform.startswith("linux"), "Linux capabilities")
    def test_inherited_capabilities(self):
        if not OPTIONS.capabilities or os.geteuid() != 0:
            self.skipTest("requires root and --capabilities in a controlled environment")
        # First prove the launcher actually conveys the capability across exec.
        baseline = subprocess.run([str(PROBE), "launch-caps", str(PROBE), "identity"], env={},
                                  stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=10)
        self.success(baseline)
        self.assertIn(b"caps_empty=0", baseline.stdout.splitlines())
        self.assertIn(b"ambient=1", baseline.stdout.splitlines())
        for prefix in ([], ["--preserve-env", "--"]):
            result = self.invoke([*prefix, PROBE, "identity"], launcher="launch-caps")
            self.check_identity(result, OPTIONS.user, True)


class FileReaderTests(unittest.TestCase):
    @contextmanager
    def worker(self, launcher=None, *, command=None, credentials=None):
        command = command or [str(HELPER), "--read-file-server-v1"]
        if launcher:
            command = [str(PROBE), launcher, *command]
        child = subprocess.Popen(command, env={}, cwd=BUILD, stdin=subprocess.PIPE,
                                 stdout=subprocess.PIPE, stderr=subprocess.PIPE, bufsize=0,
                                 **(credentials or {}))
        try:
            hello = self.read_exact(child, 16)
            self.assertEqual(hello[:8], b"NDFILE01")
            self.path_max, self.chunk_max = struct.unpack("!II", hello[8:])
            self.assertGreater(self.path_max, 0)
            self.assertEqual(self.chunk_max, 32768)
            yield child
        finally:
            child.stdin.close()
            try:
                child.wait(timeout=5)
            except subprocess.TimeoutExpired:
                child.kill()
                child.wait(timeout=5)
            child.stdout.close()
            child.stderr.close()

    def read_exact(self, child, size):
        data = bytearray()
        while len(data) < size:
            ready, _, _ = select.select([child.stdout], [], [], 5)
            self.assertTrue(ready, "reader response timed out")
            part = os.read(child.stdout.fileno(), size - len(data))
            self.assertTrue(part, "reader response was truncated")
            data.extend(part)
        return bytes(data)

    def request(self, child, path, *, op=1, regular=True, limit=1 << 20):
        path = os.fsencode(path)
        if op == 2:
            regular, limit = False, 0
        packet = struct.pack("!IIQII", op, int(regular), limit, len(path), 0) + path
        child.stdin.write(packet)
        pieces = []
        while True:
            kind, length, code, number = struct.unpack("!IIII", self.read_exact(child, 16))
            self.assertLessEqual(length, self.chunk_max)
            payload = self.read_exact(child, length)
            if kind == 1:
                self.assertEqual((code, number), (0, 0))
                pieces.append(payload)
                continue
            if kind == 3:
                self.assertEqual((op, length, code, number), (2, 16, 0, 0))
                return payload, code, number
            self.assertEqual((kind, length), (2, 0))
            return b"".join(pieces), code, number

    def fixture(self, name, data=b"synthetic-token", mode=0o644):
        path = BUILD / (self._testMethodName + "-" + name)
        path.write_bytes(data)
        path.chmod(mode)
        return path

    def test_read_rotation_symlinks_and_stat(self):
        path = self.fixture("data", b"one\0two\n")
        link = BUILD / "reader-token-link"
        link.symlink_to(path)
        with self.worker() as child:
            data, code, number = self.request(child, link)
            self.assertTrue(data == b"one\0two\n", "file bytes changed")
            self.assertEqual((code, number), (0, 0))
            replacement = self.fixture("replacement", b"rotated-token")
            replacement.replace(path)
            data, code, number = self.request(child, link)
            self.assertTrue(data == b"rotated-token", "reader cached old bytes")
            self.assertEqual((code, number), (0, 0))
            info, code, number = self.request(child, path, op=2)
            sec, ns = struct.unpack("!qq", info)
            self.assertEqual(sec * 1000000000 + ns, path.stat().st_mtime_ns)
            path.unlink()
            self.assertEqual(self.request(child, link)[1:], (1, errno.ENOENT))

    def test_regular_type_and_size_boundaries(self):
        path = self.fixture("boundary", b"x" * (1 << 20))
        fifo = BUILD / "reader-fifo"
        os.mkfifo(fifo, 0o666)
        with self.worker() as child:
            data, code, number = self.request(child, path)
            self.assertEqual((len(data), code, number), (1 << 20, 0, 0))
            with path.open("ab") as file:
                file.write(b"x")
            self.assertEqual(self.request(child, path)[1:], (6, 0))
            self.assertEqual(self.request(child, BUILD)[1:], (5, 0))
            self.assertEqual(self.request(child, fifo)[1:], (5, 0))
            self.assertEqual(self.request(child, "/dev/null")[1:], (5, 0))
            data, code, number = self.request(child, path, regular=False, limit=0)
            self.assertEqual((len(data), code, number), ((1 << 20) + 1, 0, 0))
            self.assertEqual(self.request(child, "/dev/null", regular=False, limit=0), (b"", 0, 0))

    def test_permission_errors_and_no_diagnostic_content(self):
        path = self.fixture("denied", mode=0o600 if os.geteuid() == 0 else 0)
        try:
            with self.worker() as child:
                data, code, number = self.request(child, path)
                self.assertFalse(data)
                self.assertEqual((code, number), (1, errno.EACCES))
                self.assertEqual(self.request(child, b"")[1:], (1, errno.ENOENT))
                child.stdin.close()
                child.wait(timeout=5)
                self.assertFalse(child.stderr.read(), "unexpected reader diagnostic")
        finally:
            path.chmod(0o600)

    def test_malformed_requests_and_clean_eof(self):
        for case in ("unknown-operation", "unknown-flags", "reserved", "nul", "long-path", "truncated"):
            with self.subTest(case=case), self.worker() as child:
                op, flags, length, reserved, path = 1, 1, 0, 0, b""
                if case == "unknown-operation":
                    op = 99
                elif case == "unknown-flags":
                    flags = 2
                elif case == "reserved":
                    reserved = 1
                elif case == "nul":
                    length, path = 1, b"\0"
                elif case == "long-path":
                    length = self.path_max
                packet = struct.pack("!IIQII", op, flags, 1 << 20, length, reserved) + path
                child.stdin.write(packet[:7] if case == "truncated" else packet)
                child.stdin.close()
                self.assertEqual(child.wait(timeout=5), 1)
                self.assertFalse(child.stdout.read())
                self.assertFalse(child.stderr.read())
        with self.worker() as child:
            child.stdin.close()
            self.assertEqual(child.wait(timeout=5), 0)

    def check_reduced_status(self, child):
        data, code, number = self.request(child, "/proc/self/status")
        self.assertEqual((code, number), (0, 0))
        status = dict(line.split(b":", 1) for line in data.splitlines() if b":" in line)
        for name in (b"CapEff", b"CapPrm", b"CapInh", b"CapAmb"):
            self.assertEqual(int(status[name].strip(), 16), 0, name.decode())
        account = pwd.getpwnam(OPTIONS.user)
        self.assertEqual([int(v) for v in status[b"Uid"].split()], [account.pw_uid] * 4)
        self.assertEqual([int(v) for v in status[b"Gid"].split()], [account.pw_gid] * 4)

    def installed_parent(self, shape):
        account = pwd.getpwnam(OPTIONS.user)
        parent = BUILD / (self._testMethodName + "-parent")
        shutil.copyfile(PROBE, parent)
        parent.chmod(0o755)
        if shape == "setuid":
            parent.chmod(0o4755)
        else:
            # Linux VFS capability revision 2: effective flag, followed by the
            # low/high permitted and inheritable words (all little-endian).
            # Only this disposable copied probe receives CAP_DAC_OVERRIDE.
            metadata = struct.pack("<IIIII", 0x02000001, 1 << 1, 0, 0, 0)
            try:
                os.setxattr(parent, "security.capability", metadata)
            except OSError as error:
                if error.errno in (errno.EPERM, errno.EACCES, errno.ENOTSUP, errno.EINVAL):
                    self.skipTest("test filesystem or container does not permit synthetic file capabilities")
                raise
        credentials = dict(user=account.pw_uid, group=account.pw_gid,
                           extra_groups=os.getgrouplist(account.pw_name, account.pw_gid))
        path = self.fixture("root-only", mode=0o600)
        # Feature detection also proves that this copied executable really gains
        # read authority after exec from an ordinary real UID. No host files change.
        try:
            baseline = subprocess.run([str(parent), "read-file", str(path)], env={}, cwd=BUILD,
                                      stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=5,
                                      **credentials)
        except PermissionError:
            self.skipTest(f"container cannot execute the synthetic {shape} launch shape")
        if baseline.returncode:
            self.skipTest(f"test filesystem/container suppresses synthetic {shape} elevation")
        self.assertTrue(baseline.stdout == b"synthetic-token", "elevated parent did not read the fixture")
        command = [str(parent), "launch-checked-file", str(path), str(HELPER), "--read-file-server-v1"]
        with self.worker(command=command, credentials=credentials) as child:
            self.assertEqual(self.request(child, path), (b"", 1, errno.EACCES))
            self.check_reduced_status(child)
            child.stdin.close()
            self.assertEqual(child.wait(timeout=5), 0)
            report = dict(item.split(b"=", 1) for item in child.stderr.read().split())
            self.assertEqual(int(report[b"parent_uid"]), account.pw_uid)
            self.assertEqual(int(report[b"parent_euid"]), 0 if shape == "setuid" else account.pw_uid)
            self.assertEqual(report[b"parent_readable"], b"1")
            self.assertTrue(int(report[b"parent_cap_eff"], 16) & (1 << 1), "parent lacked CAP_DAC_OVERRIDE")

    @unittest.skipUnless(sys.platform.startswith("linux") and os.geteuid() == 0, "Linux root setuid test")
    def test_setuid_parent_file_authority_is_removed(self):
        self.installed_parent("setuid")

    @unittest.skipUnless(sys.platform.startswith("linux") and os.geteuid() == 0, "Linux root file-capability test")
    def test_file_capability_parent_authority_is_removed(self):
        self.installed_parent("file-capability")

    @unittest.skipUnless(sys.platform.startswith("linux") and os.geteuid() == 0, "Linux root capability test")
    def test_ambient_file_authority_is_removed_without_libcap(self):
        path = self.fixture("root-only", mode=0o600)
        # Prove that the synthetic launcher conveys actual read authority.
        baseline = subprocess.run([str(PROBE), "launch-file-caps", str(PROBE), "read-file", str(path)],
                                  env={}, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=5)
        self.assertEqual(baseline.returncode, 0)
        self.assertTrue(baseline.stdout == b"synthetic-token", "capability baseline did not read the fixture")
        with self.worker("launch-file-caps") as child:
            self.assertEqual(self.request(child, path)[1:], (1, errno.EACCES))
            self.check_reduced_status(child)


def main():
    global OPTIONS, BUILD, HELPER, FALLBACK, PROBE
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--cc", default="cc")
    parser.add_argument("--cflags", default="")
    parser.add_argument("--user", default="nobody", help="existing unprivileged target account")
    parser.add_argument("--capabilities", action="store_true", help="build with Linux libcap")
    parser.add_argument("--no-setres", action="store_true", help="exercise setuid/setgid portability path")
    parser.add_argument("--go-tests", action="store_true", help="run Go client tests against the compiled helper")
    parser.add_argument("--source", type=Path, default=ROOT / "src/collectors/utils/nd-run.c")
    OPTIONS, remaining = parser.parse_known_args()
    if pwd.getpwnam(OPTIONS.user).pw_uid == 0:
        parser.error("--user must be unprivileged")
    with tempfile.TemporaryDirectory(prefix="nd-run-test-") as directory:
        BUILD = Path(directory)
        # Root tests exec the probe after switching accounts. Keep only the synthetic build accessible.
        BUILD.chmod(0o755)
        config = configure(BUILD / "config", OPTIONS.user)
        fallback_config = configure(BUILD / "fallback-config", "nd-run-test-missing-account")
        HELPER, FALLBACK, PROBE = (BUILD / name for name in ("nd-run", "nd-run-fallback", "probe"))
        compile_binary(OPTIONS.source, HELPER, config)
        compile_binary(OPTIONS.source, FALLBACK, fallback_config)
        compile_binary(ROOT / "tests/nd-run/probe.c", PROBE, config)
        program = unittest.main(argv=[sys.argv[0], *remaining], exit=False)
        if not program.result.wasSuccessful():
            return 1
        if OPTIONS.go_tests:
            command = [
                "go", "-C", str(ROOT / "src/go"), "test", "-race", "-count=1",
                "./pkg/credentialfile", "./pkg/web",
                "./plugin/go.d/discovery/sdext/discoverer/httpsd",
            ]
            print(shlex.join(command), file=sys.stderr)
            env = dict(os.environ, NETDATA_TEST_ND_RUN=str(HELPER))
            completed = subprocess.run(command, env=env)
            if completed.returncode:
                print(f"Go client tests failed in {Path.cwd()} with status {completed.returncode}", file=sys.stderr)
            return completed.returncode
        return 0


if __name__ == "__main__":
    sys.exit(main())
