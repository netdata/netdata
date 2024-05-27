// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SYSTEM_INTERRUPTS_H
#define NETDATA_SYSTEM_INTERRUPTS_H

#include "common-contexts.h"

#define _system_interrupt_chart() \
   rrdset_create_localhost( \
        "system" \
        , "intr" \
        , NULL \
        , "interrupts" \
        , NULL \
        , "CPU Interrupts" \
        , "interrupts/s" \
        , _COMMON_PLUGIN_NAME \
        , _COMMON_PLUGIN_MODULE_NAME \
        , NETDATA_CHART_PRIO_SYSTEM_INTR \
        , update_every \
        , RRDSET_TYPE_LINE \
        )

#ifdef OS_WINDOWS
static inline void common_interrupts(COUNTER_DATA *interrupts, int update_every) {
    static RRDSET *st_intr = NULL;
    static RRDDIM *rd_interrupts = NULL;

    if(unlikely(!st_intr)) {
        st_intr = _system_interrupt_chart();

        rrdset_flag_set(st_intr, RRDSET_FLAG_DETAIL);

        rd_interrupts = rrddim_add(, , NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        rd_interrupts = perflib_rrddim_add(st_intr, "interrupts", NULL, 1, 1, interrupts);
    }

    (void)perflib_rrddim_set_by_pointer(st_intr, rd_interrupts, interrupts);
    rrdset_done(st_intr);
}
#endif

#ifdef OS_LINUX
static inline void common_interrupts(uint64_t interrupts, int update_every) {
    static RRDSET *st_intr = NULL;
    static RRDDIM *rd_interrupts = NULL;

    if(unlikely(!st_intr)) {
        st_intr = _system_interrupt_chart();

        rrdset_flag_set(st_intr, RRDSET_FLAG_DETAIL);

        rd_interrupts = rrddim_add(st_intr, "interrupts", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(st_intr, rd_interrupts, (collected_number)interrupts);
    rrdset_done(st_intr);
}
#endif

#endif //NETDATA_SYSTEM_INTERRUPTS_H
