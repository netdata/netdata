// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"

char windows_shared_buffer[8192];

static struct proc_module {
    const char *name;
    const char *dim;
    int enabled;
    int update_every;
    int (*func)(int update_every, usec_t dt);
    RRDDIM *rd;
    ND_THREAD *thread;
    void (*cleanup)();
} win_modules[] = {

    // system metrics
    {.name = "GetSystemUptime",
     .dim = "GetSystemUptime",
     .enabled = CONFIG_BOOLEAN_YES,
     .update_every = UPDATE_EVERY_MIN,
     .func = do_GetSystemUptime,
     .rd = NULL,
     .thread = NULL,
     .cleanup = NULL},
    {.name = "GetSystemRAM",
     .dim = "GetSystemRAM",
     .enabled = CONFIG_BOOLEAN_YES,
     .update_every = UPDATE_EVERY_MIN,
     .func = do_GetSystemRAM,
     .rd = NULL,
     .thread = NULL,
     .cleanup = NULL},
    {.name = "GetPowerSupply",
     .dim = "GetPowerSupply",
     .enabled = CONFIG_BOOLEAN_YES,
     .update_every = UPDATE_EVERY_MIN,
     .func = do_GetPowerSupply,
     .rd = NULL,
     .thread = NULL,
     .cleanup = NULL},
    {.name = "PerflibServices",
     .dim = "PerflibServices",
     .enabled = CONFIG_BOOLEAN_YES,
     .update_every = 30 * UPDATE_EVERY_MIN,
     .func = do_GetServicesStatus},

    // the same is provided by PerflibProcessor, with more detailed analysis
    //{.name = "GetSystemCPU",        .dim = "GetSystemCPU",       .enabled = CONFIG_BOOLEAN_YES,
    // .update_every = UPDATE_EVERY_MIN, .func = do_GetSystemCPU},

    {.name = "PerflibProcesses",
     .dim = "PerflibProcesses",
     .enabled = CONFIG_BOOLEAN_YES,
     .update_every = UPDATE_EVERY_MIN,
     .func = do_PerflibProcesses,
     .rd = NULL,
     .thread = NULL,
     .cleanup = NULL},
    {.name = "PerflibProcessor",
     .dim = "PerflibProcessor",
     .enabled = CONFIG_BOOLEAN_YES,
     .update_every = UPDATE_EVERY_MIN,
     .func = do_PerflibProcessor,
     .rd = NULL,
     .thread = NULL,
     .cleanup = NULL},
    {.name = "PerflibMemory",
     .dim = "PerflibMemory",
     .enabled = CONFIG_BOOLEAN_YES,
     .update_every = UPDATE_EVERY_MIN,
     .func = do_PerflibMemory,
     .rd = NULL,
     .thread = NULL,
     .cleanup = NULL},
    {.name = "PerflibStorage",
     .dim = "PerflibStorage",
     .enabled = CONFIG_BOOLEAN_YES,
     .update_every = UPDATE_EVERY_MIN,
     .func = do_PerflibStorage,
     .rd = NULL,
     .thread = NULL,
     .cleanup = NULL},
    {.name = "PerflibNetwork",
     .dim = "PerflibNetwork",
     .enabled = CONFIG_BOOLEAN_YES,
     .update_every = UPDATE_EVERY_MIN,
     .func = do_PerflibNetwork,
     .rd = NULL,
     .thread = NULL,
     .cleanup = NULL},
    {.name = "PerflibObjects",
     .dim = "PerflibObjects",
     .enabled = CONFIG_BOOLEAN_YES,
     .update_every = UPDATE_EVERY_MIN,
     .func = do_PerflibObjects,
     .rd = NULL,
     .thread = NULL,
     .cleanup = NULL},
    {.name = "PerflibHyperV",
     .dim = "PerflibHyperV",
     .enabled = CONFIG_BOOLEAN_YES,
     .update_every = 5 * UPDATE_EVERY_MIN,
     .func = do_PerflibHyperV,
     .rd = NULL,
     .thread = NULL,
     .cleanup = NULL},
    {.name = "PerflibThermalZone",
     .dim = "PerflibThermalZone",
     .enabled = CONFIG_BOOLEAN_NO,
     .update_every = 5 * UPDATE_EVERY_MIN,
     .func = do_PerflibThermalZone,
     .rd = NULL,
     .thread = NULL,
     .cleanup = NULL},
    {.name = "PerflibWebService",
     .dim = "PerflibWebService",
     .enabled = CONFIG_BOOLEAN_YES,
     .update_every = UPDATE_EVERY_MIN,
     .func = do_PerflibWebService,
     .rd = NULL,
     .thread = NULL,
     .cleanup = NULL},
    {.name = "PerflibMSSQL",
     .dim = "PerflibMSSQL",
     .enabled = CONFIG_BOOLEAN_YES,
     .update_every = 10 * UPDATE_EVERY_MIN,
     .func = do_PerflibMSSQL,
     .rd = NULL,
     .thread = NULL,
     .cleanup = do_PerflibMSSQL_cleanup},
    {.name = "PerflibNetFramework",
     .dim = "PerflibNetFramework",
     .enabled = CONFIG_BOOLEAN_YES,
     .update_every = UPDATE_EVERY_MIN,
     .func = do_PerflibNetFramework,
     .rd = NULL,
     .thread = NULL,
     .cleanup = NULL},
    {.name = "PerflibAD",
     .dim = "PerflibAD",
     .enabled = CONFIG_BOOLEAN_YES,
     .update_every = 10 * UPDATE_EVERY_MIN,
     .func = do_PerflibAD,
     .rd = NULL,
     .thread = NULL,
     .cleanup = NULL},
    {.name = "PerflibADCS",
     .dim = "PerflibADCS",
     .enabled = CONFIG_BOOLEAN_YES,
     .update_every = 10 * UPDATE_EVERY_MIN,
     .func = do_PerflibADCS,
     .rd = NULL,
     .thread = NULL,
     .cleanup = NULL},
    {.name = "PerflibADFS",
     .dim = "PerflibADFS",
     .enabled = CONFIG_BOOLEAN_YES,
     .update_every = 10 * UPDATE_EVERY_MIN,
     .func = do_PerflibADFS,
     .rd = NULL,
     .thread = NULL,
     .cleanup = NULL},
    {.name = "PerflibExchange",
     .dim = "PerflibExchange",
     .enabled = CONFIG_BOOLEAN_YES,
     .update_every = 10 * UPDATE_EVERY_MIN,
     .func = do_PerflibExchange,
     .rd = NULL,
     .thread = NULL,
     .cleanup = NULL},
    {.name = "PerflibNUMA",
     .dim = "PerflibNUMA",
     .enabled = CONFIG_BOOLEAN_YES,
     .update_every = UPDATE_EVERY_MIN,
     .func = do_PerflibNUMA,
     .rd = NULL,
     .thread = NULL,
     .cleanup = NULL},
    // the terminator of this array
    {.name = "PerflibASP",
     .dim = "PerflibASP",
     .enabled = CONFIG_BOOLEAN_YES,
     .update_every = UPDATE_EVERY_MIN,
     .func = do_PerflibASP,
     .rd = NULL,
     .thread = NULL,
     .cleanup = NULL},
    // the terminator of this array
    {.name = NULL, .dim = NULL, .func = NULL, .rd = NULL, .thread = NULL, .cleanup = NULL}};

#if WORKER_UTILIZATION_MAX_JOB_TYPES < 36
#error WORKER_UTILIZATION_MAX_JOB_TYPES has to be at least 36
#endif

static void windows_main_cleanup(void *pptr)
{
    struct netdata_static_thread *static_thread = CLEANUP_FUNCTION_GET_PTR(pptr);
    if (!static_thread)
        return;

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    // Cleanup registry resources
    PerflibNamesRegistryCleanup();

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;

    worker_unregister();
}

static bool log_windows_module(BUFFER *wb, void *data)
{
    struct proc_module *pm = data;
    buffer_sprintf(wb, PLUGIN_WINDOWS_NAME "[%s]", pm->name);
    return true;
}

static void *windows_plugin_thread_worker(void *ptr __maybe_unused)
{
    struct proc_module *mod = ptr;
    heartbeat_t hb;
    int update_every = mod->update_every;

    heartbeat_init(&hb, USEC_PER_SEC);
    usec_t step = USEC_PER_SEC * update_every;
    usec_t real_step = USEC_PER_SEC;

    usec_t last = now_realtime_usec();
    while (service_running(SERVICE_COLLECTORS)) {
        heartbeat_next(&hb);
        if (real_step < step) {
            real_step += USEC_PER_SEC;
            continue;
        }
        real_step = USEC_PER_SEC;
        usec_t now = now_realtime_usec();
        mod->func(update_every, now - last);
        last = now;
    }

    return NULL;
}

void *win_plugin_main(void *ptr)
{
    worker_register("WIN");

    rrd_collector_started();
    PerflibNamesRegistryInitialize();

    CLEANUP_FUNCTION_REGISTER(windows_main_cleanup) cleanup_ptr = ptr;

    // check the enabled status for each module
    int i;
    char buf[CONFIG_MAX_NAME + 1];
    int update_every = localhost->rrd_update_every;
    for (i = 0; win_modules[i].name; i++) {
        struct proc_module *pm = &win_modules[i];
        snprintfz(buf, CONFIG_MAX_NAME, "plugin:windows:%s", pm->name);

        pm->enabled = inicfg_get_boolean(&netdata_config, "plugin:windows", pm->name, pm->enabled);
        pm->rd = NULL;

        pm->update_every = (int)inicfg_get_duration_seconds(
            &netdata_config, buf, "update every", (update_every != pm->update_every) ? pm->update_every : update_every);

        if (pm->enabled && unlikely(update_every != pm->update_every)) {
            char tag_name[ND_THREAD_TAG_MAX];
            snprintfz(tag_name, ND_THREAD_TAG_MAX - 1, "WIN_PLUGIN[%d]", i);
            pm->thread = nd_thread_create(tag_name, NETDATA_THREAD_OPTION_DEFAULT, windows_plugin_thread_worker, pm);
        }

        worker_register_job_name(i, win_modules[i].dim);
    }

    heartbeat_t hb;
    heartbeat_init(&hb, localhost->rrd_update_every * USEC_PER_SEC);

#define LGS_MODULE_ID 0

    ND_LOG_STACK lgs[] = {
        [LGS_MODULE_ID] = ND_LOG_FIELD_TXT(NDF_MODULE, PLUGIN_WINDOWS_NAME),
        ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    while (service_running(SERVICE_COLLECTORS)) {
        worker_is_idle();
        usec_t hb_dt = heartbeat_next(&hb);

        if (unlikely(!service_running(SERVICE_COLLECTORS)))
            break;

        PerflibNamesRegistryUpdate();

        for (i = 0; win_modules[i].name; i++) {
            if (unlikely(!service_running(SERVICE_COLLECTORS)))
                break;

            struct proc_module *pm = &win_modules[i];
            // if we have a thread, we are already running it
            if (pm->thread)
                continue;

            if (unlikely(!pm->enabled))
                continue;

            worker_is_busy(i);
            lgs[LGS_MODULE_ID] = ND_LOG_FIELD_CB(NDF_MODULE, log_windows_module, pm);
            pm->enabled = !pm->func(pm->update_every, hb_dt);
            lgs[LGS_MODULE_ID] = ND_LOG_FIELD_TXT(NDF_MODULE, PLUGIN_WINDOWS_NAME);
        }
    }

    // Join threads
    for (i = 0; win_modules[i].name; i++) {
        struct proc_module *pm = &win_modules[i];
        if (pm->cleanup)
            pm->cleanup();

        if (pm->thread) {
            nd_thread_join(pm->thread);
        }
    }
    return NULL;
}
