// SPDX-License-Identifier: GPL-3.0-or-later

#include "database/rrd.h"

typedef struct rrdvar {
    NETDATA_DOUBLE value;
} RRDVAR;

// ----------------------------------------------------------------------------
// RRDVAR management

inline STRING *rrdvar_name_to_string(const char *name) {
    char *variable = strdupz(name);
    rrdvar_fix_name(variable);
    STRING *name_string = string_strdupz(variable);
    freez(variable);
    return name_string;
}

static bool rrdvar_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value, void *data __maybe_unused) {
    RRDVAR *rv = old_value;
    RRDVAR *nrv = new_value;

    rv->value = nrv->value;
    return false;
}

DICTIONARY *rrdvariables_create(void) {
    DICTIONARY *dict = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
                                                  &dictionary_stats_category_rrdhealth, sizeof(RRDVAR));
    dictionary_register_conflict_callback(dict, rrdvar_conflict_callback, NULL);
    return dict;
}

void rrdvariables_destroy(DICTIONARY *dict) {
    dictionary_destroy(dict);
}

static inline const RRDVAR_ACQUIRED *rrdvar_get_and_acquire(DICTIONARY *dict, STRING *name) {
    return (const RRDVAR_ACQUIRED *)dictionary_get_and_acquire_item_advanced(dict, string2str(name), (ssize_t)string_strlen(name));
}

inline const RRDVAR_ACQUIRED *rrdvar_add_and_acquire(DICTIONARY *dict, STRING *name, NETDATA_DOUBLE value) {
    if(unlikely(!dict || !name)) return NULL;
    RRDVAR tmp = {
        .value = value,
    };
    return (const RRDVAR_ACQUIRED *)dictionary_set_and_acquire_item_advanced(
        dict, string2str(name), (ssize_t)string_strlen(name),
        &tmp, sizeof(tmp), NULL);
}

void rrdvar_delete_all(DICTIONARY *dict) {
    dictionary_flush(dict);
}

void rrdvar_release(DICTIONARY *dict, const RRDVAR_ACQUIRED *rva) {
    if(unlikely(!dict || !rva)) return;  // when health is not enabled
    dictionary_acquired_item_release(dict, (const DICTIONARY_ITEM *)rva);
}

// ----------------------------------------------------------------------------
// CUSTOM HOST VARIABLES

inline int rrdvar_walkthrough_read(DICTIONARY *dict, int (*callback)(const DICTIONARY_ITEM *item, void *rrdvar, void *data), void *data) {
    if(unlikely(!dict)) return 0;  // when health is not enabled
    return dictionary_walkthrough_read(dict, callback, data);
}

const RRDVAR_ACQUIRED *rrdvar_host_variable_add_and_acquire(RRDHOST *host, const char *name) {
    if(unlikely(!host->rrdvars)) return NULL; // when health is not enabled

    STRING *name_string = rrdvar_name_to_string(name);
    const RRDVAR_ACQUIRED *rva = rrdvar_add_and_acquire(host->rrdvars, name_string, NAN);

    string_freez(name_string);
    return rva;
}

void rrdvar_host_variable_set(RRDHOST *host, const RRDVAR_ACQUIRED *rva, NETDATA_DOUBLE value) {
    if(unlikely(!host->rrdvars || !rva)) return; // when health is not enabled

    RRDVAR *rv = dictionary_acquired_item_value((const DICTIONARY_ITEM *)rva);
    if(rv->value != value) {
        rv->value = value;

        // if the host is streaming, send this variable upstream immediately
        stream_sender_send_this_host_variable_now(host, rva);
    }
}

// ----------------------------------------------------------------------------
// CUSTOM CHART VARIABLES

const RRDVAR_ACQUIRED *rrdvar_chart_variable_add_and_acquire(RRDSET *st, const char *name) {
    if(unlikely(!st || !st->rrdvars)) return NULL;

    STRING *name_string = rrdvar_name_to_string(name);
    const RRDVAR_ACQUIRED *rs = rrdvar_add_and_acquire(st->rrdvars, name_string, NAN);
    string_freez(name_string);
    return rs;
}

void rrdvar_chart_variable_set(RRDSET *st, const RRDVAR_ACQUIRED *rva, NETDATA_DOUBLE value) {
    if(unlikely(!st || !st->rrdvars || !rva)) return;

    RRDVAR *rv = dictionary_acquired_item_value((const DICTIONARY_ITEM *)rva);
    if(rv->value != value) {
        rv->value = value;
        rrdset_flag_set(st, RRDSET_FLAG_UPSTREAM_SEND_VARIABLES);
    }
}

// ----------------------------------------------------------------------------
// RRDVAR lookup

NETDATA_DOUBLE rrdvar2number(const RRDVAR_ACQUIRED *rva) {
    if(unlikely(!rva)) return NAN;
    RRDVAR *rv = dictionary_acquired_item_value((const DICTIONARY_ITEM *)rva);
    return rv->value;
}

static inline bool rrdvar_get_value(DICTIONARY *dict, STRING *variable, NETDATA_DOUBLE *result) {
    bool found = false;

    const RRDVAR_ACQUIRED *rva = rrdvar_get_and_acquire(dict, variable);
    if(rva) {
        *result = rrdvar2number(rva);
        found = true;
        dictionary_acquired_item_release(dict, (const DICTIONARY_ITEM *)rva);
    }

    return found;
}

bool rrdvar_get_custom_host_variable_value(RRDHOST *host, STRING *variable, NETDATA_DOUBLE *result) {
    return rrdvar_get_value(host->rrdvars, variable, result);
}

bool rrdvar_get_custom_chart_variable_value(RRDSET *st, STRING *variable, NETDATA_DOUBLE *result) {
    return rrdvar_get_value(st->rrdvars, variable, result);
}

// ----------------------------------------------------------------------------
// RRDVAR to JSON

void rrdvar_to_json_members(DICTIONARY *dict, BUFFER *wb) {
    RRDVAR *rv;
    dfe_start_read(dict, rv) {
        buffer_json_member_add_double(wb, rv_dfe.name, rv->value);
    }
    dfe_done(rv);
}

void health_api_v1_chart_custom_variables2json(RRDSET *st, BUFFER *buf) {
    rrdvar_to_json_members(st->rrdvars, buf);
}

void health_api_v1_chart_variables2json(RRDSET *st, BUFFER *wb) {

    // FIXME this list is incomplete
    // alerts can also access {context}.{dimension} from the entire host database

    RRDHOST *host = st->rrdhost;

    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);

    buffer_json_member_add_string(wb, "chart", rrdset_id(st));
    buffer_json_member_add_string(wb, "chart_name", rrdset_name(st));
    buffer_json_member_add_string(wb, "chart_context", rrdset_context(st));
    buffer_json_member_add_string(wb, "family", rrdset_family(st));
    buffer_json_member_add_string(wb, "host", rrdhost_hostname(host));

    time_t now = now_realtime_sec();

    buffer_json_member_add_object(wb, "current_alert_values");
    {
        buffer_json_member_add_double(wb, "this", NAN);
        buffer_json_member_add_double(wb, "after", (NETDATA_DOUBLE)now - 1);
        buffer_json_member_add_double(wb, "before", (NETDATA_DOUBLE)now);
        buffer_json_member_add_double(wb, "now", (NETDATA_DOUBLE)now);
        buffer_json_member_add_double(wb, "status", (NETDATA_DOUBLE)RRDCALC_STATUS_REMOVED);
        buffer_json_member_add_double(wb, "REMOVED", (NETDATA_DOUBLE)RRDCALC_STATUS_REMOVED);
        buffer_json_member_add_double(wb, "UNDEFINED", (NETDATA_DOUBLE)RRDCALC_STATUS_UNDEFINED);
        buffer_json_member_add_double(wb, "UNINITIALIZED", (NETDATA_DOUBLE)RRDCALC_STATUS_UNINITIALIZED);
        buffer_json_member_add_double(wb, "CLEAR", (NETDATA_DOUBLE)RRDCALC_STATUS_CLEAR);
        buffer_json_member_add_double(wb, "WARNING", (NETDATA_DOUBLE)RRDCALC_STATUS_WARNING);
        buffer_json_member_add_double(wb, "CRITICAL", (NETDATA_DOUBLE)RRDCALC_STATUS_CRITICAL);
        buffer_json_member_add_double(wb, "green", NAN);
        buffer_json_member_add_double(wb, "red", NAN);
    }
    buffer_json_object_close(wb);

    buffer_json_member_add_object(wb, "dimensions_last_stored_values");
    {
        RRDDIM *rd;
        dfe_start_read(st->rrddim_root_index, rd) {
            buffer_json_member_add_double(wb, string2str(rd->id), rd->collector.last_stored_value);
            if(rd->name != rd->id)
                buffer_json_member_add_double(wb, string2str(rd->name), rd->collector.last_stored_value);
        }
        dfe_done(rd);
    }
    buffer_json_object_close(wb);

    buffer_json_member_add_object(wb, "dimensions_last_collected_values");
    {
        char name[RRD_ID_LENGTH_MAX + 1 + 100];
        RRDDIM *rd;
        dfe_start_read(st->rrddim_root_index, rd) {
            snprintfz(name, sizeof(name), "%s_raw", string2str(rd->id));
            buffer_json_member_add_int64(wb, name, rd->collector.last_collected_value);
            if(rd->name != rd->id) {
                snprintfz(name, sizeof(name), "%s_raw", string2str(rd->name));
                buffer_json_member_add_int64(wb, name, rd->collector.last_collected_value);
            }
        }
        dfe_done(rd);
    }
    buffer_json_object_close(wb);

    buffer_json_member_add_object(wb, "dimensions_last_collected_time");
    {
        char name[RRD_ID_LENGTH_MAX + 1 + 100];
        RRDDIM *rd;
        dfe_start_read(st->rrddim_root_index, rd) {
            snprintfz(name, sizeof(name), "%s_last_collected_t", string2str(rd->id));
            buffer_json_member_add_int64(wb, name, rd->collector.last_collected_time.tv_sec);
            if(rd->name != rd->id) {
                snprintfz(name, sizeof(name), "%s_last_collected_t", string2str(rd->name));
                buffer_json_member_add_int64(wb, name, rd->collector.last_collected_time.tv_sec);
            }
        }
        dfe_done(rd);
    }
    buffer_json_object_close(wb);

    buffer_json_member_add_object(wb, "chart_variables");
    {
        buffer_json_member_add_int64(wb, "update_every", st->update_every);
        buffer_json_member_add_uint64(wb, "last_collected_t", st->last_collected_time.tv_sec);

        rrdvar_to_json_members(st->rrdvars, wb);
    }
    buffer_json_object_close(wb);

    buffer_json_member_add_object(wb, "host_variables");
    {
        rrdvar_to_json_members(st->rrdhost->rrdvars, wb);
    }
    buffer_json_object_close(wb);

    buffer_json_member_add_object(wb, "alerts");
    {
        struct scored {
            bool existing;
            STRING *chart;
            STRING *context;
            NETDATA_DOUBLE value;
            size_t score;
        } tmp, *z;
        DICTIONARY *dict = dictionary_create(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE);

        RRDCALC *rc;
        dfe_start_read(st->rrdhost->rrdcalc_root_index, rc) {
            tmp = (struct scored) {
                .existing = false,
                .chart = string_dup(rc->rrdset->id),
                .context = string_dup(rc->rrdset->context),
                .value = rc->value,
                .score = rrdlabels_common_count(rc->rrdset->rrdlabels, st->rrdlabels),
            };
            z = dictionary_set(dict, string2str(rc->config.name), &tmp, sizeof(tmp));

            if(z->existing) {
                if(tmp.score > z->score)
                    SWAP(*z, tmp);
                z->existing = true;
                string_freez(tmp.chart);
                string_freez(tmp.context);
            }
            else
                z->existing = true;
        }
        dfe_done(rc);

        dfe_start_read(dict, z) {
            buffer_json_member_add_object(wb, z_dfe.name);
            {
                buffer_json_member_add_double(wb, "value", z->value);
                buffer_json_member_add_string(wb, "instance", string2str(z->chart));
                buffer_json_member_add_string(wb, "context", string2str(z->context));
                buffer_json_member_add_uint64(wb, "score", z->score);
            }
            buffer_json_object_close(wb);

            string_freez(z->chart);
            string_freez(z->context);
        }
        dfe_done(z);

        dictionary_destroy(dict);
    }
    buffer_json_object_close(wb);

    buffer_json_finalize(wb);
}

// ----------------------------------------------------------------------------
// RRDVAR private members examination

const char *rrdvar_name(const RRDVAR_ACQUIRED *rva) {
    return dictionary_acquired_item_name((const DICTIONARY_ITEM *)rva);
}

void rrdvar_print_to_streaming_custom_chart_variables(RRDSET *st, BUFFER *wb) {
    rrdset_flag_clear(st, RRDSET_FLAG_UPSTREAM_SEND_VARIABLES);

    // send the chart local custom variables
    RRDVAR *rv;
    dfe_start_read(st->rrdvars, rv) {
        buffer_sprintf(wb
                       , "VARIABLE CHART %s = " NETDATA_DOUBLE_FORMAT "\n"
                       , rv_dfe.name, rv->value
        );
    }
    dfe_done(rv);
}
