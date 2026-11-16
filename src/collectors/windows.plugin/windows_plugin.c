// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"

#include <ctype.h>

__thread char windows_shared_buffer[8192];

// Converts a user selector into a simple pattern list. Windows names contain backslashes, which simple patterns
// treat as escapes, so they are doubled unless they escape a separator or a leading '!'.
char *windows_plugin_normalize_patterns(const char *input)
{
    char *source = strdupz(input);
    char *patterns = mallocz((strlen(source) * 2) + 1);
    char *read = source, *write = patterns;
    bool first = true;

    while (*read) {
        char *start = read;
        while (*read && *read != ',' && *read != '|') {
            if (*read == '\\' && read[1])
                read += 2;
            else
                read++;
        }

        char *end = read;
        while (start < end && isspace((unsigned char)*start))
            start++;
        while (end > start && isspace((unsigned char)end[-1]))
            end--;

        if (start < end) {
            if (!first)
                *write++ = ',';
            bool token_start = true;
            while (start < end) {
                if (*start == '\\' && start + 1 < end) {
                    char next = start[1];
                    if (next == ',' || next == '|' || (next == '!' && token_start)) {
                        *write++ = *start++;
                        *write++ = *start++;
                        token_start = false;
                    } else {
                        *write++ = '\\';
                        *write++ = '\\';
                        start++;
                        token_start = false;
                    }
                } else if (*start == '\\') {
                    *write++ = '\\';
                    *write++ = '\\';
                    start++;
                    token_start = false;
                } else {
                    *write++ = *start++;
                    token_start = false;
                }
            }
            first = false;
        }
        if (*read)
            read++;
    }

    *write = '\0';
    freez(source);
    return patterns;
}

// Builds "<prefix><instance>_<xxh3>_<suffix>" truncating only the visible instance part, so long or similar names
// stay unique and the full "<type>.<id>" fits RRD_ID_LENGTH_MAX.
void windows_plugin_instance_chart_id(
    char *dst,
    size_t size,
    size_t chart_type_length,
    const char *prefix,
    const char *instance,
    const char *suffix,
    size_t suffix_length)
{
    const size_t instance_length = strnlen(instance, sizeof(windows_shared_buffer));
    XXH64_hash_t hash = XXH3_64bits(instance, instance_length);
    const size_t fixed_length = strlen(prefix) + 1 + 16 + 1 + suffix_length;
    const size_t full_id_limit = RRD_ID_LENGTH_MAX - 1;
    const size_t type_prefix_length = chart_type_length + 1;
    size_t chart_id_limit = full_id_limit > type_prefix_length ? full_id_limit - type_prefix_length : 0;
    if (size && chart_id_limit >= size)
        chart_id_limit = size - 1;
    size_t visible_instance_length = chart_id_limit > fixed_length ? chart_id_limit - fixed_length : 0;
    snprintfz(
        dst,
        size,
        "%s%.*s_%016llx_%s",
        prefix,
        (int)visible_instance_length,
        instance,
        (unsigned long long)hash,
        suffix);
    netdata_fix_chart_name(dst);
}

static struct proc_module {
    const char *name;
    const char *dim;
    int enabled;
    int update_every;
    // Always run on a dedicated thread, so a blocking data source cannot stall the shared loop.
    bool dedicated_thread;
    int (*func)(int update_every, usec_t dt);
    RRDDIM *rd;
    ND_THREAD *thread;
    void (*cleanup)();
    // Optional; called from the main thread at shutdown, before joining the module's thread, to abort a blocking
    // call (e.g. a WMI ConnectServer, up to ~2 minutes) that would otherwise delay the unbounded join.
    void (*interrupt)(void);
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
     .cleanup = do_GetPowerSupply_cleanup},
    {.name = "GetSensors",
     .dim = "GetSensors",
     .enabled = CONFIG_BOOLEAN_YES,
     .update_every = UPDATE_EVERY_MIN,
     .func = do_GetSensors,
     .rd = NULL,
     .thread = NULL,
     .cleanup = do_Sensors_cleanup},
    {.name = "GetHardwareInfo",
     .dim = "GetHardwareInfo",
     .enabled = CONFIG_BOOLEAN_YES,
     .update_every = 10 * UPDATE_EVERY_MIN,
     .func = do_GetHardwareInfo,
     .rd = NULL,
     .thread = NULL,
     .cleanup = do_GetHardwareInfo_cleanup},
    {.name = "PerflibServices",
     .dim = "PerflibServices",
     .enabled = CONFIG_BOOLEAN_YES,
     .update_every = 30 * UPDATE_EVERY_MIN,
     .func = do_GetServicesStatus,
     .rd = NULL,
     .thread = NULL,
     .cleanup = do_GetServicesStatus_cleanup},
    {.name = "GetPrinters",
     .dim = "GetPrinters",
     .enabled = CONFIG_BOOLEAN_NO,
     .update_every = 10 * UPDATE_EVERY_MIN,
     .dedicated_thread = true,
     .func = do_GetPrinters,
     .rd = NULL,
     .thread = NULL,
     .cleanup = do_GetPrinters_cleanup},

    // the same is provided by PerflibProcessor, with more detailed analysis
    //{.name = "GetSystemCPU",        .dim = "GetSystemCPU",       .enabled = CONFIG_BOOLEAN_YES,
    // .update_every = UPDATE_EVERY_MIN, .func = do_GetSystemCPU, .cleanup = do_GetSystemCPU_cleanup},

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
     .cleanup = do_PerflibStorage_cleanup},
    {.name = "PerflibNetwork",
     .dim = "PerflibNetwork",
     .enabled = CONFIG_BOOLEAN_YES,
     .update_every = UPDATE_EVERY_MIN,
     .func = do_PerflibNetwork,
     .rd = NULL,
     .thread = NULL,
     .cleanup = NULL},
    {.name = "PerflibSMB",
     .dim = "PerflibSMB",
     .enabled = CONFIG_BOOLEAN_YES,
     .update_every = UPDATE_EVERY_MIN,
     .func = do_PerflibSMB,
     .rd = NULL,
     .thread = NULL,
     .cleanup = do_PerflibSMB_cleanup},
    {.name = "PerflibSMBClient",
     .dim = "PerflibSMBClient",
     .enabled = CONFIG_BOOLEAN_YES,
     .update_every = UPDATE_EVERY_MIN,
     .func = do_PerflibSMBClient,
     .rd = NULL,
     .thread = NULL,
     .cleanup = do_PerflibSMBClient_cleanup},
    {.name = "PerflibTerminalServices",
     .dim = "PerflibTerminalServices",
     .enabled = CONFIG_BOOLEAN_YES,
     .update_every = UPDATE_EVERY_MIN,
     .func = do_PerflibTerminalServices,
     .rd = NULL,
     .thread = NULL,
     .cleanup = NULL},
    {.name = "PerflibDNS",
     .dim = "PerflibDNS",
     .enabled = CONFIG_BOOLEAN_YES,
     .update_every = UPDATE_EVERY_MIN,
     .func = do_PerflibDNS,
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
    {.name = "PerflibHttpService",
     .dim = "PerflibHttpService",
     .enabled = CONFIG_BOOLEAN_YES,
     .update_every = UPDATE_EVERY_MIN,
     .func = do_PerflibHttpService,
     .rd = NULL,
     .thread = NULL,
     .cleanup = NULL},
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
    {.name = "PerflibMSMQ",
     .dim = "PerflibMSMQ",
     .enabled = CONFIG_BOOLEAN_YES,
     .update_every = UPDATE_EVERY_MIN,
     .func = do_PerflibMSMQ,
     .rd = NULL,
     .thread = NULL,
     .cleanup = do_PerflibMSMQ_cleanup},
    {.name = "PerflibNUMA",
     .dim = "PerflibNUMA",
     .enabled = CONFIG_BOOLEAN_YES,
     .update_every = UPDATE_EVERY_MIN,
     .func = do_PerflibNUMA,
     .rd = NULL,
     .thread = NULL,
     .cleanup = NULL},
    {.name = "PerflibASP",
     .dim = "PerflibASP",
     .enabled = CONFIG_BOOLEAN_YES,
     .update_every = UPDATE_EVERY_MIN,
     .func = do_PerflibASP,
     .rd = NULL,
     .thread = NULL,
     .cleanup = NULL},
    {.name = "PerflibPagingFile",
     .dim = "PerflibPagingFile",
     .enabled = CONFIG_BOOLEAN_YES,
     .update_every = UPDATE_EVERY_MIN,
     .func = do_PerflibPagingFile,
     .rd = NULL,
     .thread = NULL,
     .cleanup = do_PerflibPagingFile_cleanup},

    // WMI metrics
    {.name = "GetFSRMQuota",
     .dim = "GetFSRMQuota",
     .enabled = CONFIG_BOOLEAN_NO,
     .update_every = 10 * UPDATE_EVERY_MIN,
     .dedicated_thread = true,
     .func = do_GetFSRMQuota,
     .rd = NULL,
     .thread = NULL,
     .cleanup = do_GetFSRMQuota_cleanup,
     .interrupt = do_GetFSRMQuota_interrupt},
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

static void windows_plugin_thread_worker(void *ptr __maybe_unused)
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

    // Thread-owned resources (e.g. COM/WMI) must be released on the thread that created them.
    if (mod->dedicated_thread && mod->cleanup)
        mod->cleanup();
}

void win_plugin_main(void *ptr)
{
    worker_register("WIN");

    nrpc_serving_started();
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

        if (pm->enabled && (pm->dedicated_thread || unlikely(update_every != pm->update_every))) {
            char tag_name[ND_THREAD_TAG_MAX];
            snprintfz(tag_name, ND_THREAD_TAG_MAX - 1, "WIN_PLUGIN[%d]", i);
            pm->thread = nd_thread_create(tag_name, NETDATA_THREAD_OPTION_DEFAULT, windows_plugin_thread_worker, pm);
            if (!pm->thread && pm->dedicated_thread) {
                // Running it inline could block every other module, so leave it disabled.
                nd_log(NDLS_COLLECTORS, NDLP_ERR, "%s: cannot create its thread; module disabled", pm->name);
                pm->enabled = 0;
            }
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

    // Abort blocking calls in every module thread first, so their joins below do not wait on each other.
    for (i = 0; win_modules[i].name; i++) {
        struct proc_module *pm = &win_modules[i];
        if (pm->thread && pm->interrupt)
            pm->interrupt();
    }

    // Join threads
    for (i = 0; win_modules[i].name; i++) {
        struct proc_module *pm = &win_modules[i];
        bool cleaned_by_thread = pm->thread && pm->dedicated_thread;

        if (pm->thread) {
            nd_thread_join(pm->thread);
            pm->thread = NULL;
        }

        if (pm->cleanup && !cleaned_by_thread)
            pm->cleanup();
    }
}
