// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

// Service data
struct win_service {
    char *service_name;
    char *display_name;

    RRDSET *st_service_state;
    RRDDIM *rd_service_state_running;
    RRDDIM *rd_service_state_stopped;
    RRDDIM *rd_service_state_start_pending;
    RRDDIM *rd_service_state_stop_pending;
    RRDDIM *rd_service_state_continue_pending;
    RRDDIM *rd_service_state_pause_pending;
    RRDDIM *rd_service_state_paused;
    RRDDIM *rd_service_state_unknown;

    RRDSET *st_service_status;
    RRDDIM *rd_service_status_ok;
    RRDDIM *rd_service_status_error;
    RRDDIM *rd_service_status_unknown;
    RRDDIM *rd_service_status_degraded;
    RRDDIM *rd_service_status_pred_fail;
    RRDDIM *rd_service_status_starting;
    RRDDIM *rd_service_status_stopping;
    RRDDIM *rd_service_status_service;
    RRDDIM *rd_service_status_stressed;
    RRDDIM *rd_service_status_nonrecover;
    RRDDIM *rd_service_status_no_contact;
    RRDDIM *rd_service_status_lost_comm;

    COUNTER_DATA ServiceState;
    COUNTER_DATA ServiceStatus;
};

static DICTIONARY *win_services = NULL;

void dict_win_service_insert_cb(const DICTIONARY_ITEM *item, void *value, void *data __maybe_unused)
{
    struct win_service *ptr = value;
    const char *name = dictionary_acquired_item_name((DICTIONARY_ITEM *)item);
    LPENUM_SERVICE_STATUS_PROCESS service = data;

    ptr->service_name = strdupz(name);
    netdata_fix_chart_name(ptr->service_name);
    ptr->display_name = strdupz(service->lpDisplayName);
}

static void initialize(void)
{
    win_services = dictionary_create_advanced(
        DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct win_service));

    dictionary_register_insert_callback(win_services, dict_win_service_insert_cb, NULL);
}

static BOOL fill_dictionary_with_content()
{
    static PVOID buffer = NULL;
    static ULONG initial_length = 0x8000;

    LPENUM_SERVICE_STATUS_PROCESS service, services;
    DWORD bytes_needed = 0, total_services = 0;
    SC_HANDLE ndSCMH = OpenSCManager(NULL, NULL, SC_MANAGER_ENUMERATE_SERVICE | SC_MANAGER_CONNECT);
    if (!ndSCMH) {
        return -1;
    }

    // Query to obtain overall information
    BOOL ret = EnumServicesStatusEx(
        ndSCMH,
        SC_ENUM_PROCESS_INFO,
        SERVICE_WIN32,
        SERVICE_STATE_ALL,
        NULL,
        0,
        (LPDWORD)&bytes_needed,
        (LPDWORD)&total_services,
        NULL,
        NULL);

    if (GetLastError() == ERROR_MORE_DATA) {
        if (!buffer)
            buffer = HeapAlloc(GetProcessHeap(), HEAP_ZERO_MEMORY, bytes_needed);
        else
            buffer = HeapReAlloc(GetProcessHeap(), HEAP_ZERO_MEMORY, buffer, bytes_needed);
    }

    if (!buffer) {
        ret = FALSE;
        goto endServiceCollection;
    }

    services = (LPENUM_SERVICE_STATUS_PROCESS)buffer;

    for (ULONG i = 0; i < total_services; i++) {
        service = &services[i];
        struct win_service *p = dictionary_set(win_services, service->lpServiceName, service, sizeof(*p));
        if (!p)
            continue;

        p->ServiceState.current.Data = service->ServiceStatusProcess.dwCurrentState;
    }

endServiceCollection:

    CloseServiceHandle(ndSCMH);
    return ret;
}

int do_PerflibServices(int update_every, usec_t dt __maybe_unused)
{
    static bool initialized = false;

    if (unlikely(!initialized)) {
        initialize();
        initialized = true;
    }

    if (!fill_dictionary_with_content())
        return 0;

    return 0;
}
