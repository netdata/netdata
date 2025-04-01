// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"
#include "web/api/queries/backfill.h"

#ifdef ENABLE_SYSTEMD_DBUS
#include "daemon-systemd-watcher.h"
#endif

void *aclk_main(void *ptr);
void *analytics_main(void *ptr);
void *cpuidlejitter_main(void *ptr);
void *health_main(void *ptr);
void *pluginsd_main(void *ptr);
void *service_main(void *ptr);
void *statsd_main(void *ptr);
void *profile_main(void *ptr);
void *replication_thread_main(void *ptr);

extern bool pulse_enabled;

const struct netdata_static_thread static_threads_common[] = {
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
        .name = "PULSE",
        .config_section = CONFIG_SECTION_PLUGINS,
        .config_name = "netdata pulse",
        .env_name = "NETDATA_INTERNALS_MONITORING",
        .global_variable = &pulse_enabled,
        .enabled = 1,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = pulse_thread_main
    },
    {
        .name = "PULSE-SQLITE3",
        .config_section = CONFIG_SECTION_PULSE,
        .config_name = "extended",
        .env_name = NULL,
        .global_variable = &pulse_extended_enabled,
        .enabled = 0, // the default value - it uses netdata.conf for users to enable it
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = pulse_thread_sqlite3_main
    },
    {
        .name = "PULSE-WORKERS",
        .config_section = CONFIG_SECTION_PULSE,
        .config_name = "extended",
        .env_name = NULL,
        .global_variable = &pulse_extended_enabled,
        .enabled = 0, // the default value - it uses netdata.conf for users to enable it
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = pulse_thread_workers_main
    },
    {
        .name = "PULSE-MEMORY",
        .config_section = CONFIG_SECTION_PULSE,
        .config_name = "extended",
        .env_name = NULL,
        .global_variable = &pulse_extended_enabled,
        .enabled = 0, // the default value - it uses netdata.conf for users to enable it
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = pulse_thread_memory_extended_main},
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
        .start_routine = stream_sender_start_localhost},
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
        .enable_routine = httpd_is_enabled,
        .enabled = 0,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = h2o_main
    },
#endif

    {
        .name = "ACLK_MAIN",
        .config_section = NULL,
        .config_name = NULL,
        .enabled = 1,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = aclk_main
    },

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
    {
        .name = "BACKFILL",
        .config_section = NULL,
        .config_name = NULL,
        .enable_routine = netdata_conf_is_parent,
        .enabled = 0,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = backfill_thread
    },

#ifdef ENABLE_SYSTEMD_DBUS
    {
        .name = "SDBUSWATCHER",
        .config_section = NULL,
        .config_name = NULL,
        .enable_routine = NULL,
        .enabled = 1,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = systemd_watcher_thread
    },
#endif

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
