// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

#include <windows.h>
#include <winspool.h>

// Documented PRINTER_INFO_2 and JOB_INFO_1 flags; defined here in case an older SDK header lacks them.
#ifndef PRINTER_STATUS_SERVER_UNKNOWN
#define PRINTER_STATUS_SERVER_UNKNOWN 0x00800000
#endif
#ifndef PRINTER_STATUS_POWER_SAVE
#define PRINTER_STATUS_POWER_SAVE 0x01000000
#endif
#ifndef PRINTER_STATUS_SERVER_OFFLINE
#define PRINTER_STATUS_SERVER_OFFLINE 0x02000000
#endif
#ifndef PRINTER_STATUS_DRIVER_UPDATE_NEEDED
#define PRINTER_STATUS_DRIVER_UPDATE_NEEDED 0x04000000
#endif
#ifndef JOB_STATUS_BLOCKED_DEVQ
#define JOB_STATUS_BLOCKED_DEVQ 0x00000200
#endif
#ifndef JOB_STATUS_USER_INTERVENTION
#define JOB_STATUS_USER_INTERVENTION 0x00000400
#endif

#define CONFIG_SECTION_GET_PRINTERS "plugin:windows:GetPrinters"
#define PRINTERS_DEFAULT_MAX 100
#define SPOOLER_ENUM_MAX_ATTEMPTS 5

// Printer states follow the Win32_Printer.PrinterStatus mapping used by windows_exporter (CIMWin32 parity).
enum printer_state {
    PRINTER_STATE_OTHER = 0,
    PRINTER_STATE_UNKNOWN,
    PRINTER_STATE_IDLE,
    PRINTER_STATE_PRINTING,
    PRINTER_STATE_WARMUP,

    PRINTER_STATE_MAX,
};

static const char *printer_state_names[PRINTER_STATE_MAX] = {
    [PRINTER_STATE_OTHER] = "other",
    [PRINTER_STATE_UNKNOWN] = "unknown",
    [PRINTER_STATE_IDLE] = "idle",
    [PRINTER_STATE_PRINTING] = "printing",
    [PRINTER_STATE_WARMUP] = "warmup",
};

// Job states follow the Win32_PrintJob.Status mapping used by windows_exporter, except: a job with no status flags is
// waiting its turn, so it is queued; a job the driver cannot print (BLOCKED_DEVQ) or one waiting for user action
// (USER_INTERVENTION) is degraded. CIMWin32 reports all three as unknown.
enum printer_job_state {
    PRINTER_JOB_STATE_ERROR = 0,
    PRINTER_JOB_STATE_DEGRADED,
    PRINTER_JOB_STATE_OK,
    PRINTER_JOB_STATE_QUEUED,
    PRINTER_JOB_STATE_UNKNOWN,

    PRINTER_JOB_STATE_MAX,
};

static const char *printer_job_state_names[PRINTER_JOB_STATE_MAX] = {
    [PRINTER_JOB_STATE_ERROR] = "error",
    [PRINTER_JOB_STATE_DEGRADED] = "degraded",
    [PRINTER_JOB_STATE_OK] = "ok",
    [PRINTER_JOB_STATE_QUEUED] = "queued",
    [PRINTER_JOB_STATE_UNKNOWN] = "unknown",
};

// Problem flags charted individually, so each one can be alerted on even when several are set together.
static const struct {
    DWORD flag;
    const char *name;
} printer_problems[] = {
    {PRINTER_STATUS_PAUSED, "paused"},
    {PRINTER_STATUS_ERROR, "error"},
    {PRINTER_STATUS_PENDING_DELETION, "pending_deletion"},
    {PRINTER_STATUS_PAPER_JAM, "paper_jam"},
    {PRINTER_STATUS_PAPER_OUT, "paper_out"},
    {PRINTER_STATUS_MANUAL_FEED, "manual_feed"},
    {PRINTER_STATUS_PAPER_PROBLEM, "paper_problem"},
    {PRINTER_STATUS_OFFLINE, "offline"},
    {PRINTER_STATUS_OUTPUT_BIN_FULL, "output_bin_full"},
    {PRINTER_STATUS_TONER_LOW, "toner_low"},
    {PRINTER_STATUS_NO_TONER, "no_toner"},
    {PRINTER_STATUS_PAGE_PUNT, "page_punt"},
    {PRINTER_STATUS_USER_INTERVENTION, "user_intervention"},
    {PRINTER_STATUS_OUT_OF_MEMORY, "out_of_memory"},
    {PRINTER_STATUS_DOOR_OPEN, "door_open"},
    {PRINTER_STATUS_SERVER_OFFLINE, "server_offline"},
    {PRINTER_STATUS_DRIVER_UPDATE_NEEDED, "driver_update_needed"},
};

#define PRINTER_PROBLEMS_COUNT (sizeof(printer_problems) / sizeof(printer_problems[0]))

struct printer {
    usec_t last_seen;
    DWORD last_error;

    RRDSET *st_jobs;
    RRDDIM *rd_jobs;

    RRDSET *st_status;
    RRDDIM *rd_status[PRINTER_STATE_MAX];

    RRDSET *st_problems;
    RRDDIM *rd_problems[PRINTER_PROBLEMS_COUNT];

    RRDSET *st_job_status;
    RRDDIM *rd_job_status[PRINTER_JOB_STATE_MAX];
};

typedef struct {
    char *name;
    const PRINTER_INFO_4W *info;
} PRINTER_SAMPLE;

typedef struct {
    BYTE *data;
    DWORD size;
} SPOOLER_BUFFER;

static DICTIONARY *printers = NULL;
static SIMPLE_PATTERN *printers_to_monitor = NULL;
static long long max_printers = PRINTERS_DEFAULT_MAX;
static SPOOLER_BUFFER printers_buffer = {0};
static SPOOLER_BUFFER printer_info_buffer = {0};
static SPOOLER_BUFFER jobs_buffer = {0};
static DWORD last_enum_error = ERROR_SUCCESS;

static void printer_obsolete_charts(struct printer *p)
{
    if (p->st_jobs)
        rrdset_is_obsolete___safe_from_collector_thread(p->st_jobs);
    if (p->st_status)
        rrdset_is_obsolete___safe_from_collector_thread(p->st_status);
    if (p->st_problems)
        rrdset_is_obsolete___safe_from_collector_thread(p->st_problems);
    if (p->st_job_status)
        rrdset_is_obsolete___safe_from_collector_thread(p->st_job_status);

    p->st_jobs = p->st_status = p->st_problems = p->st_job_status = NULL;
    p->rd_jobs = NULL;
    memset(p->rd_status, 0, sizeof(p->rd_status));
    memset(p->rd_problems, 0, sizeof(p->rd_problems));
    memset(p->rd_job_status, 0, sizeof(p->rd_job_status));
}

static void printer_delete_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    printer_obsolete_charts(value);
}

static void initialize(void)
{
    char *patterns = windows_plugin_normalize_patterns(
        inicfg_get(&netdata_config, CONFIG_SECTION_GET_PRINTERS, "printers to monitor", "*"));
    printers_to_monitor = simple_pattern_create(patterns, ",|", SIMPLE_PATTERN_EXACT, false);
    freez(patterns);

    max_printers =
        inicfg_get_number(&netdata_config, CONFIG_SECTION_GET_PRINTERS, "max monitored printers", PRINTERS_DEFAULT_MAX);
    if (max_printers < 0)
        max_printers = PRINTERS_DEFAULT_MAX;

    printers = dictionary_create_advanced(
        DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct printer));
    dictionary_register_delete_callback(printers, printer_delete_cb, NULL);
}

typedef BOOL (*spooler_enum_fn)(void *ctx, BYTE *data, DWORD size, DWORD *needed, DWORD *returned);

// Spooler enumerations report the needed size; it can grow between calls when printers or jobs are added.
// Returns ERROR_SUCCESS with *returned validated against the buffer, or the Win32 error.
static DWORD spooler_enum(SPOOLER_BUFFER *buffer, spooler_enum_fn fn, void *ctx, size_t item_size, DWORD *returned)
{
    for (int attempt = 0; attempt < SPOOLER_ENUM_MAX_ATTEMPTS; attempt++) {
        DWORD needed = 0;
        *returned = 0;

        if (fn(ctx, buffer->data, buffer->size, &needed, returned)) {
            if ((uint64_t)*returned * item_size > buffer->size) {
                *returned = 0;
                return ERROR_INVALID_DATA;
            }
            return ERROR_SUCCESS;
        }

        DWORD err = GetLastError();
        if (err != ERROR_INSUFFICIENT_BUFFER || needed <= buffer->size)
            return err;

        buffer->data = reallocz(buffer->data, needed);
        buffer->size = needed;
    }

    return ERROR_INSUFFICIENT_BUFFER;
}

static BOOL spooler_enum_printers_call(void *ctx __maybe_unused, BYTE *data, DWORD size, DWORD *needed, DWORD *returned)
{
    // Level 4 names come from the registry/local cache; level 2 would open every remote connection, so an
    // unreachable connection excluded by the selector could still block collection.
    return EnumPrintersW(PRINTER_ENUM_LOCAL | PRINTER_ENUM_CONNECTIONS, NULL, 4, data, size, needed, returned);
}

static BOOL spooler_get_printer_call(void *ctx, BYTE *data, DWORD size, DWORD *needed, DWORD *returned)
{
    if (!GetPrinterW((HANDLE)ctx, 2, data, size, needed))
        return FALSE;

    *returned = 1;
    return TRUE;
}

static BOOL spooler_enum_jobs_call(void *ctx, BYTE *data, DWORD size, DWORD *needed, DWORD *returned)
{
    return EnumJobsW((HANDLE)ctx, 0, 0xFFFFFFFF, 1, data, size, needed, returned);
}

// Flag groups follow the Win32_Printer.PrinterStatus mapping used by windows_exporter, except: SERVER_OFFLINE is a
// problem (an unreachable print server makes a connection unusable) and SERVER_UNKNOWN is unknown (Microsoft defines
// it as "the printer status is unknown"); CIMWin32 reports both as idle.
#define PRINTER_FLAGS_OTHER                                                                                            \
    (PRINTER_STATUS_PAUSED | PRINTER_STATUS_ERROR | PRINTER_STATUS_PENDING_DELETION | PRINTER_STATUS_PAPER_JAM |       \
     PRINTER_STATUS_PAPER_OUT | PRINTER_STATUS_MANUAL_FEED | PRINTER_STATUS_PAPER_PROBLEM | PRINTER_STATUS_OFFLINE |   \
     PRINTER_STATUS_OUTPUT_BIN_FULL | PRINTER_STATUS_TONER_LOW | PRINTER_STATUS_NO_TONER | PRINTER_STATUS_PAGE_PUNT |  \
     PRINTER_STATUS_USER_INTERVENTION | PRINTER_STATUS_OUT_OF_MEMORY | PRINTER_STATUS_DOOR_OPEN |                     \
     PRINTER_STATUS_SERVER_OFFLINE)
#define PRINTER_FLAGS_UNKNOWN (PRINTER_STATUS_NOT_AVAILABLE | PRINTER_STATUS_SERVER_UNKNOWN)
#define PRINTER_FLAGS_PRINTING                                                                                         \
    (PRINTER_STATUS_IO_ACTIVE | PRINTER_STATUS_BUSY | PRINTER_STATUS_PRINTING | PRINTER_STATUS_PROCESSING)
#define PRINTER_FLAGS_WARMUP (PRINTER_STATUS_INITIALIZING | PRINTER_STATUS_WARMING_UP)
#define PRINTER_FLAGS_IDLE (PRINTER_STATUS_WAITING | PRINTER_STATUS_POWER_SAVE)

static enum printer_state printer_state_from_flags(DWORD status, bool *decided)
{
    *decided = true;

    // CIMWin32 only matches a status that is exactly one flag, so combined flags (offline and paper out together)
    // would fall back to the jobs and read as idle. Test bits instead, problems first, so a single flag maps as in
    // windows_exporter and a combination reports its most severe state.
    if (status & PRINTER_FLAGS_OTHER)
        return PRINTER_STATE_OTHER;
    if (status & PRINTER_FLAGS_UNKNOWN)
        return PRINTER_STATE_UNKNOWN;
    if (status & PRINTER_FLAGS_PRINTING)
        return PRINTER_STATE_PRINTING;
    if (status & PRINTER_FLAGS_WARMUP)
        return PRINTER_STATE_WARMUP;
    if (status & PRINTER_FLAGS_IDLE)
        return PRINTER_STATE_IDLE;

    // No mapped flag (zero, or only DRIVER_UPDATE_NEEDED, which does not stop printing): the first queued job decides.
    *decided = false;
    return PRINTER_STATE_UNKNOWN;
}

static enum printer_state printer_state_from_first_job(const JOB_INFO_1W *jobs, DWORD jobs_count)
{
    if (!jobs_count)
        return PRINTER_STATE_IDLE;

    const JOB_INFO_1W *job = &jobs[0];

    if (job->pStatus && *job->pStatus)
        return PRINTER_STATE_UNKNOWN;

    if (job->Status & (JOB_STATUS_PAUSED | JOB_STATUS_ERROR | JOB_STATUS_DELETING | JOB_STATUS_OFFLINE |
                       JOB_STATUS_PAPEROUT | JOB_STATUS_PRINTED))
        return PRINTER_STATE_OTHER;

    if (job->Status & (JOB_STATUS_SPOOLING | JOB_STATUS_PRINTING))
        return PRINTER_STATE_PRINTING;

    // A first job with no flags is waiting its turn; with no printer flags either, nothing prints or fails, so the
    // printer is idle (CIMWin32 reports unknown). A growing backlog stays visible in the queued jobs chart.
    if (!job->Status)
        return PRINTER_STATE_IDLE;

    return PRINTER_STATE_UNKNOWN;
}

static enum printer_job_state printer_job_state(DWORD status)
{
    if (status & JOB_STATUS_ERROR)
        return PRINTER_JOB_STATE_ERROR;

    if (status & (JOB_STATUS_PAUSED | JOB_STATUS_OFFLINE | JOB_STATUS_PAPEROUT | JOB_STATUS_BLOCKED_DEVQ |
                  JOB_STATUS_USER_INTERVENTION))
        return PRINTER_JOB_STATE_DEGRADED;

    if (status & (JOB_STATUS_DELETING | JOB_STATUS_SPOOLING | JOB_STATUS_PRINTING | JOB_STATUS_PRINTED))
        return PRINTER_JOB_STATE_OK;

    if (!status)
        return PRINTER_JOB_STATE_QUEUED;

    return PRINTER_JOB_STATE_UNKNOWN;
}

static RRDSET *printer_chart_create(
    const char *name,
    const char *suffix,
    size_t suffix_length,
    const char *context,
    const char *title,
    const char *units,
    int priority,
    int update_every,
    RRDSET_TYPE type)
{
    char id[RRD_ID_LENGTH_MAX + 1];
    windows_plugin_instance_chart_id(id, sizeof(id), sizeof("printer") - 1, "printer_", name, suffix, suffix_length);

    RRDSET *st = rrdset_create_localhost(
        "printer",
        id,
        NULL,
        "printers",
        context,
        title,
        units,
        PLUGIN_WINDOWS_NAME,
        "GetPrinters",
        priority,
        update_every,
        type);

    rrdlabels_add(st->rrdlabels, "printer", name, RRDLABEL_SRC_AUTO);
    return st;
}

static void printer_chart_jobs(struct printer *p, const char *name, DWORD queued, int update_every)
{
    if (unlikely(!p->st_jobs)) {
        p->st_jobs = printer_chart_create(
            name,
            "jobs",
            sizeof("jobs") - 1,
            "windows.printer_jobs",
            "Printer queued jobs",
            "jobs",
            PRIO_PRINTER_JOBS,
            update_every,
            RRDSET_TYPE_LINE);
        p->rd_jobs = rrddim_add(p->st_jobs, "queued", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrdset_isnot_obsolete___safe_from_collector_thread(p->st_jobs);
    rrddim_set_by_pointer(p->st_jobs, p->rd_jobs, (collected_number)queued);
    rrdset_done(p->st_jobs);
}

static void printer_chart_status(struct printer *p, const char *name, enum printer_state state, int update_every)
{
    if (unlikely(!p->st_status)) {
        p->st_status = printer_chart_create(
            name,
            "status",
            sizeof("status") - 1,
            "windows.printer_status",
            "Printer status",
            "status",
            PRIO_PRINTER_STATUS,
            update_every,
            RRDSET_TYPE_LINE);
        for (int i = 0; i < PRINTER_STATE_MAX; i++)
            p->rd_status[i] = rrddim_add(p->st_status, printer_state_names[i], NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrdset_isnot_obsolete___safe_from_collector_thread(p->st_status);
    for (int i = 0; i < PRINTER_STATE_MAX; i++)
        rrddim_set_by_pointer(p->st_status, p->rd_status[i], (collected_number)(i == (int)state));
    rrdset_done(p->st_status);
}

static void printer_chart_problems(struct printer *p, const char *name, DWORD status, int update_every)
{
    if (unlikely(!p->st_problems)) {
        p->st_problems = printer_chart_create(
            name,
            "problems",
            sizeof("problems") - 1,
            "windows.printer_problems",
            "Printer problems",
            "status",
            PRIO_PRINTER_PROBLEMS,
            update_every,
            RRDSET_TYPE_LINE);
        for (size_t i = 0; i < PRINTER_PROBLEMS_COUNT; i++)
            p->rd_problems[i] = rrddim_add(p->st_problems, printer_problems[i].name, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrdset_isnot_obsolete___safe_from_collector_thread(p->st_problems);
    for (size_t i = 0; i < PRINTER_PROBLEMS_COUNT; i++)
        rrddim_set_by_pointer(
            p->st_problems, p->rd_problems[i], (collected_number)((status & printer_problems[i].flag) != 0));
    rrdset_done(p->st_problems);
}

static void printer_chart_job_status(
    struct printer *p,
    const char *name,
    const collected_number counts[PRINTER_JOB_STATE_MAX],
    int update_every)
{
    if (unlikely(!p->st_job_status)) {
        p->st_job_status = printer_chart_create(
            name,
            "job_status",
            sizeof("job_status") - 1,
            "windows.printer_job_status",
            "Printer jobs by status",
            "jobs",
            PRIO_PRINTER_JOB_STATUS,
            update_every,
            RRDSET_TYPE_STACKED);
        for (int i = 0; i < PRINTER_JOB_STATE_MAX; i++)
            p->rd_job_status[i] =
                rrddim_add(p->st_job_status, printer_job_state_names[i], NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrdset_isnot_obsolete___safe_from_collector_thread(p->st_job_status);
    for (int i = 0; i < PRINTER_JOB_STATE_MAX; i++)
        rrddim_set_by_pointer(p->st_job_status, p->rd_job_status[i], counts[i]);
    rrdset_done(p->st_job_status);
}

static void printer_log_error(struct printer *p, const char *name, const char *call, DWORD err)
{
    // Log once per error transition to keep a persistently failing printer from flooding the log.
    if (p->last_error == err)
        return;

    p->last_error = err;
    nd_log(NDLS_COLLECTORS, NDLP_ERR, "GetPrinters: %s failed for printer '%s' (error %lu)", call, name, err);
}

// Reads the job list of an open printer. Returns false when it could not be read; status charts then get a gap.
static bool printer_read_jobs(
    struct printer *p,
    const char *name,
    HANDLE handle,
    DWORD queued,
    const JOB_INFO_1W **jobs,
    DWORD *jobs_count)
{
    *jobs = NULL;
    *jobs_count = 0;

    if (!queued)
        return true;

    DWORD err = spooler_enum(&jobs_buffer, spooler_enum_jobs_call, handle, sizeof(JOB_INFO_1W), jobs_count);
    if (err != ERROR_SUCCESS) {
        printer_log_error(p, name, "EnumJobsW", err);
        *jobs_count = 0;
        return false;
    }

    *jobs = (const JOB_INFO_1W *)jobs_buffer.data;
    return true;
}

static void printer_collect(struct printer *p, const char *name, const PRINTER_INFO_4W *entry, int update_every)
{
    // Only selected printers are opened; for a printer connection this can contact the print server.
    HANDLE handle = NULL;
    if (!OpenPrinterW(entry->pPrinterName, &handle, NULL)) {
        DWORD err = GetLastError();
        // A printer deleted after enumeration is treated as missing for this cycle, not as idle.
        if (err != ERROR_INVALID_PRINTER_NAME)
            printer_log_error(p, name, "OpenPrinterW", err);
        return;
    }

    DWORD returned = 0;
    DWORD err = spooler_enum(
        &printer_info_buffer, spooler_get_printer_call, handle, sizeof(PRINTER_INFO_2W), &returned);
    if (err != ERROR_SUCCESS) {
        printer_log_error(p, name, "GetPrinterW", err);
        ClosePrinter(handle);
        return;
    }

    const PRINTER_INFO_2W *info = (const PRINTER_INFO_2W *)printer_info_buffer.data;

    // The queue length and problem flags are valid even if the job list below cannot be read.
    printer_chart_jobs(p, name, info->cJobs, update_every);
    printer_chart_problems(p, name, info->Status, update_every);

    const JOB_INFO_1W *jobs;
    DWORD jobs_count;
    bool jobs_ok = printer_read_jobs(p, name, handle, info->cJobs, &jobs, &jobs_count);
    ClosePrinter(handle);

    if (!jobs_ok)
        return;
    p->last_error = ERROR_SUCCESS;

    bool decided;
    enum printer_state state = printer_state_from_flags(info->Status, &decided);
    if (!decided)
        state = printer_state_from_first_job(jobs, jobs_count);
    printer_chart_status(p, name, state, update_every);

    collected_number counts[PRINTER_JOB_STATE_MAX] = {0};
    for (DWORD i = 0; i < jobs_count; i++)
        counts[printer_job_state(jobs[i].Status)]++;
    printer_chart_job_status(p, name, counts, update_every);
}

static int printer_sample_compare(const void *a, const void *b)
{
    const PRINTER_SAMPLE *sa = a, *sb = b;
    int cmp = strcasecmp(sa->name, sb->name);
    return cmp ? cmp : strcmp(sa->name, sb->name);
}

static void printers_evict_unseen(usec_t now_ut)
{
    struct printer *p;
    dfe_start_write(printers, p)
    {
        if (p->last_seen != now_ut)
            dictionary_del(printers, p_dfe.name);
    }
    dfe_done(p);
    dictionary_garbage_collect(printers);
}

int do_GetPrinters(int update_every, usec_t dt __maybe_unused)
{
    static bool initialized = false;

    if (unlikely(!initialized)) {
        initialize();
        initialized = true;
    }

    DWORD count = 0;
    DWORD err = spooler_enum(&printers_buffer, spooler_enum_printers_call, NULL, sizeof(PRINTER_INFO_4W), &count);
    if (err != ERROR_SUCCESS) {
        // Keep existing charts: a failed enumeration (for example a stopped Spooler) proves nothing about printers.
        if (err != last_enum_error)
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "GetPrinters: EnumPrintersW failed (error %lu)", err);
        last_enum_error = err;
        return 0;
    }
    last_enum_error = ERROR_SUCCESS;

    const PRINTER_INFO_4W *infos = (const PRINTER_INFO_4W *)printers_buffer.data;
    PRINTER_SAMPLE *samples = callocz(count ? count : 1, sizeof(*samples));
    size_t samples_count = 0;
    bool complete = true;

    for (DWORD i = 0; i < count; i++) {
        char *name = utf16_to_utf8_strdupz(infos[i].pPrinterName, NULL);
        if (!name || !*name) {
            // An undecodable name cannot be matched to existing state, so this snapshot must not retire charts.
            freez(name);
            complete = false;
            continue;
        }

        // Unselected printers are not added, so existing state for them is evicted below.
        if (!simple_pattern_matches(printers_to_monitor, name)) {
            freez(name);
            continue;
        }

        samples[samples_count++] = (PRINTER_SAMPLE){.name = name, .info = &infos[i]};
    }

    // Order matters only when the cap drops printers; sorting then keeps the selection independent of
    // enumeration order.
    if (max_printers > 0 && samples_count > (size_t)max_printers)
        qsort(samples, samples_count, sizeof(*samples), printer_sample_compare);

    usec_t now_ut = now_monotonic_usec();
    for (size_t i = 0; i < samples_count; i++) {
        PRINTER_SAMPLE *s = &samples[i];

        if (i < (size_t)max_printers) {
            struct printer *p = dictionary_set(printers, s->name, NULL, sizeof(*p));
            if (p) {
                p->last_seen = now_ut;
                printer_collect(p, s->name, s->info, update_every);
            }
        }

        freez(s->name);
    }
    freez(samples);

    if (complete)
        printers_evict_unseen(now_ut);

    return 0;
}

void do_GetPrinters_cleanup(void)
{
    if (printers) {
        dictionary_destroy(printers);
        printers = NULL;
    }

    simple_pattern_free(printers_to_monitor);
    printers_to_monitor = NULL;

    freez(printers_buffer.data);
    printers_buffer = (SPOOLER_BUFFER){0};
    freez(printer_info_buffer.data);
    printer_info_buffer = (SPOOLER_BUFFER){0};
    freez(jobs_buffer.data);
    jobs_buffer = (SPOOLER_BUFFER){0};
}
