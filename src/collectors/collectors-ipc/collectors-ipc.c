// SPDX-License-Identifier: GPL-3.0-or-later

#include "collectors-ipc.h"

#if defined(OS_LINUX)
const char *netdata_integration_pipename(enum netdata_integration_selector idx)
{
    const char *pipes[] = {"NETDATA_APPS_PIPENAME", "NETDATA_CGROUP_PIPENAME", "NETDATA_NV_PIPENAME"};
    const char *pipename = getenv(pipes[idx]);
    if (pipename)
        return pipename;

#ifdef _WIN32
    switch (idx) {
        case NETDATA_INTEGRATION_NETWORK_VIEWER_EBPF:
            return "\\\\?\\pipe\\netdata-nv-cli";
        case NETDATA_INTEGRATION_CGROUPS_EBPF:
            return "\\\\?\\pipe\\netdata-cg-cli";
        case NETDATA_INTEGRATION_APPS_EBPF:
        default:
            return "\\\\?\\pipe\\netdata-apps-cli";
    }
#else
    switch (idx) {
        case NETDATA_INTEGRATION_NETWORK_VIEWER_EBPF:
            return "/tmp/netdata-nv-ipc";
        case NETDATA_INTEGRATION_CGROUPS_EBPF:
            return "/tmp/netdata-cg-ipc";
        default:
        case NETDATA_INTEGRATION_APPS_EBPF:
            return "/tmp/netdata-apps-ipc";
    }
#endif
}

#endif // defined(OS_LINUX)