// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdcontext-internal.h"

static __thread size_t th_ignored_metrics = 0, th_ignored_instances = 0, th_zero_retention_metrics = 0;

static void rrdinstance_load_clabel(SQL_CLABEL_DATA *sld, void *data) {
    RRDINSTANCE *ri = data;
    rrdlabels_add(ri->rrdlabels, sld->label_key, sld->label_value, sld->label_source);
}

void load_instance_labels_on_demand(nd_uuid_t *uuid, void *data) {
    ctx_get_label_list(uuid, rrdinstance_load_clabel, data);
}

static void rrdinstance_load_dimension_callback(SQL_DIMENSION_DATA *sd, void *data) {
    RRDHOST *host = data;

    UUIDMAP_ID id = uuidmap_create(sd->dim_id);
    time_t min_first_time_t = LONG_MAX, max_last_time_t = 0;
    get_metric_retention_by_id(host, id, &min_first_time_t, &max_last_time_t, NULL);
    if((!min_first_time_t || min_first_time_t == LONG_MAX) && !max_last_time_t) {
        uuidmap_free(id);
        th_zero_retention_metrics++;
        return;
    }

    RRDCONTEXT_ACQUIRED *rca = (RRDCONTEXT_ACQUIRED *)dictionary_get_and_acquire_item(host->rrdctx.contexts, sd->context);
    if(!rca) {
        th_ignored_metrics++;
        uuidmap_free(id);
        return;
    }
    RRDCONTEXT *rc = rrdcontext_acquired_value(rca);
    if(!rc) {
        th_ignored_metrics++;
        rrdcontext_release(rca);
        uuidmap_free(id);
        return;
    }

    RRDINSTANCE_ACQUIRED *ria = (RRDINSTANCE_ACQUIRED *)dictionary_get_and_acquire_item(rc->rrdinstances, sd->chart_id);
    if(!ria) {
        th_ignored_metrics++;
        rrdcontext_release(rca);
        uuidmap_free(id);
        return;
    }
    RRDINSTANCE *ri = rrdinstance_acquired_value(ria);
    if(!ri) {
        th_ignored_metrics++;
        rrdinstance_release(ria);
        rrdcontext_release(rca);
        uuidmap_free(id);
        return;
    }

    RRDMETRIC trm = {
        .uuid = id,
        .id = string_strdupz(sd->id),
        .name = string_strdupz(sd->name),
        .flags = RRD_FLAG_ARCHIVED | RRD_FLAG_UPDATE_REASON_LOAD_SQL, // no need for atomic
    };
    if(sd->hidden) trm.flags |= RRD_FLAG_HIDDEN;

    dictionary_set(ri->rrdmetrics, string2str(trm.id), &trm, sizeof(trm));

    rrdinstance_release(ria);
    rrdcontext_release(rca);
}

static void rrdinstance_load_instance_callback(SQL_CHART_DATA *sc, void *data) {
    RRDHOST *host = data;

    RRDCONTEXT tc = {
        .id = string_strdupz(sc->context),
        .title = string_strdupz(sc->title),
        .units = string_strdupz(sc->units),
        .family = string_strdupz(sc->family),
        .priority = sc->priority,
        .chart_type = sc->chart_type,
        .flags = RRD_FLAG_ARCHIVED | RRD_FLAG_UPDATE_REASON_LOAD_SQL, // no need for atomics
        .rrdhost = host,
    };

    RRDCONTEXT_ACQUIRED *rca = (RRDCONTEXT_ACQUIRED *)dictionary_set_and_acquire_item(host->rrdctx.contexts, string2str(tc.id), &tc, sizeof(tc));
    if(!rca) {
        th_ignored_instances++;
        string_freez(tc.id);
        string_freez(tc.title);
        string_freez(tc.units);
        string_freez(tc.family);
        return;
    }

    RRDCONTEXT *rc = rrdcontext_acquired_value(rca);
    if(!rc) {
        th_ignored_instances++;
        rrdcontext_release(rca);
        return;
    }

    RRDINSTANCE tri = {
        .uuid = uuidmap_create(sc->chart_id),
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

    RRDINSTANCE_ACQUIRED *ria = (RRDINSTANCE_ACQUIRED *)dictionary_set_and_acquire_item(rc->rrdinstances, sc->id, &tri, sizeof(tri));
    if(!ria) {
        th_ignored_instances++;
        uuidmap_free(tri.uuid);
        string_freez(tri.id);
        string_freez(tri.name);
        string_freez(tri.title);
        string_freez(tri.units);
        string_freez(tri.family);
        rrdcontext_release(rca);
        return;
    }

    rrdinstance_release(ria);
    rrdcontext_release(rca);
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
}

void rrdhost_load_rrdcontext_data(RRDHOST *host) {
    if(host->rrdctx.contexts) return;

    rrdhost_create_rrdcontexts(host);
    if (host->rrd_memory_mode != RRD_DB_MODE_DBENGINE)
        return;

    th_ignored_metrics = th_ignored_instances = th_zero_retention_metrics = 0;

    ctx_get_context_list(&host->host_id.uuid, rrdcontext_load_context_callback, host);
    ctx_get_chart_list(&host->host_id.uuid, rrdinstance_load_instance_callback, host);
    ctx_get_dimension_list(&host->host_id.uuid, rrdinstance_load_dimension_callback, host);

    size_t ignored_metrics = th_ignored_metrics, ignored_instances = th_ignored_instances, zero_retention_metrics = th_zero_retention_metrics;
    size_t loaded_metrics = 0, loaded_instances = 0, loaded_contexts = 0;
    size_t loaded_and_deleted_instances = 0, loaded_and_deleted_contexts = 0;

    RRDCONTEXT *rc;
    dfe_start_read(host->rrdctx.contexts, rc) {
        size_t instances = 0;

        RRDINSTANCE *ri;
        dfe_start_write(rc->rrdinstances, ri) {
            size_t metrics = 0;

            RRDMETRIC *rm;
            dfe_start_read(ri->rrdmetrics, rm) {
                rrdmetric_trigger_updates(rm, __FUNCTION__ );
                loaded_metrics++;
                metrics++;
            }
            dfe_done(rm);
            dictionary_garbage_collect(ri->rrdmetrics);

            if(!metrics) {
                dictionary_del(rc->rrdinstances, ri_dfe.name);
                loaded_and_deleted_instances++;
            }
            else {
                rrdinstance_trigger_updates(ri, __FUNCTION__);
                loaded_instances++;
                instances++;
            }
        }
        dfe_done(ri);
        dictionary_garbage_collect(rc->rrdinstances);

        if(!instances) {
            metadata_queue_ctx_host_cleanup(&host->host_id.uuid, rc_dfe.name);
            rrdcontext_delete_after_loading(host, rc);
            loaded_and_deleted_contexts++;
        }
        else {
            rrdcontext_trigger_updates(rc, __FUNCTION__);
            rrdcontext_initial_processing_after_loading(rc);
            loaded_contexts++;
        }
    }
    dfe_done(rc);
    dictionary_garbage_collect(host->rrdctx.contexts);
    rrdcontext_garbage_collect_single_host(host, false);

    nd_log(NDLS_DAEMON, ignored_metrics || ignored_instances ? NDLP_WARNING : NDLP_NOTICE,
           "RRDCONTEXT: metadata for node '%s': "
           "contexts %zu (deleted %zu), "
           "instances %zu (deleted %zu, ignored %zu), and "
           "metrics %zu (ignored %zu, zero retention %zu)",
           rrdhost_hostname(host),
           loaded_contexts, loaded_and_deleted_contexts,
           loaded_instances, loaded_and_deleted_instances, ignored_instances,
           loaded_metrics, ignored_metrics, zero_retention_metrics);
}
