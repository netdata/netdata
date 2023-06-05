// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf_unittest.h"

ebpf_module_t test_em;

/**
 * Initialize structure
 *
 * Initialize structure used to run unittests
 */
void ebpf_ut_initialize_structure(netdata_run_mode_t mode)
{
    memset(&em, 0, sizeof(ebpf_module_t));
    test_em.thread_name = strdupz("process");
    test_em.config_name = test_em.thread_name;
    test_em.kernels = NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4 | NETDATA_V5_10 |
                      NETDATA_V5_14;
    test_em.pid_map_size = ND_EBPF_DEFAULT_PID_SIZE;
    test_em.apps_level = NETDATA_APPS_LEVEL_REAL_PARENT;
    test_em.mode = mode;
}
