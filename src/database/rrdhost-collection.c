// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdhost-collection.h"
#include "rrdset.h"

void rrd_finalize_collection_for_all_hosts(void) {
    RRDHOST *host;
    dfe_start_reentrant(rrdhost_root_index, host) {
        rrdhost_finalize_collection(host);
    }
    dfe_done(host);
}

void rrdhost_finalize_collection(RRDHOST *host) {
    ND_LOG_STACK lgs[] = {
        ND_LOG_FIELD_TXT(NDF_NIDL_NODE, rrdhost_hostname(host)),
        ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "RRD: 'host:%s' stopping data collection...",
           rrdhost_hostname(host));

    RRDSET *st;
    rrdset_foreach_read(st, host)
        rrdset_finalize_collection(st, true);
    rrdset_foreach_done(st);
}

bool rrdhost_matches_window(RRDHOST *host, time_t after, time_t before, time_t now) {
    time_t first_time_s, last_time_s;
    rrdhost_retention(host, now, rrdhost_is_online(host), &first_time_s, &last_time_s);
    return query_matches_retention(after, before, first_time_s, last_time_s, 0);
}
