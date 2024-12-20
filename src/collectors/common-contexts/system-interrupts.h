// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SYSTEM_INTERRUPTS_H
#define NETDATA_SYSTEM_INTERRUPTS_H

#include "common-contexts.h"

#define _

static inline void common_interrupts(uint64_t interrupts, int update_every, char *ext_module) {
    static RRDSET *st_intr = NULL;
    static RRDDIM *rd_interrupts = NULL;

    char *module =  (!ext_module) ? _COMMON_PLUGIN_MODULE_NAME: ext_module;

    if(unlikely(!st_intr)) {
        st_intr = rrdset_create_localhost( "system"
                                          , "intr"
                                          , NULL
                                          , "interrupts"
                                          , NULL
                                          , "CPU Interrupts"
                                          , "interrupts/s"
                                          , _COMMON_PLUGIN_NAME
                                          , module
                                          , NETDATA_CHART_PRIO_SYSTEM_INTR
                                          , update_every
                                          , RRDSET_TYPE_LINE);

        rd_interrupts = rrddim_add(st_intr, "interrupts", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(st_intr, rd_interrupts, (collected_number)interrupts);
    rrdset_done(st_intr);
}

#endif //NETDATA_SYSTEM_INTERRUPTS_H
