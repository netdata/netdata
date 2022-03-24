// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"

extern void *aclk_starter(void *ptr);
extern void *analytics_main(void *ptr);
extern void *checks_main(void *ptr);
extern void *cpuidlejitter_main(void *ptr);
extern void *global_statistics_main(void *ptr);
extern void *health_main(void *ptr);
extern void *pluginsd_main(void *ptr);
extern void *service_main(void *ptr);
extern void *statsd_main(void *ptr);
extern void *timex_main(void *ptr);

const struct netdata_static_thread static_threads_common[] = {
    {
        .name = "PLUGIN[timex]",
        .config_section = CONFIG_SECTION_PLUGINS,
        .config_name = "timex",
        .enabled = 1,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = timex_main
    },
    {
        .name = "PLUGIN[check]",
        .config_section = CONFIG_SECTION_PLUGINS,
        .config_name = "checks",
        .enabled = 0,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = checks_main
    },
    {
        .name = "PLUGIN[idlejitter]",
        .config_section = CONFIG_SECTION_PLUGINS,
        .config_name = "idlejitter",
        .enabled = 1,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = cpuidlejitter_main
    },
    {
        .name = "ANALYTICS",
        .config_section = NULL,
        .config_name = NULL,
        .enabled = 0,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = analytics_main
    },
    {
        .name = "GLOBAL_STATS",
        .config_section = NULL,
        .config_name = NULL,
        .enabled = 1,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = global_statistics_main
    },
    {
        .name = "HEALTH",
        .config_section = NULL,
        .config_name = NULL,
        .enabled = 1,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = health_main
    },
    {
        .name = "PLUGINSD",
        .config_section = NULL,
        .config_name = NULL,
        .enabled = 1,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = pluginsd_main
    },
    {
        .name = "SERVICE",
        .config_section = NULL,
        .config_name = NULL,
        .enabled = 1,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = service_main
    },
    {
        .name = "STATSD",
        .config_section = NULL,
        .config_name = NULL,
        .enabled = 1,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = statsd_main
    },
    {
        .name = "EXPORTING",
        .config_section = NULL,
        .config_name = NULL,
        .enabled = 1,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = exporting_main
    },
    {
        .name = "STREAM",
        .config_section = NULL,
        .config_name = NULL,
        .enabled = 0,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = rrdpush_sender_thread
    },
    {
        .name = "WEB_SERVER[static1]",
        .config_section = NULL,
        .config_name = NULL,
        .enabled = 0,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = socket_listen_main_static_threaded
    },

#if defined(ENABLE_ACLK) || defined(ACLK_NG)
    {
        .name = "ACLK_Main",
        .config_section = NULL,
        .config_name = NULL,
        .enabled = 1,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = aclk_starter
    },
#endif

    {NULL, NULL, NULL, 0, NULL, NULL, NULL}
};

struct netdata_static_thread *
static_threads_concat(const struct netdata_static_thread *lhs,
                      const struct netdata_static_thread *rhs)
{
    struct netdata_static_thread *res;

    int lhs_size = 0;
    for (; lhs[lhs_size].name; lhs_size++) {}

    int rhs_size = 0;
    for (; rhs[rhs_size].name; rhs_size++) {}

    res = callocz(lhs_size + rhs_size + 1, sizeof(struct netdata_static_thread));

    for (int i = 0; i != lhs_size; i++)
        memcpy(&res[i], &lhs[i], sizeof(struct netdata_static_thread));

    for (int i = 0; i != rhs_size; i++)
        memcpy(&res[lhs_size + i], &rhs[i], sizeof(struct netdata_static_thread));

    return res;
}
