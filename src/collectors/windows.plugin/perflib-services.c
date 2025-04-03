// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

// Service data
struct win_service {
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

void dict_win_service_insert_cb(
    const DICTIONARY_ITEM *item __maybe_unused,
    void *value __maybe_unused,
    void *data __maybe_unused)
{
    ;
}

static void initialize(void)
{
    win_services = dictionary_create_advanced(
        DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct win_service));

    dictionary_register_insert_callback(win_services, dict_win_service_insert_cb, NULL);
}

int do_PerflibServices(int update_every, usec_t dt __maybe_unused)
{
    static bool initialized = false;

    if (unlikely(!initialized)) {
        initialize();
        initialized = true;
    }

    return 0;
}
