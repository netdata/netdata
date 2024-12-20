// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SYSTEM_IPC_H
#define NETDATA_SYSTEM_IPC_H

#include "common-contexts.h"

static inline void common_semaphore_ipc(uint64_t semaphore, NETDATA_DOUBLE red, char *module, int update_every) {
    static RRDSET *st_semaphores = NULL;
    static RRDDIM *rd_semaphores = NULL;
    if(unlikely(!st_semaphores)) {
        st_semaphores = rrdset_create_localhost("system"
                                                , "ipc_semaphores"
                                                , NULL
                                                , "ipc semaphores"
                                                , NULL
                                                , "IPC Semaphores"
                                                , "semaphores"
                                                , _COMMON_PLUGIN_NAME
                                                , module
                                                , NETDATA_CHART_PRIO_SYSTEM_IPC_SEMAPHORES
                                                , update_every
                                                , RRDSET_TYPE_AREA
                                                );
        rd_semaphores = rrddim_add(st_semaphores, "semaphores", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(st_semaphores, rd_semaphores, semaphore);
    rrdset_done(st_semaphores);
    if (!strcmp(module, "ipc"))
        st_semaphores->red = red;
}

#endif //NETDATA_SYSTEM_IPC_H
