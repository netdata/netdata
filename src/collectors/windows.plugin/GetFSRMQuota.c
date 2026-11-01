// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "libnetdata/os/windows-wmi/windows-wmi.h"
#include "libnetdata/sanitizers/chart_id_and_name.h"
#include "nrpc/nrpc.h"

// File Server Resource Manager quotas, read from WMI (MSFT_FSRMQuota).
// This module runs on a dedicated thread: WMI connects cannot be bounded below ~2 minutes.

#define FSRM_WMI_NAMESPACE L"ROOT\\Microsoft\\Windows\\FSRM"
#define FSRM_WMI_QUERY                                                                                                 \
    L"SELECT Path, Template, Description, Usage, Size, PeakUsage, Disabled, SoftLimit, MatchesTemplate "              \
    L"FROM MSFT_FSRMQuota"

#define FSRM_NEXT_TIMEOUT_MS 5000
// Whole-enumeration budget; generous because the module has its own thread and large hosts can have many quotas.
#define FSRM_ENUMERATION_MIN_SECONDS 60
#define FSRM_RETRY_MAX_UT (600 * USEC_PER_SEC)

// Large enough for a 32767 UTF-16 unit extended-length path in UTF-8, so long paths are never truncated.
#define FSRM_SCRATCH_SIZE (32768 * 3)

#define FUNCTION_FSRM_QUOTAS_HELP "Shows Windows File Server Resource Manager quotas with usage, limits and state."

struct fsrm_values {
    uint64_t usage, size, peak;
    bool disabled, soft_limit, matches_template;
    bool has_usage, has_size, has_peak;
    bool has_disabled, has_soft_limit, has_matches_template;
};

struct fsrm_row {
    char *path;
    char *template_name;
    char *description;
    struct fsrm_values values;
};

struct fsrm_quota {
    usec_t last_collected;

    // Protected by fsrm_mutex: read by the Function from another thread.
    char *template_name;
    char *description;
    struct fsrm_values values;

    RRDSET *st_space;
    RRDDIM *rd_used;
    RRDDIM *rd_peak;
    RRDDIM *rd_limit;

    RRDSET *st_state;
    RRDDIM *rd_disabled;
    RRDDIM *rd_soft_limit;
    RRDDIM *rd_matches_template;
};

static ND_WMI fsrm_wmi = {0};
static DICTIONARY *fsrm_quotas = NULL;
static netdata_mutex_t fsrm_mutex;
static usec_t fsrm_last_success_ut = 0;
static int fsrm_update_every = 10;

static RRDSET *st_quotas = NULL;
static RRDDIM *rd_quotas = NULL;

static void fsrm_chart_obsolete(RRDSET **st)
{
    if (*st)
        rrdset_is_obsolete___safe_from_collector_thread(*st);
    *st = NULL;
}

static void fsrm_quota_delete_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct fsrm_quota *q = value;
    fsrm_chart_obsolete(&q->st_space);
    fsrm_chart_obsolete(&q->st_state);
    freez(q->template_name);
    freez(q->description);
    q->template_name = NULL;
    q->description = NULL;
}

static void fsrm_set_string(char **dst, const char *src)
{
    if (*dst && !strcmp(*dst, src))
        return;
    freez(*dst);
    *dst = strdupz(src);
}

static void fsrm_quota_chart_id(char *dst, size_t size, const char *path, const char *suffix)
{
    // Sanitization is not injective, so a raw-path hash keeps IDs unique; the visible part is truncated to fit.
    char sanitized[RRD_ID_LENGTH_MAX + 1];
    snprintfz(sanitized, sizeof(sanitized), "%s", path);
    netdata_fix_chart_id(sanitized);

    size_t id_limit = RRD_ID_LENGTH_MAX - strlen("fsrm.");
    if (size > id_limit)
        size = id_limit;
    size_t reserved = strlen("quota__") + 16 + 1 + strlen(suffix) + 1;
    size_t visible = size > reserved ? size - reserved : 0;
    XXH64_hash_t hash = XXH3_64bits(path, strlen(path));
    snprintfz(dst, size, "quota_%.*s_%016llx_%s", (int)visible, sanitized, (unsigned long long)hash, suffix);
}

static void fsrm_update_labels(RRDSET *st, const char *path, const char *template_name)
{
    rrdlabels_add(st->rrdlabels, "path", path, RRDLABEL_SRC_AUTO);
    if (rrdlabels_add_changed(st->rrdlabels, "template", template_name, RRDLABEL_SRC_AUTO))
        rrdset_flag_set(st, RRDSET_FLAG_METADATA_UPDATE);
}

static void fsrm_chart_quota(struct fsrm_quota *q, const char *path, int update_every)
{
    const struct fsrm_values *v = &q->values;
    char id[RRD_ID_LENGTH_MAX + 1];

    if (v->has_usage || v->has_peak || v->has_size) {
        if (unlikely(!q->st_space)) {
            fsrm_quota_chart_id(id, sizeof(id), path, "space");
            q->st_space = rrdset_create_localhost(
                "fsrm",
                id,
                NULL,
                "quotas",
                "fsrm.quota_space",
                "FSRM quota space",
                "bytes",
                PLUGIN_WINDOWS_NAME,
                "GetFSRMQuota",
                PRIO_FSRM_QUOTA_SPACE,
                update_every,
                RRDSET_TYPE_LINE);

            q->rd_used = rrddim_add(q->st_space, "used", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            q->rd_peak = rrddim_add(q->st_space, "peak", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            q->rd_limit = rrddim_add(q->st_space, "limit", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }

        fsrm_update_labels(q->st_space, path, q->template_name);
        if (v->has_usage)
            rrddim_set_by_pointer(q->st_space, q->rd_used, (collected_number)v->usage);
        if (v->has_peak)
            rrddim_set_by_pointer(q->st_space, q->rd_peak, (collected_number)v->peak);
        if (v->has_size)
            rrddim_set_by_pointer(q->st_space, q->rd_limit, (collected_number)v->size);
        rrdset_done(q->st_space);
    }

    if (v->has_disabled || v->has_soft_limit || v->has_matches_template) {
        if (unlikely(!q->st_state)) {
            fsrm_quota_chart_id(id, sizeof(id), path, "state");
            q->st_state = rrdset_create_localhost(
                "fsrm",
                id,
                NULL,
                "quotas",
                "fsrm.quota_state",
                "FSRM quota state",
                "status",
                PLUGIN_WINDOWS_NAME,
                "GetFSRMQuota",
                PRIO_FSRM_QUOTA_STATE,
                update_every,
                RRDSET_TYPE_LINE);

            q->rd_disabled = rrddim_add(q->st_state, "disabled", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            q->rd_soft_limit = rrddim_add(q->st_state, "soft_limit", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            q->rd_matches_template = rrddim_add(q->st_state, "matches_template", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }

        fsrm_update_labels(q->st_state, path, q->template_name);
        if (v->has_disabled)
            rrddim_set_by_pointer(q->st_state, q->rd_disabled, v->disabled ? 1 : 0);
        if (v->has_soft_limit)
            rrddim_set_by_pointer(q->st_state, q->rd_soft_limit, v->soft_limit ? 1 : 0);
        if (v->has_matches_template)
            rrddim_set_by_pointer(q->st_state, q->rd_matches_template, v->matches_template ? 1 : 0);
        rrdset_done(q->st_state);
    }
}

static void fsrm_chart_count(size_t count, int update_every)
{
    if (unlikely(!st_quotas)) {
        st_quotas = rrdset_create_localhost(
            "fsrm",
            "quotas",
            NULL,
            "quotas",
            "fsrm.quotas",
            "FSRM quotas",
            "quotas",
            PLUGIN_WINDOWS_NAME,
            "GetFSRMQuota",
            PRIO_FSRM_QUOTAS,
            update_every,
            RRDSET_TYPE_LINE);

        rd_quotas = rrddim_add(st_quotas, "quotas", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(st_quotas, rd_quotas, (collected_number)count);
    rrdset_done(st_quotas);
}

static char *fsrm_read_string(IWbemClassObject *obj, const wchar_t *prop)
{
    // Only the module thread reads rows, so one static scratch buffer is enough.
    static char scratch[FSRM_SCRATCH_SIZE];
    if (!nd_wmi_get_string_property(obj, prop, scratch, sizeof(scratch)))
        scratch[0] = '\0';
    return strdupz(scratch);
}

static void fsrm_rows_free(struct fsrm_row *rows, size_t count)
{
    for (size_t i = 0; i < count; i++) {
        freez(rows[i].path);
        freez(rows[i].template_name);
        freez(rows[i].description);
    }
    freez(rows);
}

static bool fsrm_read_row(IWbemClassObject *obj, struct fsrm_row *r)
{
    memset(r, 0, sizeof(*r));

    // Path is the quota identity; a row without it cannot be charted.
    r->path = fsrm_read_string(obj, L"Path");
    if (!r->path[0]) {
        freez(r->path);
        r->path = NULL;
        return false;
    }

    // Template and Description are optional; NULL means empty.
    r->template_name = fsrm_read_string(obj, L"Template");
    r->description = fsrm_read_string(obj, L"Description");

    struct fsrm_values *v = &r->values;
    v->has_usage = nd_wmi_get_uint64_property(obj, L"Usage", &v->usage);
    v->has_size = nd_wmi_get_uint64_property(obj, L"Size", &v->size);
    v->has_peak = nd_wmi_get_uint64_property(obj, L"PeakUsage", &v->peak);
    v->has_disabled = nd_wmi_get_bool_property(obj, L"Disabled", &v->disabled);
    v->has_soft_limit = nd_wmi_get_bool_property(obj, L"SoftLimit", &v->soft_limit);
    v->has_matches_template = nd_wmi_get_bool_property(obj, L"MatchesTemplate", &v->matches_template);
    return true;
}

static const char *fsrm_hresult_reason(HRESULT hr)
{
    switch ((unsigned long)hr) {
        case (unsigned long)WBEM_E_INVALID_NAMESPACE:
        case (unsigned long)WBEM_E_INVALID_CLASS:
            return "FSRM WMI provider not found (is the File Server Resource Manager role installed?)";
        case (unsigned long)WBEM_E_ACCESS_DENIED:
        case (unsigned long)E_ACCESSDENIED:
            return "access denied to the FSRM WMI namespace";
        case (unsigned long)WBEM_S_TIMEDOUT:
            return "timed out";
        default:
            return "WMI error";
    }
}

// Enumerate all quotas. Returns false unless the enumeration completed; rows are owned by the caller.
static bool fsrm_query(struct fsrm_row **rows_out, size_t *count_out, int update_every)
{
    *rows_out = NULL;
    *count_out = 0;

    const char *failed_step = NULL;
    HRESULT hr = nd_wmi_connect(&fsrm_wmi, FSRM_WMI_NAMESPACE, &failed_step);
    if (FAILED(hr)) {
        nd_log_limit_static_thread_var(erl, 3600, 0);
        nd_log_limit(
            &erl,
            NDLS_COLLECTORS,
            NDLP_ERR,
            "GetFSRMQuota: %s: %s. Error code = 0x%X",
            failed_step ? failed_step : "WMI connect failed",
            fsrm_hresult_reason(hr),
            (unsigned)hr);
        return false;
    }

    IEnumWbemClassObject *enumerator = NULL;
    hr = nd_wmi_exec_query(fsrm_wmi.pSvc, FSRM_WMI_QUERY, &enumerator);
    if (FAILED(hr)) {
        nd_log_limit_static_thread_var(erl, 3600, 0);
        nd_log_limit(
            &erl,
            NDLS_COLLECTORS,
            NDLP_ERR,
            "GetFSRMQuota: MSFT_FSRMQuota query failed: %s. Error code = 0x%X",
            fsrm_hresult_reason(hr),
            (unsigned)hr);
        // The provider or connection may be gone; reconnect on the next attempt.
        nd_wmi_disconnect(&fsrm_wmi);
        return false;
    }

    // Bound the whole enumeration as well as each Next() call.
    int budget_s = MAX(update_every, FSRM_ENUMERATION_MIN_SECONDS);
    usec_t deadline_ut = now_monotonic_usec() + (usec_t)budget_s * USEC_PER_SEC;
    size_t size = 16, count = 0;
    struct fsrm_row *rows = mallocz(size * sizeof(*rows));
    bool complete = false;

    while (service_running(SERVICE_COLLECTORS)) {
        IWbemClassObject *obj = NULL;
        ULONG returned = 0;
        hr = enumerator->lpVtbl->Next(enumerator, FSRM_NEXT_TIMEOUT_MS, 1, &obj, &returned);

        if (returned == 1 && obj) {
            if (count == size) {
                size *= 2;
                rows = reallocz(rows, size * sizeof(*rows));
            }
            bool ok = fsrm_read_row(obj, &rows[count]);
            obj->lpVtbl->Release(obj);
            if (!ok) {
                nd_log_limit_static_thread_var(erl, 3600, 0);
                nd_log_limit(
                    &erl, NDLS_COLLECTORS, NDLP_WARNING, "GetFSRMQuota: a quota row has no Path; skipping this cycle");
                break;
            }
            count++;
        }
        else if (hr == WBEM_S_FALSE) {
            complete = true;
            break;
        }
        else {
            nd_log_limit_static_thread_var(erl, 3600, 0);
            nd_log_limit(
                &erl,
                NDLS_COLLECTORS,
                NDLP_ERR,
                "GetFSRMQuota: reading MSFT_FSRMQuota rows failed: %s. Error code = 0x%X",
                fsrm_hresult_reason(hr),
                (unsigned)hr);
            break;
        }

        if (now_monotonic_usec() > deadline_ut) {
            nd_log_limit_static_thread_var(erl, 3600, 0);
            nd_log_limit(
                &erl,
                NDLS_COLLECTORS,
                NDLP_ERR,
                "GetFSRMQuota: enumeration exceeded %d seconds after %zu quotas; skipping this cycle",
                budget_s,
                count);
            break;
        }
    }

    enumerator->lpVtbl->Release(enumerator);

    if (!complete) {
        if (FAILED(hr))
            nd_wmi_disconnect(&fsrm_wmi);
        fsrm_rows_free(rows, count);
        return false;
    }

    *rows_out = rows;
    *count_out = count;
    return true;
}

static void fsrm_publish(struct fsrm_row *rows, size_t count, int update_every)
{
    usec_t now_ut = now_monotonic_usec();

    netdata_mutex_lock(&fsrm_mutex);
    for (size_t i = 0; i < count; i++) {
        struct fsrm_row *r = &rows[i];
        struct fsrm_quota *q = dictionary_set(fsrm_quotas, r->path, NULL, sizeof(*q));
        q->last_collected = now_ut;
        q->values = r->values;
        fsrm_set_string(&q->template_name, r->template_name);
        fsrm_set_string(&q->description, r->description);
        fsrm_chart_quota(q, r->path, update_every);
    }

    // Only a complete enumeration proves which quotas were removed.
    struct fsrm_quota *q;
    dfe_start_write(fsrm_quotas, q)
    {
        if (q->last_collected != now_ut)
            dictionary_del(fsrm_quotas, q_dfe.name);
    }
    dfe_done(q);
    dictionary_garbage_collect(fsrm_quotas);

    fsrm_last_success_ut = now_ut;
    netdata_mutex_unlock(&fsrm_mutex);

    fsrm_chart_count(count, update_every);
}

// ----------------------------------------------------------------------------
// Function

static void fsrm_function_add_column(
    BUFFER *wb,
    size_t *id,
    const char *name,
    const char *description,
    RRDF_FIELD_TYPE type,
    RRDF_FIELD_VISUAL visual,
    RRDF_FIELD_TRANSFORM transform,
    const char *units,
    RRDF_FIELD_SUMMARY summary,
    RRDF_FIELD_FILTER filter,
    RRDF_FIELD_OPTIONS options)
{
    buffer_rrdf_table_add_field(
        wb,
        (*id)++,
        name,
        description,
        type,
        visual,
        transform,
        0,
        units,
        NAN,
        RRDF_FIELD_SORT_ASCENDING,
        NULL,
        summary,
        filter,
        options,
        NULL);
}

static void fsrm_function_add_uint(BUFFER *wb, bool has, uint64_t value)
{
    if (has)
        buffer_json_add_array_item_uint64(wb, value);
    else
        buffer_json_add_array_item_string(wb, NULL);
}

static void fsrm_function_add_bool(BUFFER *wb, bool has, bool value)
{
    buffer_json_add_array_item_string(wb, has ? (value ? "yes" : "no") : NULL);
}

static int fsrm_function_quotas(
    BUFFER *wb,
    const char *function __maybe_unused,
    BUFFER *payload __maybe_unused,
    const char *source __maybe_unused)
{
    netdata_mutex_lock(&fsrm_mutex);

    usec_t now_ut = now_monotonic_usec();
    usec_t ttl_ut = (usec_t)fsrm_update_every * 3 * USEC_PER_SEC;
    if (!fsrm_quotas || !fsrm_last_success_ut || now_ut - fsrm_last_success_ut > ttl_ut) {
        netdata_mutex_unlock(&fsrm_mutex);
        return nrpc_call_error(wb, "no recent FSRM quota data", HTTP_RESP_SERVICE_UNAVAILABLE);
    }

    buffer_flush(wb);
    wb->content_type = CT_APPLICATION_JSON;
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);
    buffer_json_member_add_string(wb, "hostname", rrdhost_hostname(localhost));
    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_string(wb, "type", "table");
    buffer_json_member_add_time_t(wb, "update_every", fsrm_update_every);
    buffer_json_member_add_boolean(wb, "has_history", false);
    buffer_json_member_add_string(wb, "help", FUNCTION_FSRM_QUOTAS_HELP);
    buffer_json_member_add_array(wb, "data");

    struct fsrm_quota *q;
    dfe_start_read(fsrm_quotas, q)
    {
        const struct fsrm_values *v = &q->values;
        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_string(wb, q_dfe.name);
        buffer_json_add_array_item_string(wb, q->template_name);
        buffer_json_add_array_item_string(wb, q->description);
        fsrm_function_add_uint(wb, v->has_usage, v->usage);
        fsrm_function_add_uint(wb, v->has_size, v->size);
        fsrm_function_add_uint(wb, v->has_peak, v->peak);
        if (v->has_usage && v->has_size && v->size)
            buffer_json_add_array_item_double(wb, (NETDATA_DOUBLE)v->usage * 100.0 / (NETDATA_DOUBLE)v->size);
        else
            buffer_json_add_array_item_double(wb, NAN);
        fsrm_function_add_bool(wb, v->has_soft_limit, v->soft_limit);
        fsrm_function_add_bool(wb, v->has_disabled, v->disabled);
        fsrm_function_add_bool(wb, v->has_matches_template, v->matches_template);
        buffer_json_array_close(wb);
    }
    dfe_done(q);

    netdata_mutex_unlock(&fsrm_mutex);

    buffer_json_array_close(wb);
    buffer_json_member_add_object(wb, "columns");
    size_t id = 0;
    fsrm_function_add_column(
        wb, &id, "Path", "Quota path", RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
        NULL, RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
        RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_UNIQUE_KEY | RRDF_FIELD_OPTS_STICKY);
    fsrm_function_add_column(
        wb, &id, "Template", "Quota template", RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE,
        RRDF_FIELD_TRANSFORM_NONE, NULL, RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
        RRDF_FIELD_OPTS_VISIBLE);
    fsrm_function_add_column(
        wb, &id, "Description", "Quota description", RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE,
        RRDF_FIELD_TRANSFORM_NONE, NULL, RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE,
        RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_FULL_WIDTH | RRDF_FIELD_OPTS_WRAP);
    fsrm_function_add_column(
        wb, &id, "Used", "Disk space charged to the quota", RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE,
        RRDF_FIELD_TRANSFORM_NUMBER, "bytes", RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE, RRDF_FIELD_OPTS_VISIBLE);
    fsrm_function_add_column(
        wb, &id, "Limit", "Quota size", RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
        "bytes", RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE, RRDF_FIELD_OPTS_VISIBLE);
    fsrm_function_add_column(
        wb, &id, "Peak", "Highest usage charged to the quota", RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE,
        RRDF_FIELD_TRANSFORM_NUMBER, "bytes", RRDF_FIELD_SUMMARY_MAX, RRDF_FIELD_FILTER_NONE, RRDF_FIELD_OPTS_NONE);
    buffer_rrdf_table_add_field(
        wb,
        id++,
        "Used%",
        "Usage as a percentage of the quota size",
        RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
        RRDF_FIELD_VISUAL_BAR,
        RRDF_FIELD_TRANSFORM_NUMBER,
        2,
        "%",
        100.0,
        RRDF_FIELD_SORT_DESCENDING,
        NULL,
        RRDF_FIELD_SUMMARY_MAX,
        RRDF_FIELD_FILTER_RANGE,
        RRDF_FIELD_OPTS_VISIBLE,
        NULL);
    fsrm_function_add_column(
        wb, &id, "SoftLimit", "Soft limit (yes) or hard limit (no)", RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE,
        RRDF_FIELD_TRANSFORM_NONE, NULL, RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
        RRDF_FIELD_OPTS_VISIBLE);
    fsrm_function_add_column(
        wb, &id, "Disabled", "Quota is disabled", RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE,
        RRDF_FIELD_TRANSFORM_NONE, NULL, RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
        RRDF_FIELD_OPTS_VISIBLE);
    fsrm_function_add_column(
        wb, &id, "MatchesTemplate", "Quota properties match its template", RRDF_FIELD_TYPE_STRING,
        RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE, NULL, RRDF_FIELD_SUMMARY_COUNT,
        RRDF_FIELD_FILTER_MULTISELECT, RRDF_FIELD_OPTS_NONE);
    buffer_json_object_close(wb);
    buffer_json_member_add_string(wb, "default_sort_column", "Used%");

    buffer_json_member_add_time_t(wb, "expires", now_realtime_sec() + fsrm_update_every);
    buffer_json_finalize(wb);
    return HTTP_RESP_OK;
}

// ----------------------------------------------------------------------------

static void initialize(int update_every)
{
    netdata_mutex_init(&fsrm_mutex);
    fsrm_update_every = update_every;
    fsrm_quotas =
        dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct fsrm_quota));
    dictionary_register_delete_callback(fsrm_quotas, fsrm_quota_delete_cb, NULL);
}

static bool fsrm_initialized = false;

int do_GetFSRMQuota(int update_every, usec_t dt __maybe_unused)
{
    static bool function_registered = false;
    static usec_t retry_after_ut = 0;
    static usec_t retry_delay_ut = 0;

    if (unlikely(!fsrm_initialized)) {
        initialize(update_every);
        fsrm_initialized = true;
    }

    usec_t now_ut = now_monotonic_usec();
    if (now_ut < retry_after_ut)
        return 0;

    struct fsrm_row *rows = NULL;
    size_t count = 0;
    if (!fsrm_query(&rows, &count, update_every)) {
        // Leave a gap rather than publish zeros; back off exponentially so a missing role stays cheap.
        usec_t base_ut = (usec_t)update_every * USEC_PER_SEC;
        retry_delay_ut = retry_delay_ut ? MIN(retry_delay_ut * 2, FSRM_RETRY_MAX_UT) : base_ut;
        retry_after_ut = now_monotonic_usec() + retry_delay_ut;
        return 0;
    }

    retry_delay_ut = 0;
    retry_after_ut = 0;
    fsrm_publish(rows, count, update_every);
    fsrm_rows_free(rows, count);

    if (!function_registered) {
        nrpc_method_register_builtin(&(struct nrpc_builtin_desc){
            .owner = rrdhost_nrpc_owner(localhost),
            .name = "fsrm-quotas",
            .help = FUNCTION_FSRM_QUOTAS_HELP,
            .tags = "top",
            .timeout_s = 10,
            .priority = NRPC_PRIORITY_DEFAULT,
            .version = NRPC_VERSION_DEFAULT,
            .access = HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE,
            .handler = fsrm_function_quotas,
        });
        function_registered = true;
    }

    return 0;
}

void do_GetFSRMQuota_cleanup(void)
{
    if (!fsrm_initialized)
        return;

    nrpc_method_unregister(rrdhost_nrpc_owner(localhost), "fsrm-quotas", NRPC_SOURCE_DAEMON);

    netdata_mutex_lock(&fsrm_mutex);
    dictionary_destroy(fsrm_quotas);
    fsrm_quotas = NULL;
    netdata_mutex_unlock(&fsrm_mutex);

    fsrm_chart_obsolete(&st_quotas);
    rd_quotas = NULL;

    // Runs on the module's own thread (dedicated_thread), which owns the COM apartment.
    nd_wmi_disconnect(&fsrm_wmi);
}
