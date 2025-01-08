// SPDX-License-Identifier: GPL-3.0-or-later

#include "internal.h"

static __thread size_t ignored_metrics = 0, ignored_instances = 0;
static __thread size_t loaded_metrics = 0, loaded_instances = 0, loaded_contexts = 0;

static void rrdinstance_load_clabel(SQL_CLABEL_DATA *sld, void *data) {
    RRDINSTANCE *ri = data;
    rrdlabels_add(ri->rrdlabels, sld->label_key, sld->label_value, sld->label_source);
}

void load_instance_labels_on_demand(nd_uuid_t *uuid, void *data) {
    ctx_get_label_list(uuid, rrdinstance_load_clabel, data);
}

static void rrdinstance_load_dimension_callback(SQL_DIMENSION_DATA *sd, void *data) {
    RRDHOST *host = data;
    RRDCONTEXT_ACQUIRED *rca = (RRDCONTEXT_ACQUIRED *)dictionary_get_and_acquire_item(host->rrdctx.contexts, sd->context);
    if(!rca) {
        ignored_metrics++;
//        nd_log(NDLS_DAEMON, NDLP_ERR,
//               "RRDCONTEXT: context '%s' is not found in host '%s' - not loading dimensions",
//               sd->context, rrdhost_hostname(host));
        return;
    }
    RRDCONTEXT *rc = rrdcontext_acquired_value(rca);

    RRDINSTANCE_ACQUIRED *ria = (RRDINSTANCE_ACQUIRED *)dictionary_get_and_acquire_item(rc->rrdinstances, sd->chart_id);
    if(!ria) {
        rrdcontext_release(rca);
        ignored_metrics++;
//        nd_log(NDLS_DAEMON, NDLP_ERR,
//               "RRDCONTEXT: instance '%s' of context '%s' is not found in host '%s' - not loading dimensions",
//               sd->chart_id, sd->context, rrdhost_hostname(host));
        return;
    }
    RRDINSTANCE *ri = rrdinstance_acquired_value(ria);

    RRDMETRIC trm = {
        .id = string_strdupz(sd->id),
        .name = string_strdupz(sd->name),
        .flags = RRD_FLAG_ARCHIVED | RRD_FLAG_UPDATE_REASON_LOAD_SQL, // no need for atomic
    };
    if(sd->hidden) trm.flags |= RRD_FLAG_HIDDEN;

    uuid_copy(trm.uuid, sd->dim_id);

    dictionary_set(ri->rrdmetrics, string2str(trm.id), &trm, sizeof(trm));

    rrdinstance_release(ria);
    rrdcontext_release(rca);
    loaded_metrics++;
}

static void rrdinstance_load_instance_callback(SQL_CHART_DATA *sc, void *data) {
    RRDHOST *host = data;

    RRDCONTEXT_ACQUIRED *rca = (RRDCONTEXT_ACQUIRED *)dictionary_get_and_acquire_item(host->rrdctx.contexts, sc->context);
    if(!rca) {
        ignored_instances++;
//        nd_log(NDLS_DAEMON, NDLP_ERR,
//               "RRDCONTEXT: context '%s' is not found in host '%s' - not loadings instances",
//               sc->context, rrdhost_hostname(host));
        return;
    }
    RRDCONTEXT *rc = rrdcontext_acquired_value(rca);

    RRDINSTANCE tri = {
        .id = string_strdupz(sc->id),
        .name = string_strdupz(sc->name),
        .title = string_strdupz(sc->title),
        .units = string_strdupz(sc->units),
        .family = string_strdupz(sc->family),
        .chart_type = sc->chart_type,
        .priority = sc->priority,
        .update_every_s = sc->update_every,
        .flags = RRD_FLAG_ARCHIVED | RRD_FLAG_UPDATE_REASON_LOAD_SQL, // no need for atomics
    };
    uuid_copy(tri.uuid, sc->chart_id);

    RRDINSTANCE_ACQUIRED *ria = (RRDINSTANCE_ACQUIRED *)dictionary_set_and_acquire_item(rc->rrdinstances, sc->id, &tri, sizeof(tri));

    rrdinstance_release(ria);
    rrdcontext_release(rca);
    loaded_instances++;
}

static void rrdcontext_load_context_callback(VERSIONED_CONTEXT_DATA *ctx_data, void *data) {
    RRDHOST *host = data;
    (void)host;

    RRDCONTEXT trc = {
        .id = string_strdupz(ctx_data->id),
        .flags = RRD_FLAG_ARCHIVED | RRD_FLAG_UPDATE_REASON_LOAD_SQL, // no need for atomics

        // no need to set more data here
        // we only need the hub data

        .hub = *ctx_data,
    };
    dictionary_set(host->rrdctx.contexts, string2str(trc.id), &trc, sizeof(trc));
    loaded_contexts++;
}

void rrdhost_load_rrdcontext_data(RRDHOST *host) {
    if(host->rrdctx.contexts) return;

    rrdhost_create_rrdcontexts(host);
    if (host->rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE)
        return;

    ignored_metrics = 0;
    ignored_instances = 0;
    loaded_metrics = 0;
    loaded_instances = 0;
    loaded_contexts = 0;

    ctx_get_context_list(&host->host_id.uuid, rrdcontext_load_context_callback, host);
    ctx_get_chart_list(&host->host_id.uuid, rrdinstance_load_instance_callback, host);
    ctx_get_dimension_list(&host->host_id.uuid, rrdinstance_load_dimension_callback, host);

    nd_log(NDLS_DAEMON, ignored_metrics || ignored_instances ? NDLP_WARNING : NDLP_NOTICE,
           "RRDCONTEXT: metadata for node '%s':"
           " loaded %zu contexts, %zu instances, and %zu metrics,"
           " ignored %zu instances and %zu metrics",
           rrdhost_hostname(host),
           loaded_contexts, loaded_instances, loaded_metrics,
           ignored_instances, ignored_metrics);

    RRDCONTEXT *rc;
    dfe_start_read(host->rrdctx.contexts, rc) {
        RRDINSTANCE *ri;
        dfe_start_read(rc->rrdinstances, ri) {
            RRDMETRIC *rm;
            dfe_start_read(ri->rrdmetrics, rm) {
                rrdmetric_trigger_updates(rm, __FUNCTION__ );
            }
            dfe_done(rm);
            rrdinstance_trigger_updates(ri, __FUNCTION__ );
        }
        dfe_done(ri);
        rrdcontext_trigger_updates(rc, __FUNCTION__ );
    }
    dfe_done(rc);

    rrdcontext_garbage_collect_single_host(host, false);
}
