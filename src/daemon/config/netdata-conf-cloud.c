// SPDX-License-Identifier: GPL-3.0-or-later

#include "netdata-conf-cloud.h"
#include "../common.h"

int netdata_conf_cloud_query_threads(void) {
    int cpus = MIN(get_netdata_cpus(), 256); // max 256 cores
    int threads = MIN(cpus * (stream_conf_is_parent(false) ? 2 : 1), libuv_worker_threads / 2);
    threads = MAX(threads, 6);

    threads = config_get_number(CONFIG_SECTION_CLOUD, "query threads", threads);
    if(threads < 1) {
        netdata_log_error("[" CONFIG_SECTION_CLOUD "].query threads in netdata.conf needs to be at least 1. Overwriting it.");
        threads = 1;
        config_set_number(CONFIG_SECTION_CLOUD, "query threads", threads);
    }
    return threads;
}
