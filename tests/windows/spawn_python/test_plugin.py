#!/usr/bin/env python3
# Minimal "plugin" used by test_spawn_python.c.
# Prints one line identifying the interpreter then exits cleanly.
import sys
print("INTERP=" + sys.executable, flush=True)
