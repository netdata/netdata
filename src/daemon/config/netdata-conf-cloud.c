// SPDX-License-Identifier: GPL-3.0-or-later

#include "netdata-conf-cloud.h"
#include "../common.h"

size_t netdata_conf_cloud_query_threads(void) {
    size_t cpus = MIN(netdata_conf_cpus(), 256); // max 256 cores
    size_t threads = MIN(cpus * (netdata_conf_is_parent() ? 2 : 1), (size_t)libuv_worker_threads / 2);
    threads = MAX(threads, 6);

    // each one is a dedicated thread; more than the libuv threadpool could never run in parallel
    // when Cloud queries ran as pool jobs, so that stays the ceiling
    return (size_t)inicfg_get_number_range(&netdata_config, CONFIG_SECTION_CLOUD, "query threads",
                                           (long long)threads, 1, libuv_worker_threads);
}
