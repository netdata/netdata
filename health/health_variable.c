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

struct variable_lookup_score {
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
};

static void variable_lookup_add_result_with_score(struct variable_lookup_job *vbd, NETDATA_DOUBLE n, size_t score) {
    if(vbd->result.used >= vbd->result.size) {
        if(!vbd->result.size)
            vbd->result.size = 1;

        vbd->result.size *= 2;
        vbd->result.array = reallocz(vbd->result.array, sizeof(struct variable_lookup_score) * vbd->result.size);
    }

    vbd->result.array[vbd->result.used++] = (struct variable_lookup_score) {
        .value = n,
        .score = score,
    };
}

bool variable_lookup_in_chart_dimensions(struct variable_lookup_job *vbd, RRDSET *st) {
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
        size_t score = rrdlabels_common_count(vbd->rc->rrdset->rrdlabels, st->rrdlabels);

        switch (vbd->dimension_selection) {
            case DIM_SELECT_NORMAL:
                variable_lookup_add_result_with_score(vbd, (NETDATA_DOUBLE)rd->collector.last_stored_value, score);
                break;
            case DIM_SELECT_RAW:
                variable_lookup_add_result_with_score(vbd, (NETDATA_DOUBLE)rd->collector.last_collected_value, score);
                break;
            case DIM_SELECT_LAST_COLLECTED:
                variable_lookup_add_result_with_score(vbd, (NETDATA_DOUBLE)rd->collector.last_collected_time.tv_sec, score);
                break;
        }

        dictionary_acquired_item_release(st->rrddim_root_index, item);
        found = true;
    }

    // TODO find the chart variables
    {
        ;
    }

    // alert names
    {
        rw_spinlock_read_lock(&st->alerts.spinlock);
        for(RRDCALC *rc = st->alerts.base ; rc ; rc = rc->next) {
            if(rc->config.name == vbd->variable) {
                variable_lookup_add_result_with_score(vbd, (NETDATA_DOUBLE)rc->value, SIZE_MAX);
                found = true;
                break;
            }
        }
        rw_spinlock_read_unlock(&st->alerts.spinlock);
    }

    return found;
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
        found = variable_lookup_in_chart_dimensions(vbd, rrdset_acquired_to_rrdset(rsa));
        rrdset_acquired_release(rsa);
        if(found) goto cleanup;
    }

    // TODO lookup context in contexts, then foreach chart

cleanup:
    string_freez(vbd->dim);
    *vbd = vbd_back;
    return found;
}

bool variable_lookup(STRING *variable, void *data, NETDATA_DOUBLE *result) {
    static STRING *last_collected_t = NULL, *green = NULL, *red = NULL, *update_every = NULL;

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
        return true;
    }

    if(variable == update_every) {
        *result = (NETDATA_DOUBLE)st->update_every;
        return true;
    }

    if(variable == green) {
        *result = (NETDATA_DOUBLE)rc->config.green;
        return true;
    }

    if(variable == red) {
        *result = (NETDATA_DOUBLE)rc->config.red;
        return true;
    }

    // find the dimension id/name

    struct variable_lookup_job vbd = {
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

    bool found = variable_lookup_in_chart_dimensions(&vbd, st);
    if(found) goto find_best_scored;

    // TODO find the host variables

    // find the components of the variable
    {
        char id[string_strlen(vbd.dim) + 1];
        memcpy(id, string2str(vbd.dim), string_strlen(vbd.dim));
        id[string_strlen(vbd.dim)] = '\0';

        char *dot = strrchr(id, '.');
        while(dot && !found) {
            *dot = '\0';

            if(strchr(id, '.') == NULL) break;

            found = variable_lookup_context(&vbd, id, dot + 1);

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

        *result = best->value;
        freez(vbd.result.array);
    }
    else {
        found = false;
        *result = NAN;
    }

    string_freez(vbd.dim);

    return found;
}
