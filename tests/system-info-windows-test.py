#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""Compile real Windows probe functions with synthetic APIs; no Windows runtime required.

Run: python3 tests/system-info-windows-test.py
This checks probe decisions/ownership, not Windows ABI or native API behavior.
"""
import os
from pathlib import Path
import re
import shlex
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[1]


def function(source, name):
    match = re.search(r"^(?:static )?[^\n]+\b" + name + r"\([^;]*?\)\s*\{", source, re.M)
    if not match:
        raise AssertionError(f"Missing function: {name}")
    depth = 1
    end = match.end()
    while depth:
        depth += (source[end] == "{") - (source[end] == "}")
        end += 1
    return source[match.start():end]


PRELUDE = r'''
#include <assert.h>
#include <stdbool.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <wchar.h>
#include <locale.h>
#include <arpa/inet.h>
typedef unsigned int DWORD;
typedef unsigned long ULONG;
typedef unsigned long long ULONGLONG;
typedef int HANDLE;
#define INVALID_HANDLE_VALUE -1
#define GENERIC_READ 1
#define FILE_SHARE_VALID_FLAGS 1
#define OPEN_EXISTING 1
#define IOCTL_DISK_GET_LENGTH_INFO 1
#define DRIVE_FIXED 3
#define CP_UTF8 65001
#define ERROR_HOST_UNREACHABLE 1
#define ERROR_NETWORK_UNREACHABLE 2
#define ERROR_NO_DATA 3
#define ERROR_BUFFER_OVERFLOW 4
#define NO_ERROR 0
#define GAA_FLAG_INCLUDE_PREFIX 1
#define NETDATA_WIN_DETECTION_METHOD "windows-api"
struct rrdhost_system_info { char value[64]; unsigned writes, detection_writes; };
static const char *expected_key;
static bool expected_runtime = true;
static unsigned environment_writes;
static void rrdhost_system_info_detected_set(struct rrdhost_system_info *si, const char *key, const char *value) {
    assert(expected_runtime);
    assert(!strcmp(key, expected_key));
    snprintf(si->value, sizeof(si->value), "%s", value);
    si->writes++;
}
static int nd_setenv(const char *key, const char *value, int overwrite) {
    assert(!expected_runtime);
    assert(!strcmp(key, "NETDATA_SYSTEM_DISK_DETECTION"));
    assert(!strcmp(value, NETDATA_WIN_DETECTION_METHOD) && overwrite == 1);
    environment_writes++;
    return 0;
}
static void rrdhost_system_info_set_by_name(struct rrdhost_system_info *si, const char *key, const char *value) {
    assert(!expected_runtime);
    if (!strcmp(key, "NETDATA_SYSTEM_DISK_DETECTION")) {
        assert(!strcmp(value, NETDATA_WIN_DETECTION_METHOD));
        si->detection_writes++;
    }
    else {
        assert(!strcmp(key, expected_key));
        snprintf(si->value, sizeof(si->value), "%s", value);
        si->writes++;
    }
}
typedef struct { unsigned dwNumberOfProcessors; unsigned wProcessorArchitecture; } SYSTEM_INFO;
static void GetSystemInfo(SYSTEM_INFO *si) { si->dwNumberOfProcessors = 8; si->wProcessorArchitecture = 9; }
static char *netdata_windows_arch(DWORD value) { (void)value; assert(!"runtime architecture probe"); return NULL; }
static void netdata_windows_cpu_from_registry(struct rrdhost_system_info *si) {
    (void)si; assert(!"runtime registry probe");
}
typedef struct { struct { long long QuadPart; } Length; } GET_LENGTH_INFORMATION;
static DWORD drive_mask;
static unsigned drive_types[26], opened, closed;
static ULONGLONG sizes[26];
static int fail_open = -1, fail_ioctl = -1;
static DWORD GetLogicalDrives(void) { return drive_mask; }
static unsigned GetDriveTypeA(const char *root) { assert(!strcmp(root + 1, ":\\")); return drive_types[root[0] - 'A']; }
static HANDLE CreateFile(const char *path, unsigned access, unsigned share, int security, unsigned disposition, int flags, int template) {
    (void)access; (void)share; (void)security; (void)disposition; (void)flags; (void)template;
    assert(!strncmp(path, "\\\\.\\", 4));
    int idx = path[4] - 'A'; assert(drive_types[idx] == DRIVE_FIXED); opened++;
    return idx == fail_open ? INVALID_HANDLE_VALUE : idx;
}
static bool DeviceIoControl(HANDLE disk, unsigned code, int input, int input_size, GET_LENGTH_INFORMATION *length, unsigned size, DWORD *ret, int overlap) {
    (void)code; (void)input; (void)input_size; (void)size; (void)ret; (void)overlap;
    if (disk == fail_ioctl) return false;
    length->Length.QuadPart = sizes[disk]; return true;
}
static void CloseHandle(HANDLE disk) { (void)disk; closed++; }
typedef struct { DWORD dwForwardIfIndex; } MIB_IPFORWARDROW;
typedef struct unicast {
    struct unicast *Next;
    struct { struct sockaddr *lpSockaddr; } Address;
} *PIP_ADAPTER_UNICAST_ADDRESS;
typedef struct adapter {
    struct adapter *Next;
    DWORD IfIndex;
    wchar_t *FriendlyName;
    PIP_ADAPTER_UNICAST_ADDRESS FirstUnicastAddress;
} *PIP_ADAPTER_ADDRESSES;
static DWORD route_rc, adapters_rc;
static struct adapter adapter;
static unsigned conversion_calls, adapter_calls;
static bool fail_conversion;
static DWORD GetBestRoute(int destination, int source, MIB_IPFORWARDROW *route) {
    (void)destination; (void)source; route->dwForwardIfIndex = 1; return route_rc;
}
static DWORD GetAdaptersAddresses(int family, unsigned flags, void *reserved, PIP_ADAPTER_ADDRESSES out, ULONG *length) {
    (void)family; (void)flags; (void)reserved; (void)length;
    adapter_calls++; *out = adapter; return adapters_rc;
}
static int WideCharToMultiByte(unsigned cp, unsigned flags, const wchar_t *src, int src_len, char *dst, int size, void *def, void *used) {
    assert(cp == CP_UTF8 && flags == 0 && src_len == -1 && !def && !used);
    assert(!wcscmp(src, L"Ethernet \u03b1"));
    conversion_calls++;
    if (fail_conversion) return 0;
    const char utf8[] = "Ethernet \xce\xb1";
    if (dst) { assert(size == sizeof(utf8)); memcpy(dst, utf8, sizeof(utf8)); }
    return sizeof(utf8);
}
'''

TESTS = r'''
static void disk_tests(void) {
    struct rrdhost_system_info si = { .value = "previous" };
    expected_key = "NETDATA_SYSTEM_TOTAL_DISK_SIZE";
    drive_mask = (1U << 2) | (1U << 3) | (1U << 4) | (1U << 5);
    drive_types[2] = DRIVE_FIXED; sizes[2] = 1024;
    drive_types[3] = 5; /* empty optical drive */
    drive_types[4] = 2; /* empty removable card reader */
    drive_types[5] = DRIVE_FIXED; sizes[5] = 2048;
    netdata_windows_get_total_disk_size(&si, true);
    assert(si.writes == 1 && !strcmp(si.value, "3072"));
    assert(opened == 2 && closed == 2);
    fail_open = 5;
    netdata_windows_get_total_disk_size(&si, true);
    assert(si.writes == 1 && !strcmp(si.value, "3072"));
    fail_open = -1; fail_ioctl = 2;
    unsigned before = closed;
    netdata_windows_get_total_disk_size(&si, true);
    assert(si.writes == 1 && closed == before + 1);
    fail_ioctl = -1; sizes[2] = 0;
    netdata_windows_get_total_disk_size(&si, true);
    assert(si.writes == 1);
    drive_mask = 0;
    netdata_windows_get_total_disk_size(&si, true);
    assert(si.writes == 1);
}
static void startup_disk_tests(void) {
    struct rrdhost_system_info si = { .value = "previous" };
    expected_runtime = false;
    expected_key = "NETDATA_SYSTEM_TOTAL_DISK_SIZE";
    opened = closed = environment_writes = 0;
    drive_mask = (1U << 2) | (1U << 3) | (1U << 4) | (1U << 5);
    drive_types[2] = drive_types[5] = DRIVE_FIXED;
    drive_types[3] = 5; /* optical drive: never opened, even when populated */
    drive_types[4] = 2; /* removable drive: never opened, even when populated */
    sizes[2] = 1024; sizes[3] = sizes[4] = 8192; sizes[5] = 2048;
    netdata_windows_get_total_disk_size(&si, false);
    assert(si.writes == 1 && !strcmp(si.value, "3072"));
    assert(opened == 2 && closed == 2);
    assert(si.detection_writes == 1 && environment_writes == 1);

    /* Startup preserves its best-effort partial sum and detection metadata. */
    fail_open = 5;
    netdata_windows_get_total_disk_size(&si, false);
    assert(si.writes == 2 && !strcmp(si.value, "1024"));
    assert(opened == 4 && closed == 3);
    assert(si.detection_writes == 2 && environment_writes == 2);
    fail_open = -1; fail_ioctl = 2;
    netdata_windows_get_total_disk_size(&si, false);
    assert(si.writes == 3 && !strcmp(si.value, "2048"));
    assert(opened == 6 && closed == 5);
    assert(si.detection_writes == 3 && environment_writes == 3);
    fail_ioctl = -1;

    /* An inventory containing only non-fixed drives publishes zero at startup. */
    drive_mask = (1U << 3) | (1U << 4);
    netdata_windows_get_total_disk_size(&si, false);
    assert(si.writes == 4 && !strcmp(si.value, "0"));
    assert(opened == 6 && closed == 5);
    assert(si.detection_writes == 4 && environment_writes == 4);
    drive_mask = 0;
    netdata_windows_get_total_disk_size(&si, false);
    assert(si.writes == 4 && !strcmp(si.value, "0"));
    assert(si.detection_writes == 4 && environment_writes == 4);
    expected_runtime = true;
}
static void network_tests(void) {
    struct sockaddr_in sa = { .sin_family = AF_INET };
    assert(inet_pton(AF_INET, "192.0.2.1", &sa.sin_addr) == 1);
    struct unicast ua = { .Address.lpSockaddr = (struct sockaddr *)&sa };
    adapter = (struct adapter) { .IfIndex = 1, .FriendlyName = L"Ethernet \u03b1", .FirstUnicastAddress = &ua };
    char *iface, *ip;
    setlocale(LC_ALL, "C");
    assert(netdata_win_default_network(&iface, &ip) == 1);
    assert(!strcmp(iface, "Ethernet \xce\xb1") && !strcmp(ip, "192.0.2.1"));
    assert(conversion_calls == 2); free(iface); free(ip);
    fail_conversion = true;
    assert(netdata_win_default_network(&iface, &ip) == -1 && !iface && !ip);
    fail_conversion = false;
    ua.Address.lpSockaddr = NULL;
    assert(netdata_win_default_network(&iface, &ip) == 0 && !iface && !ip);
    ua.Address.lpSockaddr = (struct sockaddr *)&sa;
    adapter.IfIndex = 2;
    assert(netdata_win_default_network(&iface, &ip) == -1 && !iface && !ip);
    adapter.IfIndex = 1; route_rc = ERROR_HOST_UNREACHABLE;
    assert(netdata_win_default_network(&iface, &ip) == 0 && !iface && !ip);
    route_rc = 99;
    assert(netdata_win_default_network(&iface, &ip) == -1 && !iface && !ip);
    route_rc = 0; adapters_rc = ERROR_BUFFER_OVERFLOW; adapter_calls = 0;
    assert(netdata_win_default_network(&iface, &ip) == -1 && !iface && !ip && adapter_calls == 3);
}
int main(void) {
    struct rrdhost_system_info si = {0};
    expected_key = "NETDATA_SYSTEM_CPU_LOGICAL_CPU_COUNT";
    netdata_windows_get_cpu(&si, true);
    assert(si.writes == 1 && !strcmp(si.value, "8"));
    disk_tests(); startup_disk_tests(); network_tests();
    puts("Windows probe source/stub tests passed (not native Windows validation)");
}
'''


def main():
    hardware = (ROOT / "src/daemon/win_system-info.c").read_text()
    network = (ROOT / "src/libnetdata/os/windows-api/windows_api.c").read_text()
    parts = [PRELUDE]
    for name in ("netdata_windows_runtime_set", "netdata_windows_cpu_from_system_info", "netdata_windows_get_cpu",
                 "netdata_windows_get_disk_size", "netdata_windows_get_total_disk_size"):
        parts.append(function(hardware, name))
    parts.extend([function(network, "netdata_win_default_network"), TESTS])
    with tempfile.TemporaryDirectory(prefix="netdata-win-probe-test-") as directory:
        source = Path(directory) / "test.c"
        binary = Path(directory) / "test"
        source.write_text("\n".join(parts))
        subprocess.run(shlex.split(os.environ.get("CC", "cc")) +
                       ["-std=gnu11", "-Wall", "-Wextra", "-Werror", str(source), "-o", str(binary)], check=True)
        subprocess.run([str(binary)], check=True)


if __name__ == "__main__":
    main()
