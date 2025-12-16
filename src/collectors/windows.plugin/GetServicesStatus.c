// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

// Service data
struct win_service {
    char *service_name;
    DWORD pid;

    RRDSET *st_service_state;
    RRDDIM *rd_service_state_running;
    RRDDIM *rd_service_state_stopped;
    RRDDIM *rd_service_state_start_pending;
    RRDDIM *rd_service_state_stop_pending;
    RRDDIM *rd_service_state_continue_pending;
    RRDDIM *rd_service_state_pause_pending;
    RRDDIM *rd_service_state_paused;
    RRDDIM *rd_service_state_unknown;

    COUNTER_DATA ServiceState;
};

static DICTIONARY *win_services = NULL;

void dict_win_service_insert_cb(const DICTIONARY_ITEM *item, void *value, void *data __maybe_unused)
{
    struct win_service *ptr = value;
    const char *name = dictionary_acquired_item_name((DICTIONARY_ITEM *)item);

    ptr->service_name = strdupz(name);
}

static void initialize(void)
{
    win_services = dictionary_create_advanced(
        DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct win_service));

    dictionary_register_insert_callback(win_services, dict_win_service_insert_cb, NULL);
}

static BOOL fill_dictionary_with_content()
{
    PVOID buffer = NULL;
    DWORD bytes_needed = 0;

    LPENUM_SERVICE_STATUS_PROCESS service, services;
    DWORD total_services = 0;
    SC_HANDLE ndSCMH = OpenSCManager(NULL, NULL, SC_MANAGER_ENUMERATE_SERVICE | SC_MANAGER_CONNECT);
    if (unlikely(!ndSCMH)) {
        return FALSE;
    }

    // Query to obtain overall information
    BOOL ret = EnumServicesStatusEx(
        ndSCMH,
        SC_ENUM_PROCESS_INFO,
        SERVICE_WIN32,
        SERVICE_STATE_ALL,
        (LPBYTE)buffer,
        bytes_needed,
        (LPDWORD)&bytes_needed,
        (LPDWORD)&total_services,
        NULL,
        NULL);

    if (ret) {
        // This only happens if there are truly 0 services in the system (a valid edge case).
        goto endServiceCollection;
    }

    if (GetLastError() != ERROR_MORE_DATA) {
        goto endServiceCollection;
    }

    buffer = HeapAlloc(GetProcessHeap(), HEAP_ZERO_MEMORY, bytes_needed);
    if (!buffer) {
        ret = FALSE;
        goto endServiceCollection;
    }

    if (!EnumServicesStatusEx(ndSCMH, SC_ENUM_PROCESS_INFO, SERVICE_WIN32, SERVICE_STATE_ALL,
                              (LPBYTE)buffer, bytes_needed, &bytes_needed, &total_services, NULL, NULL)) {
        goto endServiceCollection;
    }

    services = (LPENUM_SERVICE_STATUS_PROCESS)buffer;

    for (ULONG i = 0; i < total_services; i++) {
        service = &services[i];
        if (!service->lpServiceName || !*service->lpServiceName)
            continue;

        struct win_service *p = dictionary_set(win_services, service->lpServiceName, NULL, sizeof(*p));
        if (!p)
            continue;

        p->ServiceState.current.Data = service->ServiceStatusProcess.dwCurrentState;
        p->pid = service->ServiceStatusProcess.dwProcessId;
    }

    ret = TRUE;

endServiceCollection:
    if (buffer)
        HeapFree(GetProcessHeap(), 0, buffer);

    CloseServiceHandle(ndSCMH);
    return ret;
}

static RRDDIM *win_service_select_dim(struct win_service *p, uint32_t selector)
{
    // Values defined according to https://learn.microsoft.com/en-us/windows/win32/api/winsvc/ns-winsvc-service_status
    switch (selector) {
        case 1:
            return p->rd_service_state_stopped;
        case 2:
            return p->rd_service_state_start_pending;
        case 3:
            return p->rd_service_state_stop_pending;
        case 4:
            return p->rd_service_state_running;
        case 5:
            return p->rd_service_state_continue_pending;
        case 6:
            return p->rd_service_state_pause_pending;
        case 7:
            return p->rd_service_state_paused;
        case 8:
        default:
            return p->rd_service_state_unknown;
    }
}

static int
dict_win_services_charts_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct win_service *p = value;
    int *update_every = data;

    if (!p->st_service_state) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "service_%s_state", p->service_name);
        netdata_fix_chart_name(id);
        p->st_service_state = rrdset_create_localhost(
            "service",
            id,
            NULL,
            "service",
            "windows.service_state",
            "Service state",
            "state",
            PLUGIN_WINDOWS_NAME,
            "PerflibbService",
            PRIO_SERVICE_STATE,
            *update_every,
            RRDSET_TYPE_LINE);

        p->rd_service_state_running = rrddim_add(p->st_service_state, "running", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        p->rd_service_state_stopped = rrddim_add(p->st_service_state, "stopped", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        p->rd_service_state_start_pending =
            rrddim_add(p->st_service_state, "start_pending", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        p->rd_service_state_stop_pending =
            rrddim_add(p->st_service_state, "stop_pending", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        p->rd_service_state_continue_pending =
            rrddim_add(p->st_service_state, "continue_pending", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        p->rd_service_state_pause_pending =
            rrddim_add(p->st_service_state, "pause_pending", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        p->rd_service_state_paused = rrddim_add(p->st_service_state, "paused", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        p->rd_service_state_unknown = rrddim_add(p->st_service_state, "unknown", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(p->st_service_state->rrdlabels, "service", p->service_name, RRDLABEL_SRC_AUTO);
    }

    if (p->st_service_state) {
#define NETDATA_WINDOWS_SERVICE_STATE_TOTAL_STATES (8)
        uint32_t current_state = (uint32_t)p->ServiceState.current.Data;
        for (uint32_t i = 1; i <= NETDATA_WINDOWS_SERVICE_STATE_TOTAL_STATES; i++) {
            RRDDIM *dim = win_service_select_dim(p, i);
            if (!dim)
                continue;
            uint32_t chart_value = (current_state == i) ? 1 : 0;

            rrddim_set_by_pointer(p->st_service_state, dim, (collected_number)chart_value);
        }

        rrdset_done(p->st_service_state);
    }

    return 1;
}

int do_GetServicesStatus(int update_every, usec_t dt __maybe_unused)
{
#define NETDATA_SERVICE_MAX_TRY (5)
    static int limit = 0;
    static bool initialized = false;

    if (unlikely(!initialized)) {
        initialize();
        initialized = true;
    }

    if (!fill_dictionary_with_content()) {
        if (++limit == NETDATA_SERVICE_MAX_TRY) {
            nd_log(
                NDLS_COLLECTORS,
                NDLP_ERR,
                "Disabling thread after %u consecutive tries to open Service Management.",
                NETDATA_SERVICE_MAX_TRY);
            return -1;
        }
        return 0;
    }

    limit = 0;
    dictionary_sorted_walkthrough_read(win_services, dict_win_services_charts_cb, &update_every);

    return 0;
}
