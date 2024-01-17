// SPDX-License-Identifier: GPL-3.0-or-later

#include "health.h"
#include "health_internals.h"
//
//struct var {
//    void *value;
//    RRDVAR_FLAGS flags:24;
//    RRDVAR_TYPE type:8;
//    int16_t refcount;
//    void *owner;
//};
//
//struct varname {
//    STRING *name;
//    SPINLOCK spinlock;
//    uint16_t size;
//    uint16_t used;
//    struct var *array;
//};
//

bool rrdsetvar_get_custom_chart_variable_value(RRDSET *st, STRING *variable, NETDATA_DOUBLE *result);
bool rrdvar_get_custom_host_variable_value(RRDHOST *host, STRING *variable, NETDATA_DOUBLE *result);

struct variable_lookup_score {
#ifdef NETDATA_INTERNAL_CHECKS
    RRDSET *st;
    const char *source;
#endif
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
#ifdef NETDATA_INTERNAL_CHECKS
        .st = st,
        .source = source,
#endif
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
        if(rrdsetvar_get_custom_chart_variable_value(st, vbd->variable, &n)) {
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
    vbd->dimension_selection = DIM_SELECT_NORMAL;

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
    vbd->dimension_selection = vbd_back.dimension_selection;

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

bool alert_variable_lookup(STRING *variable, void *data, NETDATA_DOUBLE *result) {
    static STRING *last_collected_t = NULL, *green = NULL, *red = NULL, *update_every = NULL;

    struct variable_lookup_job vbd = { 0 };

//    const char *v_name = string2str(variable);
//    bool trace_this = false;
//    if(strcmp(v_name, "btrfs_allocated") == 0)
//        trace_this = true;

    bool found = false;

#ifdef NETDATA_INTERNAL_CHECKS
    const char *source = NULL;
    RRDSET *source_st = NULL;
#endif

    RRDCALC *rc = data;
    RRDSET *st = rc->rrdset;

    if(!st)
        return false;

    if(unlikely(!last_collected_t)) {
        last_collected_t = string_strdupz("last_collected_t");
        green = string_strdupz("green");
        red = string_strdupz("red");
        update_every = string_strdupz("update_every");
    }

    if(variable == last_collected_t) {
        *result = (NETDATA_DOUBLE)st->last_collected_time.tv_sec;
#ifdef NETDATA_INTERNAL_CHECKS
        source = "last_collected_t";
        source_st = st;
#endif
        found = true;
        goto log;
    }

    if(variable == update_every) {
        *result = (NETDATA_DOUBLE)st->update_every;
#ifdef NETDATA_INTERNAL_CHECKS
        source = "update_every";
        source_st = st;
#endif
        found = true;
        goto log;
    }

    if(variable == green) {
        *result = (NETDATA_DOUBLE)rc->config.green;
#ifdef NETDATA_INTERNAL_CHECKS
        source = "green";
        source_st = st;
#endif
        found = true;
        goto log;
    }

    if(variable == red) {
        *result = (NETDATA_DOUBLE)rc->config.red;
#ifdef NETDATA_INTERNAL_CHECKS
        source = "red";
        source_st = st;
#endif
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

#ifdef NETDATA_INTERNAL_CHECKS
        source = best->source;
        source_st = best->st;
#endif
        *result = best->value;
        freez(vbd.result.array);
    }
    else {
        found = false;
        *result = NAN;
    }

log:
#ifdef NETDATA_INTERNAL_CHECKS
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

    string_freez(vbd.dim);

    return found;
}
