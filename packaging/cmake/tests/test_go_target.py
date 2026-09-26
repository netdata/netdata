#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""Exercise the real CMake Go helper with pure-Go and cgo build artifacts."""

import os
from pathlib import Path
import shlex
import shutil
import subprocess
import sys
import tempfile
import unittest


REPO = Path(__file__).resolve().parents[3]


@unittest.skipUnless(all(shutil.which(tool) for tool in ("cmake", "go", "cc")),
                     "requires CMake, Go and a C compiler")
class GoTargetTest(unittest.TestCase):
    def test_cgo_is_opt_in_and_extra_dependencies_rebuild(self):
        with tempfile.TemporaryDirectory(prefix="netdata-go-target-") as tmp:
            root = Path(tmp)
            module = root / "module"
            native = module / "native"
            pure = module / "pure"
            native.mkdir(parents=True)
            pure.mkdir()
            (module / "go.mod").write_text("module fixture\n\ngo 1.22\n")
            (module / "go.sum").touch()
            (native / "native.h").write_text("#define VALUE 10\nint value(void);\n")
            (native / "native.c").write_text('#include "native.h"\nint value(void) { return VALUE; }\n')
            (native / "value.txt").write_text("first")
            (native / "main.go").write_text('''package main
// #include "native.h"
import "C"
import "fmt"
import _ "embed"
//go:embed value.txt
var text string
func main() { fmt.Printf("%d:%s\\n", int(C.value()), text) }
''')
            (pure / "main.go").write_text('''package main
import "fmt"
import "runtime/debug"
func main() {
    info, _ := debug.ReadBuildInfo()
    for _, setting := range info.Settings {
        if setting.Key == "CGO_ENABLED" { fmt.Println(setting.Value); return }
    }
    panic("no CGO build setting")
}
''')
            compiler = root / "compiler with spaces"
            compiler.symlink_to(shutil.which("cc"))
            go = Path(shutil.which("go")).resolve()
            goroot = self.run_command([str(go), "env", "GOROOT"], root).strip()
            (root / "CMakeLists.txt").write_text(f'''cmake_minimum_required(VERSION 3.16)
project(go_target_fixture C)
set(GO_EXECUTABLE "{go}")
set(GO_ROOT "{goroot}")
set(CMAKE_C_COMPILER "{compiler}")
set(NETDATA_VERSION_STRING test)
include("{REPO}/packaging/cmake/Modules/NetdataGoTools.cmake")
add_go_target(native native-plugin module native CGO
    DEPENDS module/native/native.c module/native/native.h module/native/value.txt)
# Keep the pure target after cgo to catch accidental macro-state leakage.
add_go_target(pure pure-plugin module pure)
''')
            build = root / "build"
            self.run_command(["cmake", "-S", str(root), "-B", str(build)], root)
            build_command = ["cmake", "--build", str(build)]
            self.run_command(build_command, root)
            self.assertEqual(self.run_command([str(build / "pure-plugin")], root).strip(), "0")
            self.assertEqual(self.run_command([str(build / "native-plugin")], root).strip(), "10:first")

            # Change only non-Go inputs. Each must rebuild the actual executable.
            updates = [
                ("native.c", '#include "native.h"\nint value(void) { return VALUE + 1; }\n', "11:first"),
                ("native.h", "#define VALUE 20\nint value(void);\n", "21:first"),
                ("value.txt", "second", "21:second"),
            ]
            for filename, contents, expected in updates:
                with self.subTest(filename=filename):
                    (native / filename).write_text(contents)
                    self.run_command(build_command, root)
                    self.assertEqual(self.run_command([str(build / "native-plugin")], root).strip(), expected)

    def run_command(self, argv, cwd):
        print(f"+ {shlex.join(argv)}", file=sys.stderr)
        result = subprocess.run(argv, cwd=cwd, env=os.environ.copy(), text=True,
                                stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
        self.assertEqual(result.returncode, 0,
                         f"command failed in {cwd} with status {result.returncode}:\n{result.stdout}")
        return result.stdout


if __name__ == "__main__":
    unittest.main()
