// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"

void *aclk_main(void *ptr);
void *analytics_main(void *ptr);
void *cpuidlejitter_main(void *ptr);
void *global_statistics_main(void *ptr);
void *global_statistics_workers_main(void *ptr);
void *global_statistics_sqlite3_main(void *ptr);
void *health_main(void *ptr);
void *pluginsd_main(void *ptr);
void *service_main(void *ptr);
void *statsd_main(void *ptr);
void *timex_main(void *ptr);
void *profile_main(void *ptr);
void *replication_thread_main(void *ptr __maybe_unused);

extern bool global_statistics_enabled;

const struct netdata_static_thread static_threads_common[] = {
    {
        .name = "P[timex]",
        .config_section = CONFIG_SECTION_PLUGINS,
        .config_name = "timex",
        .enabled = 1,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = timex_main
    },
    {
        .name = "P[idlejitter]",
        .config_section = CONFIG_SECTION_PLUGINS,
        .config_name = "idlejitter",
        .enabled = 1,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = cpuidlejitter_main
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
        .name = "ANALYTICS",
        .config_section = NULL,
        .config_name = NULL,
        .enabled = 0,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = analytics_main
    },
    {
        .name = "STATS_GLOBAL",
        .config_section = CONFIG_SECTION_PLUGINS,
        .config_name = "netdata monitoring",
        .env_name = "NETDATA_INTERNALS_MONITORING",
        .global_variable = &global_statistics_enabled,
        .enabled = 0,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = global_statistics_main
    },
    {
        .name = "STATS_WORKERS",
        .config_section = CONFIG_SECTION_PLUGINS,
        .config_name = "netdata monitoring extended",
        .env_name = "NETDATA_INTERNALS_MONITORING",
        .global_variable = &global_statistics_enabled,
        .enabled = 0, // this is ignored - check main() for "netdata monitoring extended"
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = global_statistics_workers_main
    },
    {
        .name = "STATS_SQLITE3",
        .config_section = CONFIG_SECTION_PLUGINS,
        .config_name = "netdata monitoring extended",
        .env_name = "NETDATA_INTERNALS_MONITORING",
        .global_variable = &global_statistics_enabled,
        .enabled = 0, // this is ignored - check main() for "netdata monitoring extended"
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = global_statistics_sqlite3_main
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
        .name = "STATSD_FLUSH",
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
        .name = "SNDR[localhost]",
        .config_section = NULL,
        .config_name = NULL,
        .enabled = 0,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = rrdpush_sender_thread
    },
    {
        .name = "WEB[1]",
        .config_section = NULL,
        .config_name = NULL,
        .enabled = 0,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = socket_listen_main_static_threaded
    },

#ifdef ENABLE_H2O
    {
        .name = "h2o",
        .config_section = NULL,
        .config_name = NULL,
        .enabled = 0,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = h2o_main
    },
#endif

#ifdef ENABLE_ACLK
    {
        .name = "ACLK_MAIN",
        .config_section = NULL,
        .config_name = NULL,
        .enabled = 1,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = aclk_main
    },
#endif

    {
        .name = "RRDCONTEXT",
        .config_section = NULL,
        .config_name = NULL,
        .enabled = 1,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = rrdcontext_main
    },

    {
        .name = "REPLAY[1]",
        .config_section = NULL,
        .config_name = NULL,
        .enabled = 1,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = replication_thread_main
    },
    {
        .name = "P[PROFILE]",
        .config_section = CONFIG_SECTION_PLUGINS,
        .config_name = "profile",
        .enabled = 0,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = profile_main
    },

    // terminator
    {
        .name = NULL,
        .config_section = NULL,
        .config_name = NULL,
        .env_name = NULL,
        .enabled = 0,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = NULL
    }
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
