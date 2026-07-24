// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPFGO_SHM_LIVENESS_H
#define NETDATA_EBPFGO_SHM_LIVENESS_H 1

#if defined(OS_LINUX)

#include "libnetdata/libnetdata.h"
#include <stdint.h>

/* Dynamic stale timeout shared by all eBPFGo SHM readers: 2× the publisher's
 * update_every + 5 s slack.  Falls back to 60 s when update_every_s == 0
 * (old writer that predates the update_every_s field).  The old hardcoded
 * 10 s value caused data blackouts for any update_every ≥ 10. */
static inline usec_t ebpfgo_shm_stale_timeout_ut(uint32_t update_every_s)
{
    if (update_every_s == 0)
        return 60ULL * USEC_PER_SEC;
    return (usec_t)update_every_s * 2ULL * USEC_PER_SEC + 5ULL * USEC_PER_SEC;
}

#endif /* OS_LINUX */

#endif /* NETDATA_EBPFGO_SHM_LIVENESS_H */
