// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

struct terminal_services_sessions {
    COUNTER_DATA active;
    COUNTER_DATA inactive;

    RRDSET *st;
    RRDDIM *rd_active;
    RRDDIM *rd_inactive;
};

static struct terminal_services_sessions sessions = {
    .active = {.key = "Active Sessions"},
    .inactive = {.key = "Inactive Sessions"},
};

static bool do_terminal_services(PERF_DATA_BLOCK *pDataBlock, int update_every)
{
    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, "Terminal Services");
    if (!pObjectType)
        return false;

    bool has_active = perflibGetObjectCounter(pDataBlock, pObjectType, &sessions.active);
    bool has_inactive = perflibGetObjectCounter(pDataBlock, pObjectType, &sessions.inactive);

    if (!sessions.st && (!has_active || !has_inactive))
        return false;

    if (unlikely(!sessions.st)) {
        sessions.st = rrdset_create_localhost(
            "terminal_services",
            "sessions",
            NULL,
            "sessions",
            "terminal_services.sessions",
            "Terminal Services sessions",
            "sessions",
            PLUGIN_WINDOWS_NAME,
            "PerflibTerminalServices",
            PRIO_TERMINAL_SERVICES_SESSIONS,
            update_every,
            RRDSET_TYPE_STACKED);

        sessions.rd_active = perflib_rrddim_add(sessions.st, "active", NULL, 1, 1, &sessions.active);
        sessions.rd_inactive = perflib_rrddim_add(sessions.st, "inactive", NULL, 1, 1, &sessions.inactive);
    }

    if (has_active)
        perflib_rrddim_set_by_pointer(sessions.st, sessions.rd_active, &sessions.active);
    if (has_inactive)
        perflib_rrddim_set_by_pointer(sessions.st, sessions.rd_inactive, &sessions.inactive);
    rrdset_done(sessions.st);

    return true;
}

int do_PerflibTerminalServices(int update_every, usec_t dt __maybe_unused)
{
    DWORD id = RegistryFindIDByName("Terminal Services");
    if (id == PERFLIB_REGISTRY_NAME_NOT_FOUND)
        return -1;

    PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
    if (!pDataBlock)
        return 0;

    do_terminal_services(pDataBlock, update_every);

    return 0;
}
