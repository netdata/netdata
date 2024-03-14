// SPDX-License-Identifier: GPL-3.0-or-later

#include "health.h"
#include "health_internals.h"

struct variable_lookup_score {
    RRDSET *st;
    const char *source;
    NETDATA_DOUBLE value;
    size_t score;
};

struct variable_lookup_job {
    RRDCALC *rc;
    RRDHOST *host;
    STRING *variable;
    STRING *dim;
    const char *dimension;
    size_t dimension_length;
    enum {
        DIM_SELECT_NORMAL,
        DIM_SELECT_RAW,
        DIM_SELECT_LAST_COLLECTED,
    } dimension_selection;

    struct {
        size_t size;
        size_t used;
        struct variable_lookup_score *array;
    } result;

    struct {
        RRDSET *last_rrdset;
        size_t last_score;
    } score;
};

static void variable_lookup_add_result_with_score(struct variable_lookup_job *vbd, NETDATA_DOUBLE n, RRDSET *st, const char *source __maybe_unused) {
    if(vbd->score.last_rrdset != st) {
        vbd->score.last_rrdset = st;
        vbd->score.last_score = rrdlabels_common_count(vbd->rc->rrdset->rrdlabels, st->rrdlabels);
    }

    if(vbd->result.used >= vbd->result.size) {
        if(!vbd->result.size)
            vbd->result.size = 1;

        vbd->result.size *= 2;
        vbd->result.array = reallocz(vbd->result.array, sizeof(struct variable_lookup_score) * vbd->result.size);
    }

    vbd->result.array[vbd->result.used++] = (struct variable_lookup_score) {
        .value = n,
        .score = vbd->score.last_score,
        .st = st,
        .source = source,
    };
}

static bool variable_lookup_in_chart(struct variable_lookup_job *vbd, RRDSET *st, bool stop_on_match) {
    bool found = false;
    const DICTIONARY_ITEM *item = NULL;
    RRDDIM *rd = NULL;
    dfe_start_read(st->rrddim_root_index, rd) {
        if(rd->id == vbd->dim || rd->name == vbd->dim) {
            item = dictionary_acquired_item_dup(st->rrddim_root_index, rd_dfe.item);
            break;
        }
    }
    dfe_done(rd);

    if (item) {
        switch (vbd->dimension_selection) {
            case DIM_SELECT_NORMAL:
                variable_lookup_add_result_with_score(vbd, (NETDATA_DOUBLE)rd->collector.last_stored_value, st, "last stored value of dimension");
                break;
            case DIM_SELECT_RAW:
                variable_lookup_add_result_with_score(vbd, (NETDATA_DOUBLE)rd->collector.last_collected_value, st, "last collected value of dimension");
                break;
            case DIM_SELECT_LAST_COLLECTED:
                variable_lookup_add_result_with_score(vbd, (NETDATA_DOUBLE)rd->collector.last_collected_time.tv_sec, st, "last collected time of dimension");
                break;
        }

        dictionary_acquired_item_release(st->rrddim_root_index, item);
        found = true;
    }
    if(found && stop_on_match) goto cleanup;

    // chart variable
    {
        NETDATA_DOUBLE n;
        if(rrdvar_get_custom_chart_variable_value(st, vbd->variable, &n)) {
            variable_lookup_add_result_with_score(vbd, n, st, "chart variable");
            found = true;
        }
    }
    if(found && stop_on_match) goto cleanup;

cleanup:
    return found;
}

static int foreach_instance_in_context_cb(RRDSET *st, void *data) {
    struct variable_lookup_job *vbd = data;
    return variable_lookup_in_chart(vbd, st, false) ? 1 : 0;
}

static bool variable_lookup_context(struct variable_lookup_job *vbd, const char *chart_or_context, const char *dim_id_or_name) {
    struct variable_lookup_job vbd_back = *vbd;

    vbd->dimension = dim_id_or_name;
    vbd->dim = string_strdupz(vbd->dimension);
    vbd->dimension_length = string_strlen(vbd->dim);
    // vbd->dimension_selection = DIM_SELECT_NORMAL;

    bool found = false;

    // lookup chart in host

    RRDSET_ACQUIRED *rsa = rrdset_find_and_acquire(vbd->host, chart_or_context);
    if(rsa) {
        if(variable_lookup_in_chart(vbd, rrdset_acquired_to_rrdset(rsa), false))
            found = true;
        rrdset_acquired_release(rsa);
    }

    // lookup context in contexts, then foreach chart

    if(rrdcontext_foreach_instance_with_rrdset_in_context(vbd->host, chart_or_context, foreach_instance_in_context_cb, vbd) > 0)
        found = true;

    string_freez(vbd->dim);

    vbd->dimension = vbd_back.dimension;
    vbd->dim = vbd_back.dim;
    vbd->dimension_length = vbd_back.dimension_length;
    // vbd->dimension_selection = vbd_back.dimension_selection;

    return found;
}

bool alert_variable_from_running_alerts(struct variable_lookup_job *vbd) {
    bool found = false;
    RRDCALC *rc;
    foreach_rrdcalc_in_rrdhost_read(vbd->host, rc) {
        if(rc->config.name == vbd->variable) {
            variable_lookup_add_result_with_score(vbd, (NETDATA_DOUBLE)rc->value, rc->rrdset, "alarm value");
            found = true;
        }
    }
    foreach_rrdcalc_in_rrdhost_done(rc);
    return found;
}

bool alert_variable_lookup_internal(STRING *variable, void *data, NETDATA_DOUBLE *result, BUFFER *wb) {
    static STRING *this_string = NULL,
                  *now_string = NULL,
                  *after_string = NULL,
                  *before_string = NULL,
                  *status_string = NULL,
                  *removed_string = NULL,
                  *uninitialized_string = NULL,
                  *undefined_string = NULL,
                  *clear_string = NULL,
                  *warning_string = NULL,
                  *critical_string = NULL,
                  *last_collected_t_string = NULL,
                  *update_every_string = NULL;


    struct variable_lookup_job vbd = { 0 };

//    const char *v_name = string2str(variable);
//    bool trace_this = false;
//    if(strcmp(v_name, "btrfs_allocated") == 0)
//        trace_this = true;

    bool found = false;

    const char *source = NULL;
    RRDSET *source_st = NULL;

    RRDCALC *rc = data;
    RRDSET *st = rc->rrdset;

    if(!st)
        return false;

    if(unlikely(!last_collected_t_string)) {
        this_string = string_strdupz("this");
        now_string = string_strdupz("now");
        after_string = string_strdupz("after");
        before_string = string_strdupz("before");
        status_string = string_strdupz("status");
        removed_string = string_strdupz("REMOVED");
        undefined_string = string_strdupz("UNDEFINED");
        uninitialized_string = string_strdupz("UNINITIALIZED");
        clear_string = string_strdupz("CLEAR");
        warning_string = string_strdupz("WARNING");
        critical_string = string_strdupz("CRITICAL");
        last_collected_t_string = string_strdupz("last_collected_t");
        update_every_string = string_strdupz("update_every");
    }

    if(unlikely(variable == this_string)) {
        *result = (NETDATA_DOUBLE)rc->value;
        source = "current alert value";
        source_st = st;
        found = true;
        goto log;
    }

    if(unlikely(variable == after_string)) {
        *result = (NETDATA_DOUBLE)rc->db_after;
        source = "current alert query start time";
        source_st = st;
        found = true;
        goto log;
    }

    if(unlikely(variable == before_string)) {
        *result = (NETDATA_DOUBLE)rc->db_before;
        source = "current alert query end time";
        source_st = st;
        found = true;
        goto log;
    }

    if(unlikely(variable == now_string)) {
        *result = (NETDATA_DOUBLE)now_realtime_sec();
        source = "current wall-time clock timestamp";
        source_st = st;
        found = true;
        goto log;
    }

    if(unlikely(variable == status_string)) {
        *result = (NETDATA_DOUBLE)rc->status;
        source = "current alert status";
        source_st = st;
        found = true;
        goto log;
    }

    if(unlikely(variable == removed_string)) {
        *result = (NETDATA_DOUBLE)RRDCALC_STATUS_REMOVED;
        source = "removed status constant";
        source_st = st;
        found = true;
        goto log;
    }

    if(unlikely(variable == uninitialized_string)) {
        *result = (NETDATA_DOUBLE)RRDCALC_STATUS_UNINITIALIZED;
        source = "uninitialized status constant";
        source_st = st;
        found = true;
        goto log;
    }

    if(unlikely(variable == undefined_string)) {
        *result = (NETDATA_DOUBLE)RRDCALC_STATUS_UNDEFINED;
        source = "undefined status constant";
        source_st = st;
        found = true;
        goto log;
    }

    if(unlikely(variable == clear_string)) {
        *result = (NETDATA_DOUBLE)RRDCALC_STATUS_CLEAR;
        source = "clear status constant";
        source_st = st;
        found = true;
        goto log;
    }

    if(unlikely(variable == warning_string)) {
        *result = (NETDATA_DOUBLE)RRDCALC_STATUS_WARNING;
        source = "warning status constant";
        source_st = st;
        found = true;
        goto log;
    }

    if(unlikely(variable == critical_string)) {
        *result = (NETDATA_DOUBLE)RRDCALC_STATUS_CRITICAL;
        source = "critical status constant";
        source_st = st;
        found = true;
        goto log;
    }

    if(unlikely(variable == last_collected_t_string)) {
        *result = (NETDATA_DOUBLE)st->last_collected_time.tv_sec;
        source = "current instance last_collected_t";
        source_st = st;
        found = true;
        goto log;
    }

    if(unlikely(variable == update_every_string)) {
        *result = (NETDATA_DOUBLE)st->update_every;
        source = "current instance update_every";
        source_st = st;
        found = true;
        goto log;
    }

    // find the dimension id/name

    vbd = (struct variable_lookup_job){
        .rc = rc,
        .host = st->rrdhost,
        .variable = variable,
        .dimension = string2str(variable),
        .dimension_length = string_strlen(variable),
        .dimension_selection = DIM_SELECT_NORMAL,
        .dim = string_dup(variable),
        .result = { 0 },
    };
    if (strendswith_lengths(vbd.dimension, vbd.dimension_length, "_raw", 4)) {
        vbd.dimension_length -= 4;
        vbd.dimension_selection = DIM_SELECT_RAW;
        vbd.dim = string_strndupz(vbd.dimension, vbd.dimension_length);
    } else if (strendswith_lengths(vbd.dimension, vbd.dimension_length, "_last_collected_t", 17)) {
        vbd.dimension_length -= 17;
        vbd.dimension_selection = DIM_SELECT_LAST_COLLECTED;
        vbd.dim = string_strndupz(vbd.dimension, vbd.dimension_length);
    }

    if(variable_lookup_in_chart(&vbd, st, true)) {
        found = true;
        goto find_best_scored;
    }

    // host variables
    {
        NETDATA_DOUBLE n;
        found = rrdvar_get_custom_host_variable_value(vbd.host,  vbd.variable, &n);
        if(found) {
            variable_lookup_add_result_with_score(&vbd, n, st, "host variable");
            goto find_best_scored;
        }
    }

    // alert names
    if(alert_variable_from_running_alerts(&vbd)) {
        found = true;
        goto find_best_scored;
    }

    // find the components of the variable
    {
        char id[string_strlen(vbd.dim) + 1];
        memcpy(id, string2str(vbd.dim), string_strlen(vbd.dim));
        id[string_strlen(vbd.dim)] = '\0';

        char *dot = strrchr(id, '.');
        while(dot) {
            *dot = '\0';

            if(strchr(id, '.') == NULL) break;

            if(variable_lookup_context(&vbd, id, dot + 1))
                found = true;

            char *dot2 = strrchr(id, '.');
            *dot = '.';
            dot = dot2;
        }
    }

find_best_scored:
    if(found && vbd.result.array) {
        struct variable_lookup_score *best = &vbd.result.array[0];
        for (size_t i = 1; i < vbd.result.used; i++)
            if (vbd.result.array[i].score > best->score)
                best = &vbd.result.array[i];

        source = best->source;
        source_st = best->st;
        *result = best->value;
        freez(vbd.result.array);
    }
    else {
        found = false;
        *result = NAN;
    }

log:
#ifdef NETDATA_LOG_HEALTH_VARIABLES_LOOKUP
    if(found) {
        nd_log(NDLS_DAEMON, NDLP_INFO,
               "HEALTH_VARIABLE_LOOKUP: variable '%s' of alert '%s' of chart '%s', context '%s', host '%s' "
               "resolved with %s of chart '%s' and context '%s'",
               string2str(variable),
               string2str(rc->config.name),
               string2str(rc->rrdset->id),
               string2str(rc->rrdset->context),
               string2str(rc->rrdset->rrdhost->hostname),
               source,
               string2str(source_st->id),
               string2str(source_st->context)
               );
    }
    else {
        nd_log(NDLS_DAEMON, NDLP_INFO,
               "HEALTH_VARIABLE_LOOKUP: variable '%s' of alert '%s' of chart '%s', context '%s', host '%s' "
               "could not be resolved",
               string2str(variable),
               string2str(rc->config.name),
               string2str(rc->rrdset->id),
               string2str(rc->rrdset->context),
               string2str(rc->rrdset->rrdhost->hostname)
        );
    }
#endif

    if(unlikely(wb)) {
        buffer_json_member_add_string(wb, "variable", string2str(variable));
        buffer_json_member_add_string(wb, "instance", string2str(st->id));
        buffer_json_member_add_string(wb, "context", string2str(st->context));
        buffer_json_member_add_boolean(wb, "found", found);

        if (found) {
            buffer_json_member_add_double(wb, "value", *result);
            buffer_json_member_add_object(wb, "source");
            {
                buffer_json_member_add_string(wb, "description", source);
                buffer_json_member_add_string(wb, "instance", string2str(source_st->id));
                buffer_json_member_add_string(wb, "context", string2str(source_st->context));
                buffer_json_member_add_uint64(wb, "candidates", vbd.result.used ? vbd.result.used : 1);
            }
            buffer_json_object_close(wb); // source
        }
    }

    string_freez(vbd.dim);

    return found;
}

bool alert_variable_lookup(STRING *variable, void *data, NETDATA_DOUBLE *result) {
    return alert_variable_lookup_internal(variable, data, result, NULL);
}

int alert_variable_lookup_trace(RRDHOST *host __maybe_unused, RRDSET *st, const char *variable, BUFFER *wb) {
    int code = HTTP_RESP_INTERNAL_SERVER_ERROR;

    buffer_flush(wb);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);

    STRING *v = string_strdupz(variable);
    RRDCALC rc = {
        .rrdset = st,
    };

    NETDATA_DOUBLE n;
    alert_variable_lookup_internal(v, &rc, &n, wb);

    string_freez(v);

    buffer_json_finalize(wb);
    return code;
}
