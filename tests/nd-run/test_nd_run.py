#!/usr/bin/env python3
"""Compile and exercise the real nd-run helper with synthetic environments only."""

import argparse
import errno
import json
import os
from pathlib import Path
import pwd
import shlex
import signal
import select
import shutil
import struct
import subprocess
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
    sources = [str(source)]
    if source == OPTIONS.source:
        sources.append(str(ROOT / "src/collectors/utils/nd-file-reader.c"))
    command = [OPTIONS.cc, "-std=gnu11", "-Wall", "-Wextra", "-Werror", "-g",
               *shlex.split(OPTIONS.cflags), "-I", str(config),
               "-I", str(ROOT / "src/collectors/utils"), *sources, "-o", str(output)]
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
        # Only argv[1] == --preserve-env is special; other leading dashes remain commands.
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
        if sys.platform.startswith("linux"):
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
        if os.geteuid() != 0:
            self.skipTest("requires root in a controlled environment")
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
    invoke = NdRunTests.invoke
    success = NdRunTests.success

    def fixture(self, name, data=b"synthetic-token", mode=0o644):
        path = BUILD / (self._testMethodName + "-" + name)
        path.write_bytes(data)
        path.chmod(mode)
        return path

    def read(self, path, *, policy="regular", limit=0, launcher=None, helper=None):
        return self.invoke(["--file-reader", "read", policy, limit, path],
                           launcher=launcher, helper=helper)

    def result(self, result, code=0, number=0, data=None):
        self.assertEqual(result.returncode, int(code != 0), "unexpected reader exit status")
        self.assertEqual(result.stderr, f"NDFILE {code} {number}\n".encode(),
                         "invalid terminal status")
        if data is not None:
            self.assertTrue(result.stdout == data, "reader bytes differ (content withheld)")

    def test_binary_bytes_empty_and_arbitrary_path_arguments(self):
        data = bytes(range(256)) * 513 + b"\0\r\n"
        for name in ("binary", "with space\nand newline", "--preserve-env", "-"):
            path = self.fixture(name, data)
            self.result(self.read(path), data=data)
        # Test an actual dash-leading relative argv value, not only an absolute path.
        path = BUILD / "--file-reader"
        path.write_bytes(data)
        path.chmod(0o644)
        self.result(self.read(path.name), data=data)
        self.result(self.read(self.fixture("empty", b"")), data=b"")

    def test_stat_nanoseconds_symlinks_and_rotation(self):
        path = self.fixture("data", b"first\0\n")
        link = BUILD / (self._testMethodName + "-link")
        link.symlink_to(path)
        self.result(self.read(link), data=b"first\0\n")
        replacement = self.fixture("replacement", b"rotated")
        replacement.replace(path)
        self.result(self.read(link), data=b"rotated")
        for ns in (1700000000123456789, -123456789):
            os.utime(path, ns=(ns, ns))
            actual_ns = path.stat().st_mtime_ns
            sec, nsec = divmod(actual_ns, 1000000000)
            self.result(self.invoke(["--file-reader", "stat", link]),
                        data=f"{sec} {nsec}\n".encode())
        # Point a symlink at a missing target without deleting a fixture.
        missing = BUILD / (self._testMethodName + "-missing-link")
        missing.symlink_to(BUILD / "nonexistent-file-reader-target")
        self.result(self.read(missing), 1, errno.ENOENT, b"")
        self.result(self.invoke(["--file-reader", "stat", missing]), 2, errno.ENOENT, b"")

    def test_size_boundaries_and_unlimited_read(self):
        for size in (0, 1, 32767, 32768, 32769, 65537):
            data = b"x" * size
            path = self.fixture(str(size), data)
            for limit in (0, size, size + 1):
                self.result(self.read(path, limit=limit), data=data)
            if size > 1:
                self.result(self.read(path, limit=size - 1), 6, 0, b"")
        self.result(self.read(path, limit="18446744073709551615"), data=data)
        self.result(self.read(path, limit="00065537"), data=data)

    def test_types_and_stream_policy(self):
        fifo = BUILD / (self._testMethodName + "-fifo")
        os.mkfifo(fifo, 0o644)
        for path in (BUILD, fifo, "/dev/null", "/dev/zero"):
            self.result(self.read(path), 5, 0, b"")
        self.result(self.read("/dev/null", policy="stream"), data=b"")
        self.result(self.read(BUILD, policy="stream"), 3, errno.EISDIR, b"")
        result = self.read("/dev/zero", policy="stream", limit=65537)
        self.result(result, 6, 0)
        self.assertGreater(len(result.stdout), 0, "expected partial bytes before terminal size failure")
        self.assertLessEqual(len(result.stdout), 65537)
        self.assertTrue(all(byte == 0 for byte in result.stdout))

    def test_fifo_stream_reads_until_writer_closes(self):
        fifo = BUILD / (self._testMethodName + "-fifo")
        os.mkfifo(fifo, 0o644)
        data = b"fifo\0stream\n"
        writer = os.open(fifo, os.O_RDWR | os.O_NONBLOCK)
        try:
            os.write(writer, data)
            with subprocess.Popen([str(HELPER), "--file-reader", "read", "stream", "0", str(fifo)],
                                  env={}, cwd=BUILD, stdout=subprocess.PIPE,
                                  stderr=subprocess.PIPE) as child:
                try:
                    self.assertTrue(select.select([child.stdout], [], [], 5)[0], "FIFO stream blocked")
                    self.assertTrue(os.read(child.stdout.fileno(), len(data)) == data)
                    os.close(writer)
                    writer = None
                    output, status = child.communicate(timeout=5)
                    self.assertEqual(child.returncode, 0)
                    self.assertFalse(output)
                    self.assertEqual(status, b"NDFILE 0 0\n")
                finally:
                    if child.poll() is None:
                        child.kill()
                        child.wait(timeout=5)
        finally:
            if writer is not None:
                os.close(writer)

    def test_permission_and_path_errors_hide_content(self):
        marker = b"synthetic-private-content-never-diagnostic"
        path = self.fixture("synthetic-private-path", marker, 0o600 if os.geteuid() == 0 else 0)
        try:
            for policy in ("regular", "stream"):
                result = self.read(path, policy=policy)
                self.result(result, 1, errno.EACCES, b"")
                self.assertNotIn(os.fsencode(path), result.stderr)
                self.assertNotIn(marker, result.stderr)
        finally:
            path.chmod(0o600)
        self.result(self.read(""), 1, errno.ENOENT, b"")
        self.result(self.invoke(["--file-reader", "stat", ""]), 2, errno.ENOENT, b"")
        self.result(self.read(path / "child"), 1, errno.ENOTDIR, b"")

    def test_malformed_arguments_have_no_file_result_or_path_echo(self):
        path = self.fixture("synthetic-private-path", b"private-marker")
        invalid = [[], ["read"], ["read", "regular"], ["read", "regular", "0"],
                   ["stat"], ["stat", path, "extra"], ["unknown", path],
                   ["read", "unknown", "0", path], ["read", "regular", "0", path, "extra"],
                   ["read", "regular", "0", path, "stat", path]]
        invalid.extend(["read", "regular", limit, path] for limit in
                       ("", "-1", "+1", " 1", "1 ", "1x", "0x10", "1\n", "18446744073709551616"))
        for arguments in invalid:
            with self.subTest(arguments_count=len(arguments)):
                result = self.invoke(["--file-reader", *arguments])
                self.assertEqual(result.returncode, 1)
                self.assertNotIn(b"NDFILE", result.stderr)
                self.assertNotIn(os.fsencode(path), result.stdout + result.stderr)
                self.assertNotIn(b"private-marker", result.stdout + result.stderr)

    def test_one_shot_does_not_wait_for_stdin_eof(self):
        path = self.fixture("data", b"one-shot")
        with subprocess.Popen([str(HELPER), "--file-reader", "read", "regular", "0", str(path)],
                              cwd=BUILD, env={}, stdin=subprocess.PIPE,
                              stdout=subprocess.PIPE, stderr=subprocess.PIPE) as child:
            # wait() keeps stdin open; a protocol/session loop would hang here.
            try:
                self.assertEqual(child.wait(timeout=5), 0)
                self.assertTrue(child.stdout.read() == b"one-shot")
                self.assertEqual(child.stderr.read(), b"NDFILE 0 0\n")
            finally:
                if child.poll() is None:
                    child.kill()
                    child.wait(timeout=5)

    def test_output_pipe_failure_and_cancellation(self):
        command = [str(PROBE), "launch-signals", str(HELPER), "--file-reader", "read", "stream", "0", "/dev/zero"]
        with subprocess.Popen(command, env={}, cwd=BUILD, stdout=subprocess.PIPE,
                              stderr=subprocess.PIPE) as child:
            child.stdout.close()
            try:
                self.assertEqual(child.wait(timeout=5), -signal.SIGPIPE)
                self.assertNotIn(b"NDFILE 0 0", child.stderr.read())
            finally:
                if child.poll() is None:
                    child.kill()
                    child.wait(timeout=5)

        fifo = BUILD / (self._testMethodName + "-blocked-fifo")
        os.mkfifo(fifo, 0o644)
        with subprocess.Popen([str(HELPER), "--file-reader", "read", "stream", "0", str(fifo)],
                              env={}, cwd=BUILD, stdout=subprocess.PIPE, stderr=subprocess.PIPE) as child:
            try:
                with self.assertRaises(subprocess.TimeoutExpired):
                    child.wait(timeout=0.1)
                child.kill()
                self.assertEqual(child.wait(timeout=5), -signal.SIGKILL)
                self.assertFalse(child.stdout.read())
                self.assertFalse(child.stderr.read())
            finally:
                if child.poll() is None:
                    child.kill()
                    child.wait(timeout=5)
        with subprocess.Popen(command, env={}, cwd=BUILD, stdout=subprocess.PIPE,
                              stderr=subprocess.PIPE) as child:
            try:
                self.assertEqual(len(child.stdout.read(1)), 1)
                self.assertIsNone(child.poll())
                child.terminate()  # Popen targets only the exact PID created above.
                self.assertEqual(child.wait(timeout=5), -signal.SIGTERM)
                self.assertNotIn(b"NDFILE 0 0", child.stderr.read())
            finally:
                if child.poll() is None:
                    child.kill()
                    child.wait(timeout=5)

    def check_reduced_status(self, result, account, groups):
        self.result(result)
        status = dict(line.split(b":", 1) for line in result.stdout.splitlines() if b":" in line)
        for name in (b"CapEff", b"CapPrm", b"CapInh", b"CapAmb"):
            self.assertEqual(int(status[name].strip(), 16), 0, name.decode())
        self.assertEqual([int(v) for v in status[b"Uid"].split()], [account.pw_uid] * 4)
        self.assertEqual([int(v) for v in status[b"Gid"].split()], [account.pw_gid] * 4)
        self.assertEqual({int(v) for v in status[b"Groups"].split()}, set(groups))

    @unittest.skipUnless(sys.platform.startswith("linux"), "Linux process identity")
    def test_process_identity_and_foreign_account_fallback(self):
        if os.geteuid() == 0:
            account = pwd.getpwnam(OPTIONS.user)
            credentials = dict(user=account.pw_uid, group=account.pw_gid, extra_groups=[account.pw_gid, 31415])
            groups = [account.pw_gid, 31415]
        else:
            account = pwd.getpwuid(os.getuid())
            credentials = {}
            baseline = self.invoke(["identity"], helper=PROBE)
            groups = [int(v) for v in baseline.stdout.splitlines()[1].split()]
        for helper in (HELPER, FALLBACK):
            result = subprocess.run([str(helper), "--file-reader", "read", "regular", "0", "/proc/self/status"],
                                    cwd=BUILD, env={}, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                    timeout=5, **credentials)
            self.check_reduced_status(result, account, groups)

    @unittest.skipUnless(sys.platform.startswith("linux") and os.geteuid() == 0, "Linux root ID test")
    def test_real_root_ids_are_normalized_in_all_modes(self):
        account = pwd.getpwnam(OPTIONS.user)
        for prefix in ([], ["--preserve-env", "--"]):
            result = self.invoke([*prefix, PROBE, "identity"], launcher="launch-real-root")
            NdRunTests.check_identity(self, result, OPTIONS.user, True)
        self.check_reduced_status(self.read("/proc/self/status", launcher="launch-real-root"),
                                  account, os.getgrouplist(account.pw_name, account.pw_gid))

    @unittest.skipUnless(os.geteuid() == 0, "root-only rejection test")
    def test_file_mode_rejects_root_target(self):
        helper = BUILD / "nd-run-root-target"
        compile_binary(OPTIONS.source, helper, configure(BUILD / "root-config", "root"))
        for prefix in ([], ["--preserve-env", "--"]):
            self.success(self.invoke([*prefix, PROBE, "exit", "0"], helper=helper))
        path = self.fixture("root-only", mode=0o600)
        result = self.read(path, helper=helper)
        self.assertEqual(result.returncode, 1)
        self.assertFalse(result.stdout)
        self.assertNotIn(b"NDFILE 0 0", result.stderr)
        self.assertNotIn(os.fsencode(path), result.stderr)

    def installed_parent(self, shape):
        account = pwd.getpwnam(OPTIONS.user)
        parent = BUILD / (self._testMethodName + "-parent")
        shutil.copyfile(PROBE, parent)
        parent.chmod(0o755)
        if shape == "setuid":
            parent.chmod(0o4755)
        else:
            metadata = struct.pack("<IIIII", 0x02000001, 1 << 1, 0, 0, 0)
            os.setxattr(parent, "security.capability", metadata)
        # Also put the same synthetic authority directly on nd-run. File
        # capabilities on a parent alone disappear when it execs a plain helper.
        privileged_helper = BUILD / (self._testMethodName + "-helper")
        shutil.copyfile(HELPER, privileged_helper)
        privileged_helper.chmod(0o4755 if shape == "setuid" else 0o755)
        if shape != "setuid":
            os.setxattr(privileged_helper, "security.capability", metadata)
        credentials = dict(user=account.pw_uid, group=account.pw_gid,
                           extra_groups=os.getgrouplist(account.pw_name, account.pw_gid))
        path = self.fixture("root-only", mode=0o600)
        baseline = subprocess.run([str(parent), "read-file", str(path)], env={}, cwd=BUILD,
                                  stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=5, **credentials)
        self.assertEqual(baseline.returncode, 0, f"synthetic {shape} authority baseline failed")
        self.assertTrue(baseline.stdout == b"synthetic-token", "authority baseline content differs")
        for target in (path, "/proc/self/status"):
            direct = subprocess.run([str(privileged_helper), "--file-reader", "read", "regular", "0", str(target)],
                                    env={}, cwd=BUILD, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                    timeout=5, **credentials)
            if target == path:
                self.result(direct, 1, errno.EACCES, b"")
            else:
                self.check_reduced_status(direct, account, credentials["extra_groups"])
            command = [str(parent), "launch-checked-file", str(path), str(HELPER),
                       "--file-reader", "read", "regular", "0", str(target)]
            result = subprocess.run(command, env={}, cwd=BUILD, stdout=subprocess.PIPE,
                                    stderr=subprocess.PIPE, timeout=5, **credentials)
            report, separator, terminal = result.stderr.partition(b"\n")
            self.assertTrue(separator, "missing parent authority report")
            report = dict(item.split(b"=", 1) for item in report.split())
            self.assertEqual(int(report[b"parent_uid"]), account.pw_uid)
            self.assertEqual(int(report[b"parent_euid"]), 0 if shape == "setuid" else account.pw_uid)
            self.assertEqual(report[b"parent_readable"], b"1")
            self.assertTrue(int(report[b"parent_cap_eff"], 16) & (1 << 1))
            result.stderr = terminal
            if target == path:
                self.result(result, 1, errno.EACCES, b"")
            else:
                self.check_reduced_status(result, account, credentials["extra_groups"])

    @unittest.skipUnless(sys.platform.startswith("linux") and os.geteuid() == 0, "Linux root setuid test")
    def test_setuid_parent_file_authority_is_removed(self):
        self.installed_parent("setuid")

    @unittest.skipUnless(sys.platform.startswith("linux") and os.geteuid() == 0, "Linux root file-capability test")
    def test_file_capability_parent_authority_is_removed(self):
        self.installed_parent("file-capability")

    @unittest.skipUnless(sys.platform.startswith("linux") and os.geteuid() == 0, "Linux root capability test")
    def test_root_and_ambient_file_authority_is_removed(self):
        path = self.fixture("root-only", mode=0o600)
        for launcher in (None, "launch-file-caps"):
            baseline = self.invoke(["read-file", path], helper=PROBE, launcher=launcher)
            self.assertEqual(baseline.returncode, 0)
            self.assertTrue(baseline.stdout == b"synthetic-token", "authority baseline did not read fixture")
            self.result(self.read(path, launcher=launcher), 1, errno.EACCES, b"")
            account = pwd.getpwnam(OPTIONS.user)
            self.check_reduced_status(self.read("/proc/self/status", launcher=launcher), account,
                                      os.getgrouplist(account.pw_name, account.pw_gid))


def main():
    global OPTIONS, BUILD, HELPER, FALLBACK, PROBE
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--cc", default="cc")
    parser.add_argument("--cflags", default="")
    parser.add_argument("--user", default="nobody", help="existing unprivileged target account")
    parser.add_argument("--no-setres", action="store_true", help="exercise setreuid/setregid portability path")
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
        return 0 if program.result.wasSuccessful() else 1


if __name__ == "__main__":
    sys.exit(main())
