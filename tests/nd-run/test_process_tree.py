#!/usr/bin/env python3
"""Real Linux nd-run ownership tests; run in a disposable Linux environment.

Compile from source with no installed Agent dependency. Tests use the current
account, including an unprivileged container user. No process-wide searches or
process-group signals are used. Capability fixtures wrap only procfs reads.
"""

import ctypes
import errno
import os
from pathlib import Path
import pwd
import select
import shlex
import signal
import subprocess
import sys
import tempfile
import time
import unittest

ROOT = Path(__file__).resolve().parents[2]
BUILD = None
HELPER = None
PROBE = None


def compile_binary(output, sources, *flags, config=None):
    command = [os.environ.get("CC", "cc"), "-std=gnu11", "-Wall", "-Wextra", "-Werror",
               "-g", "-I", str(config or BUILD), *map(str, sources), *flags, "-o", str(output)]
    print(shlex.join(command), file=sys.stderr)
    subprocess.run(command, check=True)


class TreeTests(unittest.TestCase):
    def start(self, args, *, launcher=False, helper=None, alias=None):
        control_read, control_write = os.pipe()
        status_read, status_write = os.pipe()
        os.set_blocking(status_read, False)
        command = [str(helper or HELPER), "--supervise-tree", str(control_read),
                   str(status_write), "--", *map(str, args)]
        if launcher:
            command = [str(PROBE), "ignored-launcher", *command]
        extra_fd = os.dup(control_read if alias == "control" else status_write) if alias else None
        inherited = (control_read, status_write) + ((extra_fd,) if alias else ())
        process = subprocess.Popen(command, pass_fds=inherited,
                                   stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                                   stderr=subprocess.PIPE)
        os.close(control_read)
        os.close(status_write)
        if extra_fd is not None:
            os.close(extra_fd)
        self.addCleanup(process.stdout.close)
        self.addCleanup(process.stderr.close)
        self.addCleanup(self.cleanup, process, control_write, status_read)
        return process, control_write, status_read

    @staticmethod
    def cleanup(process, control, status):
        for fd in (control, status):
            try:
                os.close(fd)
            except OSError as error:
                if error.errno != errno.EBADF:
                    raise
        # Closing control requests owned cleanup. A failure must remain visible;
        # test cleanup does not fabricate a successful supervisor join.
        process.wait(timeout=10)

    def ready(self, process):
        pids = []
        deadline = time.monotonic() + 10
        # Read unbuffered bytes: buffered readline can hide a subsequent line
        # from select even though it is already available to Python.
        data = b""
        while b"ready\n" not in data:
            remaining = deadline - time.monotonic()
            self.assertGreater(remaining, 0, "payload readiness timeout")
            self.assertTrue(select.select([process.stdout], [], [], remaining)[0])
            chunk = os.read(process.stdout.fileno(), 4096)
            self.assertTrue(chunk, "payload exited before readiness")
            data += chunk
        for line in data.splitlines():
            if line.startswith(b"pid "):
                pids.append(int(line[4:]))
        self.assertEqual(len(pids), 2)
        return pids

    def complete(self, process, status_fd, expected):
        process.wait(timeout=10)
        frame = os.read(status_fd, 256)
        self.assertEqual(frame, f"NDTREE1 {expected}\n".encode())
        self.assertEqual(os.read(status_fd, 1), b"")
        normalized = os.waitstatus_to_exitcode(expected)
        self.assertEqual(process.returncode, normalized if normalized >= 0 else 128 - normalized)
        self.assertEqual(process.stderr.read(), b"")

    def assert_gone(self, pids):
        for pid in pids:
            with self.assertRaises(ProcessLookupError):
                os.kill(pid, 0)

    def test_exit_drains_detached_doublefork_and_clone_children(self):
        for mode in ("detached", "doublefork", "clone", "clone-parent"):
            with self.subTest(mode=mode):
                process, _, status = self.start([PROBE, mode, "exit"])
                pids = self.ready(process)
                self.complete(process, status, 23 << 8)
                self.assert_gone(pids)

    def test_cancel_drains_noncooperative_trees(self):
        for mode in ("detached", "doublefork", "clone", "clone-parent"):
            with self.subTest(mode=mode):
                process, control, status = self.start([PROBE, mode])
                pids = self.ready(process)
                self.assertFalse(select.select([status], [], [], 0)[0])
                os.write(control, b"cancel")
                self.complete(process, status, signal.SIGKILL)
                self.assert_gone(pids)

    def test_parent_control_eof_drains(self):
        process, control, status = self.start([PROBE, "doublefork"])
        pids = self.ready(process)
        os.close(control)
        self.complete(process, status, signal.SIGKILL)
        self.assert_gone(pids)

    def test_lost_status_reader_still_drains(self):
        process, _, status = self.start([PROBE, "doublefork"])
        pids = self.ready(process)
        os.close(status)
        process.wait(timeout=10)
        self.assertEqual(process.returncode, 126)
        self.assert_gone(pids)

    def test_inherited_ignored_sigchld_and_termination(self):
        process, _, status = self.start([PROBE, "doublefork"], launcher=True)
        pids = self.ready(process)
        process.send_signal(signal.SIGTERM)
        self.complete(process, status, signal.SIGKILL)
        self.assert_gone(pids)

    def test_payload_closes_protocol_and_restores_sigpipe(self):
        process, _, status = self.start([PROBE, "fds"], launcher=True)
        self.complete(process, status, 0)

    def test_protocol_aliases_are_rejected_before_launch(self):
        for alias in ("control", "status"):
            with self.subTest(alias=alias):
                process, _, status = self.start([PROBE, "detached"], alias=alias)
                process.wait(timeout=10)
                self.assertEqual(process.returncode, 126)
                self.assertEqual(process.stdout.read(), b"")
                self.assertEqual(os.read(status, 256), b"")
                self.assertIn(b"distinct private protocol pipes", process.stderr.read())

    def test_exec_failure_has_verified_empty_tree(self):
        process, _, status = self.start([BUILD / "missing-executable"])
        process.wait(timeout=10)
        self.assertEqual(process.returncode, 127)
        self.assertEqual(os.read(status, 256), b"NDTREE1 32512\n")
        self.assertIn(b"tree execvp", process.stderr.read())

    def test_capability_unavailable_does_not_launch_target(self):
        for name in ("no-children", "bad-namespace",
                     "setup-failed", "fork-failed", "nnp-set-failed", "nnp-get-failed"):
            with self.subTest(helper=name):
                process, _, status = self.start([PROBE, "detached"], helper=BUILD / name)
                process.wait(timeout=10)
                self.assertEqual(process.returncode, 126)
                self.assertEqual(process.stdout.read(), b"")
                self.assertEqual(os.read(status, 256), b"NDTREE1 unavailable\n")
                self.assertEqual(os.read(status, 1), b"")
                self.assertTrue(process.stderr.read())

    @unittest.skipUnless(os.geteuid() == 0, "setuid authority fixture requires root")
    def test_setuid_exec_cannot_escape_supervisor_identity(self):
        account = pwd.getpwnam("nobody")
        probe = BUILD / "setuid-probe"
        # Establish that this filesystem/kernel actually permits the privilege
        # gain; a nosuid mount or inherited NNP must not yield a false positive.
        direct = subprocess.run([probe], user=account.pw_uid, group=account.pw_gid,
                                extra_groups=[], capture_output=True, check=True)
        if direct.stdout != b"0 0 0 0 0\n":
            self.skipTest("fixture cannot establish setuid exec privilege gain")
        # Legacy exec policy is intentionally unchanged by the tree-mode fix.
        legacy = subprocess.run([BUILD / "nd-run-nobody", probe],
                                capture_output=True, check=True)
        self.assertEqual(legacy.stdout, direct.stdout)
        process, _, status = self.start([probe], helper=BUILD / "nd-run-nobody")
        self.complete(process, status, 0)
        expected = f"-1 {account.pw_uid} {account.pw_uid} {account.pw_uid} 1\n".encode()
        self.assertEqual(process.stdout.read(), expected)

    def test_unavailable_without_echild_has_no_frame(self):
        process, _, status = self.start([PROBE, "detached"], helper=BUILD / "wait-not-empty")
        process.wait(timeout=10)
        self.assertEqual(process.returncode, 126)
        self.assertEqual(process.stdout.read(), b"")
        self.assertEqual(os.read(status, 256), b"")

    def test_unavailable_lost_reader_does_not_die_of_sigpipe(self):
        process, _, status = self.start([PROBE, "detached"], helper=BUILD / "no-children")
        os.close(status)
        process.wait(timeout=10)
        self.assertEqual(process.returncode, 126)
        self.assertEqual(process.stdout.read(), b"")

    def test_supervisor_loss_cannot_supply_completion(self):
        # The test runner is a subreaper only so this destructive failure fixture
        # can safely adopt, kill and reap its own recorded descendants afterwards.
        libc = ctypes.CDLL(None, use_errno=True)
        self.assertEqual(libc.prctl(36, 1, 0, 0, 0), 0)  # PR_SET_CHILD_SUBREAPER
        process, _, status = self.start([PROBE, "detached"])
        pids = self.ready(process)
        process.kill()
        process.wait(timeout=10)
        try:
            self.assertEqual(os.read(status, 256), b"")
        finally:
            # Kill/reap leader first; after its join the descendant is our child.
            # Each identity stays unreaped until its final signal, preventing reuse.
            for pid in pids:
                os.kill(pid, signal.SIGKILL)
                os.waitpid(pid, 0)
        self.assert_gone(pids)

    def test_exit_cancel_adoption_race(self):
        for _ in range(30):
            process, control, status = self.start([PROBE, "doublefork", "exit"])
            pids = self.ready(process)
            try:
                os.write(control, b"cancel")
            except BrokenPipeError:
                pass
            process.wait(timeout=10)
            frame = os.read(status, 256)
            self.assertIn(frame, (b"NDTREE1 5888\n", b"NDTREE1 9\n"))
            self.assert_gone(pids)


if __name__ == "__main__":
    if not sys.platform.startswith("linux"):
        raise SystemExit("tree supervision tests require Linux")
    with tempfile.TemporaryDirectory(prefix="nd-run-tree-") as directory:
        BUILD = Path(directory)
        HELPER = BUILD / "nd-run"
        PROBE = BUILD / "tree-probe"
        account = pwd.getpwuid(os.getuid()).pw_name
        (BUILD / "config.h").write_text(
            '#define _GNU_SOURCE 1\n#define HAVE_SETRESUID 1\n#define HAVE_SETRESGID 1\n'
            f'#define NETDATA_USER "{account}"\n')
        sources = [ROOT / "src/collectors/utils" / name for name in
                   ("nd-run.c", "nd-file-reader.c", "nd-process-tree.c")]
        fixture = ROOT / "tests/nd-run/tree_probe.c"
        compile_binary(HELPER, sources)
        compile_binary(PROBE, [fixture])
        for name, define in (("no-children", "TREE_MISSING_CHILDREN"),
                             ("bad-namespace", "TREE_BAD_NAMESPACE")):
            compile_binary(BUILD / name, [*sources, fixture], f"-D{define}", "-Wl,--wrap=fopen")
        compile_binary(BUILD / "setup-failed", [*sources, fixture],
                       "-DTREE_SETUP_FAIL", "-Wl,--wrap=prctl")
        compile_binary(BUILD / "fork-failed", [*sources, fixture],
                       "-DTREE_FORK_FAIL", "-Wl,--wrap=fork")
        compile_binary(BUILD / "wait-not-empty", [*sources, fixture],
                       "-DTREE_MISSING_CHILDREN", "-DTREE_WAIT_NOT_EMPTY",
                       "-Wl,--wrap=fopen", "-Wl,--wrap=waitpid")
        for name, define in (("nnp-set-failed", "TREE_NNP_SET_FAIL"),
                             ("nnp-get-failed", "TREE_NNP_GET_FAIL")):
            compile_binary(BUILD / name, [*sources, fixture], f"-D{define}", "-Wl,--wrap=prctl")
        if os.geteuid() == 0:
            BUILD.chmod(0o755)
            unprivileged = BUILD / "nobody-config"
            unprivileged.mkdir()
            (unprivileged / "config.h").write_text(
                '#define _GNU_SOURCE 1\n#define HAVE_SETRESUID 1\n#define HAVE_SETRESGID 1\n'
                '#define NETDATA_USER "nobody"\n')
            compile_binary(BUILD / "nd-run-nobody", sources, config=unprivileged)
            compile_binary(BUILD / "setuid-probe", [fixture], "-DTREE_PRIVILEGE_PROBE")
            (BUILD / "setuid-probe").chmod(0o4755)
        unittest.main(verbosity=2)
