// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"

char windows_shared_buffer[8192];

static struct proc_module {
    const char *name;
    const char *dim;
    int enabled;
    int (*func)(int update_every, usec_t dt);
    RRDDIM *rd;
} win_modules[] = {

    // system metrics
    {.name = "GetSystemUptime",     .dim = "GetSystemUptime",         .func = do_GetSystemUptime},
    {.name = "GetSystemRAM",        .dim = "GetSystemRAM",            .func = do_GetSystemRAM},

    // the same is provided by PerflibProcessor, with more detailed analysis
    //{.name = "GetSystemCPU",        .dim = "GetSystemCPU",            .func = do_GetSystemCPU},

    {.name = "PerflibProcesses",    .dim = "PerflibProcesses",        .func = do_PerflibProcesses},
    {.name = "PerflibProcessor",    .dim = "PerflibProcessor",        .func = do_PerflibProcessor},
    {.name = "PerflibMemory",       .dim = "PerflibMemory",           .func = do_PerflibMemory},
    {.name = "PerflibStorage",      .dim = "PerflibStorage",          .func = do_PerflibStorage},
    {.name = "PerflibNetwork",      .dim = "PerflibNetwork",          .func = do_PerflibNetwork},

    // the terminator of this array
    {.name = NULL, .dim = NULL, .func = NULL}
};

#if WORKER_UTILIZATION_MAX_JOB_TYPES < 36
#error WORKER_UTILIZATION_MAX_JOB_TYPES has to be at least 36
#endif

static void windows_main_cleanup(void *pptr) {
    struct netdata_static_thread *static_thread = CLEANUP_FUNCTION_GET_PTR(pptr);
    if(!static_thread) return;

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    collector_info("cleaning up...");

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;

    worker_unregister();
}

static bool log_windows_module(BUFFER *wb, void *data) {
    struct proc_module *pm = data;
    buffer_sprintf(wb, PLUGIN_WINDOWS_NAME "[%s]", pm->name);
    return true;
}

void *win_plugin_main(void *ptr) {
    worker_register("WIN");

    rrd_collector_started();
    PerflibNamesRegistryInitialize();

    CLEANUP_FUNCTION_REGISTER(windows_main_cleanup) cleanup_ptr = ptr;

    // check the enabled status for each module
    int i;
    for(i = 0; win_modules[i].name; i++) {
        struct proc_module *pm = &win_modules[i];

        pm->enabled = config_get_boolean("plugin:windows", pm->name, CONFIG_BOOLEAN_YES);
        pm->rd = NULL;

        worker_register_job_name(i, win_modules[i].dim);
    }

    usec_t step = localhost->rrd_update_every * USEC_PER_SEC;
    heartbeat_t hb;
    heartbeat_init(&hb);

#define LGS_MODULE_ID 0

    ND_LOG_STACK lgs[] = {
        [LGS_MODULE_ID] = ND_LOG_FIELD_TXT(NDF_MODULE, PLUGIN_WINDOWS_NAME),
        ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    while(service_running(SERVICE_COLLECTORS)) {
        worker_is_idle();
        usec_t hb_dt = heartbeat_next(&hb, step);

        if(unlikely(!service_running(SERVICE_COLLECTORS)))
            break;

        PerflibNamesRegistryUpdate();

        for(i = 0; win_modules[i].name; i++) {
            if(unlikely(!service_running(SERVICE_COLLECTORS)))
                break;

            struct proc_module *pm = &win_modules[i];
            if(unlikely(!pm->enabled))
                continue;

            worker_is_busy(i);
            lgs[LGS_MODULE_ID] = ND_LOG_FIELD_CB(NDF_MODULE, log_windows_module, pm);
            pm->enabled = !pm->func(localhost->rrd_update_every, hb_dt);
            lgs[LGS_MODULE_ID] = ND_LOG_FIELD_TXT(NDF_MODULE, PLUGIN_WINDOWS_NAME);
        }
    }
    return NULL;
}
