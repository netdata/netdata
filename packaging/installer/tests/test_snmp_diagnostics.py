# SPDX-License-Identifier: GPL-3.0-or-later
"""Exercise the shipped collector helpers and manifest against a private fixture.

Unix tests source the real functions and initialize only private fixture state.
Windows tests retain checked section extraction. CI additionally runs complete
installed-path bundles on POSIX and Windows.
"""
import json
import os
import re
import shlex
import shutil
from pathlib import Path
import subprocess
import tempfile
import unittest
import zipfile

INSTALLER = Path(__file__).resolve().parents[1]
REPO = INSTALLER.parents[1]
CURRENT = "11111111-1111-4111-8111-111111111111"
PREVIOUS = "22222222-2222-4222-8222-222222222222"
ORPHAN = "33333333-3333-4333-8333-333333333333"
DEVICE = "device-00000000000000000007.zst"
CHECKPOINT = "checkpoint-00000000000000000001.zst"


def unique_marker(source, pattern, label):
    matches = list(re.finditer(pattern, source, re.MULTILINE))
    if len(matches) != 1:
        raise ValueError(f"expected one complete {label} marker, found {len(matches)}")
    return matches[0]


def collection_sections(source, windows):
    # Fail before writing any executable fixture if source boundaries drift.
    environment = unique_marker(source, r"^# --- environment detection -+\n", "environment")
    manifest = unique_marker(source, "^" + re.escape(
        "# emit MANIFEST.json LAST so every file (incl. summary.txt and README.md) is indexed\n"
    ), "manifest")
    heading = "zip" if windows else "tarball"
    archive = unique_marker(source, r"^# =+\n# " + heading + r"\n# =+\n", "archive section")
    if not environment.end() < manifest.start() < manifest.end() < archive.start():
        raise ValueError("environment, manifest and archive markers are out of order")
    return source[:environment.start()], source[manifest.end():archive.start()], source[archive.end():]


class SnmpDiagnosticsTests(unittest.TestCase):
    def collect(self, fixture, include=True, deadline=False):
        shell = os.environ.get("SUPPORT_BUNDLE_TEST_SHELL", "sh")
        windows = shell in ("powershell", "pwsh")
        script = INSTALLER / ("netdata-support-bundle.ps1" if windows else "netdata-support-bundle")
        if windows:
            source = script.read_text()
            prefix, manifest, _ = collection_sections(source, windows)
            body = r'''
$Work = Join-Path $env:SNMP_FIXTURE 'work'
New-Item -ItemType Directory -Path $Work -Force | Out-Null
$LibDir = Join-Path $env:SNMP_FIXTURE 'lib'
if ($env:SNMP_EXPIRED -eq '1') { $GlobalDeadline = (Get-Date).AddSeconds(-1) }
Collect-SnmpDiagnostics
# Ordinary text collection must still redact secrets even with raw SNMP enabled.
$GlobalDeadline = (Get-Date).AddSeconds(60)
Save-File 'config.txt' 'fixture config' (Join-Path $env:SNMP_FIXTURE 'config.txt')
$RuntimeSecs = 0; $NetdataProc = $null; $ApiOk = $false
'''
            body += manifest + '\nRemove-Item $Staging -Recurse -Force\n'
            path = fixture / "run.ps1"
            path.write_text(prefix + body)
            args = [shell, "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", str(path)]
            if include:
                args.append("-IncludeSnmpDiagnostics")
        else:
            body = r'''
set -u
ND_SUPPORT_BUNDLE_SOURCE_ONLY=1 . "$SUPPORT_BUNDLE_SCRIPT"
init_defaults
parse_options "$@"
init_staging
detect_timeout
HOST_SHORT=""; HOST_FQDN=""; RUN_USER=""
WORK="$SNMP_FIXTURE/work"
mkdir -p "$WORK"
LIBDIR="$SNMP_FIXTURE/lib"
[ "$SNMP_EXPIRED" = 1 ] && START_TS=0
collect_snmp_diagnostics
START_TS=$(date +%s)
collect_file config.txt 'fixture config' "$SNMP_FIXTURE/config.txt"
NETDATA_PID=""; api_ok=0; IS_CONTAINER=0
'''
            path = fixture / "run.sh"
            path.write_text(body + "\nwrite_manifest\n")
            args = (shell.split() + [str(path)] + (["--include-snmp-diagnostics"] if include else []))
        env = dict(os.environ, SNMP_FIXTURE=str(fixture), SNMP_EXPIRED=str(int(deadline)), ND_SUPPORT_BUNDLE_DEMOTED="1", SUPPORT_BUNDLE_SCRIPT=str(script))
        env["PATH"] = str(fixture / "bin") + os.pathsep + env["PATH"]
        result = subprocess.run(args, env=env, text=True, capture_output=True, timeout=60)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        manifest = json.loads((fixture / "work/MANIFEST.json").read_text(encoding="utf-8"))
        self.assertNotIn("SENTINEL-SECRET", (fixture / "work/config.txt").read_text())
        return manifest

    def fixture(self, root):
        (root / "config.txt").write_text("password=SENTINEL-SECRET\nkeep=this\n")
        diag = root / "lib/snmp/diagnostics"
        for run in (CURRENT, PREVIOUS, ORPHAN):
            (diag / "normal" / run).mkdir(parents=True)
            (diag / "normal" / run / DEVICE).write_bytes(b"\x28\xb5\x2f\xfdRAW-SNMP\x00\xff")
        (diag / "normal/runs.json").write_text(json.dumps({"current": CURRENT, "previous": PREVIOUS}) + "\n")
        (diag / "topology").mkdir()
        archive = REPO / "src/go/plugin/go.d/collector/snmp_topology/testdata/topology-diagnostic-archive-replay-v1.zst"
        (diag / "topology" / CHECKPOINT).write_bytes(archive.read_bytes())
        # Larger than FILE_CAP: binary evidence must never be line-truncated.
        (diag / "lifecycle.zst").write_bytes(b"\x00\xff\r\n" * 300000)
        (diag / "lifecycle.zst.tmp").write_bytes(b"unfinished")
        (diag / "private.pem").write_text("UNRELATED-SENTINEL")
        return diag

    def test_collection(self):
        cases = {
            "default omission": (False, False, "not_requested", 0),
            "explicit inclusion": (True, False, "complete", 5),
            "deadline omission": (True, True, "partial", 0),
        }
        for name, (include, deadline, status, count) in cases.items():
            with self.subTest(name=name), tempfile.TemporaryDirectory() as temp:
                root = Path(temp)
                diag = self.fixture(root)
                manifest = self.collect(root, include, deadline)
                self.assertEqual(manifest["snmp_diagnostics"], {"requested": include, "status": status, "files": count})
                self.assertEqual(manifest["secrets_redacted"], count == 0)
                self.assertEqual(manifest["pii_obfuscated"], count == 0)
                entries = [e for e in manifest["files"] if e["path"].startswith("06-state/snmp-diagnostics/")]
                self.assertEqual(len(entries), count)
                for entry in entries:
                    rel = entry["path"].split("06-state/snmp-diagnostics/", 1)[1]
                    self.assertNotIn(ORPHAN, rel)
                    self.assertFalse(entry["sanitized"])
                    self.assertFalse(entry["pii_obfuscated"])
                    data = (root / "work" / entry["path"]).read_bytes()
                    self.assertEqual(data, (diag / rel).read_bytes())
                    self.assertEqual(entry["bytes"], len(data))
                self.assertFalse(any(p.name.endswith(".tmp") for p in (root / "work").rglob("*")))

    def test_missing_and_invalid_index(self):
        for name, index in {
            "truncated": '{"current":',
            "unsafe identity": '{"current":"../../other"}',
            "duplicate identity": json.dumps({"current": CURRENT, "previous": CURRENT}),
            "extra field": json.dumps({"current": CURRENT, "unexpected": True}),
            "missing index": None,
        }.items():
            with self.subTest(name=name), tempfile.TemporaryDirectory() as temp:
                root = Path(temp)
                diag = self.fixture(root)
                path = diag / "normal/runs.json"
                if index is None:
                    path.unlink()
                else:
                    path.write_text(index)
                manifest = self.collect(root)
                self.assertEqual(manifest["snmp_diagnostics"]["status"], "partial")
                self.assertFalse(any(DEVICE in e["path"] for e in manifest["files"]))
                self.assertEqual((root / "work/06-state/snmp-diagnostics/topology" / CHECKPOINT).read_bytes(), (diag / "topology" / CHECKPOINT).read_bytes())

    @unittest.skipIf(os.name == "nt" or os.environ.get("SUPPORT_BUNDLE_TEST_SHELL") == "pwsh", "POSIX copy failure injection")
    def test_failed_copy_withholds_partial_file(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            self.fixture(root)
            (root / "bin").mkdir()
            copier = root / "bin/cp"
            copier.write_text(
                "#!/bin/sh\n"
                'case "$2" in */lifecycle.zst) printf partial > "$3"; exit 1;; esac\n'
                + "exec " + shlex.quote(shutil.which("cp")) + ' "$@"\n'
            )
            copier.chmod(0o755)
            manifest = self.collect(root)
            self.assertEqual(manifest["snmp_diagnostics"], {"requested": True, "status": "partial", "files": 4})
            self.assertFalse(any(p.name.startswith("lifecycle.zst") for p in (root / "work").rglob("*")))

    @unittest.skipUnless(os.environ.get("SUPPORT_BUNDLE_TEST_SHELL") == "pwsh", "PowerShell 7 allocation counters")
    def test_zip_streams_evidence(self):
        # Exercise the shipped packaging block. Update-mode ZIP writers allocate
        # at least the entire input; a create-mode writer only needs IO buffers.
        source = (INSTALLER / "netdata-support-bundle.ps1").read_text()
        _, _, packaging = collection_sections(source, True)
        map_start = unique_marker(packaging, "^" + re.escape(
            '$MapPath = Join-Path $Output "$BundleName.pseudonym-map.tsv"\n'
        ), "pseudonym map")
        packaging = packaging[:map_start.start()]
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            work = root / "bundle"
            work.mkdir()
            size = 16 * 1024 * 1024
            for n in range(4):
                with (work / (str(n) + ".zst")).open("wb") as f:
                    f.truncate(size)
            script = root / "zip.ps1"
            script.write_text(r"""
$ErrorActionPreference = 'Stop'
$Work = Join-Path $env:SNMP_FIXTURE 'bundle'
$Staging = $env:SNMP_FIXTURE
$Output = Join-Path $Staging 'output'
$BundleName = 'bundle'
$KeepStaging = $true
function Show-Info($message) { Write-Output $message }
$before = [GC]::GetTotalAllocatedBytes($true)
""" + packaging + '\nWrite-Output ("ALLOCATED=" + ([GC]::GetTotalAllocatedBytes($true) - $before))\n')
            result = subprocess.run(["pwsh", "-NoProfile", "-File", str(script)],
                                    env=dict(os.environ, SNMP_FIXTURE=str(root)),
                                    text=True, capture_output=True, timeout=60)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            allocated = int(next(line.split("=", 1)[1] for line in result.stdout.splitlines() if line.startswith("ALLOCATED=")))
            self.assertLess(allocated, size * 4, "ZIP creation allocated the complete evidence payload")
            with zipfile.ZipFile(root / "output/bundle.zip") as archive:
                self.assertEqual(set(archive.namelist()), {"bundle/" + str(n) + ".zst" for n in range(4)})
                for name in archive.namelist():
                    self.assertEqual(archive.getinfo(name).file_size, size)
                self.assertIsNone(archive.testzip())

    def test_missing_lifecycle(self):
        cases = {
            "normal and topology": (True, True, "partial", 4),
            "normal only": (True, False, "partial", 3),
            "topology only": (False, True, "partial", 1),
            "empty store": (False, False, "unavailable", 0),
        }
        for name, (normal, topology, status, count) in cases.items():
            with self.subTest(name=name), tempfile.TemporaryDirectory() as temp:
                root = Path(temp)
                diag = self.fixture(root)
                (diag / "lifecycle.zst").unlink()
                for section, keep in (("normal", normal), ("topology", topology)):
                    if not keep:
                        (diag / section).rename(root / section)
                manifest = self.collect(root)
                self.assertEqual(manifest["snmp_diagnostics"], {"requested": True, "status": status, "files": count})
                self.assertIn("lifecycle.zst: missing", (root / "work/06-state/snmp-diagnostics-status.txt").read_text())
                for entry in manifest["files"]:
                    if entry["path"].startswith("06-state/snmp-diagnostics/"):
                        rel = entry["path"].split("06-state/snmp-diagnostics/", 1)[1]
                        self.assertEqual((root / "work" / entry["path"]).read_bytes(), (diag / rel).read_bytes())

    def test_collection_section_boundaries(self):
        # Mutate source only in memory: malformed extracts must never execute.
        for windows in (True,):
            source = (INSTALLER / ("netdata-support-bundle.ps1" if windows else "netdata-support-bundle")).read_text()
            environment = next(line for line in source.splitlines(True) if line.startswith("# --- environment detection"))
            manifest = next(line for line in source.splitlines(True) if line.startswith("# emit MANIFEST.json LAST"))
            heading = "zip" if windows else "tarball"
            archive = re.search(r"(?m)^# =+\n# " + heading + r"\n# =+\n", source).group()
            cases = {
                "missing environment": source.replace(environment, ""),
                "duplicate environment": source.replace(environment, environment * 2),
                "incomplete environment": source.replace(environment, "# --- environment detection\n"),
                "missing manifest": source.replace(manifest, ""),
                "duplicate manifest": source.replace(manifest, manifest * 2),
                "incomplete manifest": source.replace(manifest, "# emit MANIFEST.json LAST\n"),
                "missing archive header": source.replace(archive, ""),
                "duplicate archive header": source.replace(archive, archive * 2),
                "incomplete archive header": source.replace(archive, "# " + heading + "\n"),
                "wrong order": manifest + source.replace(manifest, ""),
            }
            for name, changed in cases.items():
                with self.subTest(windows=windows, name=name), self.assertRaises(ValueError):
                    collection_sections(changed, windows)

    def test_run_selection(self):
        for name, previous, remove_run, expected in (
            ("current only", False, False, 4),
            ("indexed run vanished", True, True, 4),
        ):
            with self.subTest(name=name), tempfile.TemporaryDirectory() as temp:
                root = Path(temp)
                diag = self.fixture(root)
                if not previous:
                    (diag / "normal/runs.json").write_text(json.dumps({"current": CURRENT}))
                if remove_run:
                    (diag / "normal" / CURRENT).rename(root / "retired")
                manifest = self.collect(root)
                self.assertEqual(manifest["snmp_diagnostics"]["files"], expected)
                self.assertEqual(manifest["snmp_diagnostics"]["status"], "partial" if remove_run else "complete")

    def test_absent_diagnostics(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            (root / "config.txt").write_text("password=SENTINEL-SECRET\n")
            manifest = self.collect(root)
            self.assertEqual(manifest["snmp_diagnostics"]["status"], "unavailable")
            self.assertEqual(manifest["snmp_diagnostics"]["files"], 0)

    def test_symlinks(self):
        names = ["topology", "normal", "normal/" + CURRENT]
        if os.name != "nt":
            names += ["lifecycle.zst", "normal/" + CURRENT + "/" + DEVICE]
        for name in names:
            with self.subTest(name=name), tempfile.TemporaryDirectory() as temp:
                root = Path(temp)
                diag = self.fixture(root)
                path = diag / name
                outside = root / "outside"
                path.rename(outside)
                if os.name == "nt":
                    # Directory junctions do not require Windows developer mode.
                    subprocess.run(["cmd", "/c", "mklink", "/J", str(path), str(outside)], check=True, capture_output=True)
                else:
                    path.symlink_to(outside, target_is_directory=outside.is_dir())
                manifest = self.collect(root)
                self.assertEqual(manifest["snmp_diagnostics"]["status"], "partial")
                entries = [e["path"] for e in manifest["files"]]
                self.assertFalse(any(p.startswith("06-state/snmp-diagnostics/" + name) for p in entries))


if __name__ == "__main__":
    unittest.main()
