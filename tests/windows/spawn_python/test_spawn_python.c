// SPDX-License-Identifier: GPL-3.0-or-later
//
// test_spawn_python.c — validates python.d.plugin spawning on Windows
//
// Scenarios tested:
//   1. No -p flag  →  SearchPathA finds python3.exe / python.exe in PATH
//   2. -p<path>    →  explicit interpreter is used directly (bypasses PATH)
//   3. -p<path> when Python is NOT in PATH  →  -p still works
//
// Build (MSYS2 UCRT64, from repo root after a cmake build in ./build/):
//
//   cd tests/windows/spawn_python
//   gcc -std=gnu11 -DOS_WINDOWS -DNETDATA_INTERNAL_CHECKS \
//       -o test_spawn_python test_spawn_python.c \
//       -I ../../../src \
//       -L ../../../build/src/libnetdata \
//       -lnetdata -lws2_32 -lpthread -lshlwapi \
//       && ./test_spawn_python
//
// The test binary must be run from the same directory that contains
// test_plugin.py, or PLUGIN_PATH must be adjusted below.

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#ifdef OS_WINDOWS

#include <ctype.h>
#include <windows.h>

#include "libnetdata/libnetdata.h"
#include "libnetdata/spawn_server/spawn_popen.h"

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

static bool read_first_line(POPEN_INSTANCE *pi, char *buf, size_t len)
{
    FILE *fp = spawn_popen_stdout(pi);
    if (!fp) return false;
    if (!fgets(buf, (int)len, fp)) return false;
    // strip trailing newline
    size_t n = strlen(buf);
    if (n > 0 && buf[n - 1] == '\n') buf[--n] = '\0';
    if (n > 0 && buf[n - 1] == '\r') buf[--n] = '\0';
    return true;
}

// Case-insensitive path comparison that handles / vs \ slashes.
static bool paths_equal(const char *a, const char *b)
{
    size_t la = strlen(a), lb = strlen(b);
    if (la != lb) return false;
    for (size_t i = 0; i < la; i++) {
        char ca = (a[i] == '/') ? '\\' : (char)tolower((unsigned char)a[i]);
        char cb = (b[i] == '/') ? '\\' : (char)tolower((unsigned char)b[i]);
        if (ca != cb) return false;
    }
    return true;
}

static int pass_count = 0;
static int fail_count = 0;

static void report(const char *name, bool ok, const char *detail)
{
    if (ok) {
        printf("  PASS  %s\n", name);
        pass_count++;
    } else {
        printf("  FAIL  %s — %s\n", name, detail ? detail : "");
        fail_count++;
    }
}

// ---------------------------------------------------------------------------
// test helpers
// ---------------------------------------------------------------------------

// Returns the path of test_plugin.py relative to this binary.
static const char *plugin_path(void)
{
    static char buf[MAX_PATH + 1];
    GetModuleFileNameA(NULL, buf, MAX_PATH);
    char *last = strrchr(buf, '\\');
    if (!last) last = strrchr(buf, '/');
    if (last) *(last + 1) = '\0';
    strncat(buf, "test_plugin.py", MAX_PATH - strlen(buf));
    return buf;
}

// Spawns the plugin with cmd, reads the first output line, kills the process.
// Returns true if the output line starts with "INTERP=".
static bool spawn_and_read_interp(const char *cmd, char *interp_out, size_t interp_len)
{
    POPEN_INSTANCE *pi = spawn_popen_run(cmd);
    if (!pi) return false;

    char line[MAX_PATH + 32] = {0};
    bool got = read_first_line(pi, line, sizeof(line));
    spawn_popen_kill(pi, 3000);

    if (!got || strncmp(line, "INTERP=", 7) != 0) return false;

    strncpy(interp_out, line + 7, interp_len - 1);
    interp_out[interp_len - 1] = '\0';
    return true;
}

// ---------------------------------------------------------------------------
// tests
// ---------------------------------------------------------------------------

static void test_path_search(const char *plugin, const char *python_in_path)
{
    printf("\nTest 1: PATH search (no -p flag)\n");

    if (!python_in_path[0]) {
        printf("  SKIP  no python3.exe / python.exe found in PATH — "
               "install Python and add it to PATH to run this test\n");
        return;
    }

    char cmd[MAX_PATH * 2 + 32];
    snprintf(cmd, sizeof(cmd), "exec \"%s\" 1", plugin);

    char interp[MAX_PATH + 1] = {0};
    bool ok = spawn_and_read_interp(cmd, interp, sizeof(interp));

    report("spawn succeeds", ok, "spawn_popen_run returned NULL");
    if (ok)
        report("correct interpreter reported", paths_equal(interp, python_in_path),
               "interpreter path does not match the one found by SearchPathA");
}

static void test_p_override(const char *plugin, const char *python_in_path)
{
    printf("\nTest 2: -p<interpreter> override\n");

    if (!python_in_path[0]) {
        printf("  SKIP  no python3.exe found in PATH — cannot form the -p argument\n");
        return;
    }

    char cmd[MAX_PATH * 2 + 64];
    snprintf(cmd, sizeof(cmd), "exec \"%s\" 1 -p%s", plugin, python_in_path);

    char interp[MAX_PATH + 1] = {0};
    bool ok = spawn_and_read_interp(cmd, interp, sizeof(interp));

    report("spawn succeeds with -p", ok, "spawn_popen_run returned NULL");
    if (ok)
        report("-p interpreter used (not PATH fallback)",
               paths_equal(interp, python_in_path),
               "reported interpreter does not match the -p argument");
}

static void test_p_override_no_path(const char *plugin, const char *python_in_path)
{
    printf("\nTest 3: -p<interpreter> when Python is NOT in PATH\n");

    if (!python_in_path[0]) {
        printf("  SKIP  no python3.exe found — cannot form the -p argument\n");
        return;
    }

    // Temporarily hide Python from PATH so SearchPathA would fail.
    // We do this by setting PATH to a directory that definitely has no Python.
    char saved_path[32768] = {0};
    GetEnvironmentVariableA("PATH", saved_path, sizeof(saved_path));
    SetEnvironmentVariableA("PATH", "C:\\Windows\\System32");

    // Reset the cached python_searched flag so spawn_popen_run re-searches.
    // We cannot do this without rebuilding the static; instead we rely on the
    // fact that this is a fresh process run where the cache is not yet warm.
    // (If the cache is already warm from test 1/2, SearchPathA is not called
    // again anyway — this test verifies the -p path, which never calls SearchPathA.)

    char cmd[MAX_PATH * 2 + 64];
    snprintf(cmd, sizeof(cmd), "exec \"%s\" 1 -p%s", plugin, python_in_path);

    char interp[MAX_PATH + 1] = {0};
    bool ok = spawn_and_read_interp(cmd, interp, sizeof(interp));

    // Restore PATH before any report (even on failure).
    SetEnvironmentVariableA("PATH", saved_path);

    report("spawn succeeds with -p even without Python in PATH",
           ok, "spawn_popen_run returned NULL");
    if (ok)
        report("correct interpreter used",
               paths_equal(interp, python_in_path),
               "reported interpreter does not match -p argument");
}

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------

int main(int argc, char *argv[])
{
    printf("=== spawn_popen python.d.plugin test ===\n");

    // Initialise the spawn server (re-uses this binary as the server).
    const char *srv_argv[] = { argv[0], NULL };
    if (!netdata_main_spawn_server_init("test-spawn-python", 1, srv_argv)) {
        fprintf(stderr, "FATAL: cannot initialize spawn server\n");
        return 1;
    }

    const char *plugin = plugin_path();
    printf("Plugin path : %s\n", plugin);

    // Check that the test plugin is actually present.
    if (GetFileAttributesA(plugin) == INVALID_FILE_ATTRIBUTES) {
        fprintf(stderr, "FATAL: test_plugin.py not found at '%s'\n"
                        "       Run this binary from the tests/windows/spawn_python/ directory.\n",
                plugin);
        netdata_main_spawn_server_cleanup();
        return 1;
    }

    // Discover the Python interpreter that is in PATH (for tests 1 and 2).
    char python_in_path[MAX_PATH + 1] = {0};
    if (SearchPathA(NULL, "python3.exe", NULL, MAX_PATH, python_in_path, NULL) == 0)
        SearchPathA(NULL, "python.exe", NULL, MAX_PATH, python_in_path, NULL);
    printf("Python in PATH: %s\n", python_in_path[0] ? python_in_path : "(none)");

    test_path_search(plugin, python_in_path);
    test_p_override(plugin, python_in_path);
    test_p_override_no_path(plugin, python_in_path);

    netdata_main_spawn_server_cleanup();

    printf("\n=== Results: %d passed, %d failed ===\n", pass_count, fail_count);
    return fail_count ? 1 : 0;
}

#else // !OS_WINDOWS

int main(void)
{
    printf("This test is Windows-only. On non-Windows the spawn path does not "
           "use the python.d.plugin special-case.\n");
    return 0;
}

#endif
