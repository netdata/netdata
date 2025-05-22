// SPDX-License-Identifier: GPL-3.0-or-later

#include "jsonwrap.h"
#include "jsonwrap-internal.h"

static void rrdset_rrdcalc_entries_v2(BUFFER *wb, RRDINSTANCE_ACQUIRED *ria) {
    RRDSET *st = rrdinstance_acquired_rrdset(ria);
    if(st) {
        rw_spinlock_read_lock(&st->alerts.spinlock);
        if(st->alerts.base) {
            buffer_json_member_add_object(wb, "alerts");
            for(RRDCALC *rc = st->alerts.base; rc ;rc = rc->next) {
                if(rc->status < RRDCALC_STATUS_CLEAR)
                    continue;

                buffer_json_member_add_object(wb, string2str(rc->config.name));
                buffer_json_member_add_string(wb, "st", rrdcalc_status2string(rc->status));
                buffer_json_member_add_double(wb, "vl", rc->value);
                buffer_json_member_add_string(wb, "un", string2str(rc->config.units));
                buffer_json_object_close(wb);
            }
            buffer_json_object_close(wb);
        }
        rw_spinlock_read_unlock(&st->alerts.spinlock);
    }
}

void query_target_detailed_objects_tree(BUFFER *wb, RRDR *r, RRDR_OPTIONS options) {
    QUERY_TARGET *qt = r->internal.qt;
    buffer_json_member_add_object(wb, "nodes");

    time_t now_s = now_realtime_sec();
    RRDHOST *last_host = NULL;
    RRDCONTEXT_ACQUIRED *last_rca = NULL;
    RRDINSTANCE_ACQUIRED *last_ria = NULL;

    size_t h = 0, c = 0, i = 0, m = 0, q = 0;
    for(; h < qt->nodes.used ; h++) {
        QUERY_NODE *qn = query_node(qt, h);
        RRDHOST *host = qn->rrdhost;

        for( ;c < qt->contexts.used ;c++) {
            QUERY_CONTEXT *qc = query_context(qt, c);
            RRDCONTEXT_ACQUIRED *rca = qc->rca;
            if(!rrdcontext_acquired_belongs_to_host(rca, host)) break;

            for( ;i < qt->instances.used ;i++) {
                QUERY_INSTANCE *qi = query_instance(qt, i);
                RRDINSTANCE_ACQUIRED *ria = qi->ria;
                if(!rrdinstance_acquired_belongs_to_context(ria, rca)) break;

                for( ; m < qt->dimensions.used ; m++) {
                    QUERY_DIMENSION *qd = query_dimension(qt, m);
                    RRDMETRIC_ACQUIRED *rma = qd->rma;
                    if(!rrdmetric_acquired_belongs_to_instance(rma, ria)) break;

                    QUERY_METRIC *qm = NULL;
                    bool queried = false;
                    for( ; q < qt->query.used ;q++) {
                        QUERY_METRIC *tqm = query_metric(qt, q);
                        QUERY_DIMENSION *tqd = query_dimension(qt, tqm->link.query_dimension_id);
                        if(tqd->rma != rma) break;

                        queried = tqm->status & RRDR_DIMENSION_QUERIED;
                        qm = tqm;
                    }

                    if(!queried & !(options & RRDR_OPTION_ALL_DIMENSIONS))
                        continue;

                    if(host != last_host) {
                        if(last_host) {
                            if(last_rca) {
                                if(last_ria) {
                                    buffer_json_object_close(wb); // dimensions
                                    buffer_json_object_close(wb); // instance
                                    last_ria = NULL;
                                }
                                buffer_json_object_close(wb); // instances
                                buffer_json_object_close(wb); // context
                                last_rca = NULL;
                            }
                            buffer_json_object_close(wb); // contexts
                            buffer_json_object_close(wb); // host
                        }

                        buffer_json_member_add_object(wb, host->machine_guid);
                        if(qn->node_id[0])
                            buffer_json_member_add_string(wb, "nd", qn->node_id);
                        buffer_json_member_add_uint64(wb, "ni", qn->slot);
                        buffer_json_member_add_string(wb, "nm", rrdhost_hostname(host));
                        buffer_json_member_add_object(wb, "contexts");

                        last_host = host;
                    }

                    if(rca != last_rca) {
                        if(last_rca) {
                            if(last_ria) {
                                buffer_json_object_close(wb); // dimensions
                                buffer_json_object_close(wb); // instance
                                last_ria = NULL;
                            }
                            buffer_json_object_close(wb); // instances
                            buffer_json_object_close(wb); // context
                            last_rca = NULL;
                        }

                        buffer_json_member_add_object(wb, rrdcontext_acquired_id(rca));
                        buffer_json_member_add_object(wb, "instances");

                        last_rca = rca;
                    }

                    if(ria != last_ria) {
                        if(last_ria) {
                            buffer_json_object_close(wb); // dimensions
                            buffer_json_object_close(wb); // instance
                            last_ria = NULL;
                        }

                        buffer_json_member_add_object(wb, rrdinstance_acquired_id(ria));
                        buffer_json_member_add_string(wb, "nm", rrdinstance_acquired_name(ria));
                        buffer_json_member_add_time_t(wb, "ue", rrdinstance_acquired_update_every(ria));
                        RRDLABELS *labels = rrdinstance_acquired_labels(ria);
                        if(labels) {
                            buffer_json_member_add_object(wb, "labels");
                            rrdlabels_to_buffer_json_members(labels, wb);
                            buffer_json_object_close(wb);
                        }
                        rrdset_rrdcalc_entries_v2(wb, ria);
                        buffer_json_member_add_object(wb, "dimensions");

                        last_ria = ria;
                    }

                    buffer_json_member_add_object(wb, rrdmetric_acquired_id(rma));
                    {
                        buffer_json_member_add_string(wb, "nm", rrdmetric_acquired_name(rma));
                        buffer_json_member_add_uint64(wb, "qr", queried ? 1 : 0);
                        time_t first_entry_s = rrdmetric_acquired_first_entry(rma);
                        time_t last_entry_s = rrdmetric_acquired_last_entry(rma);
                        buffer_json_member_add_time_t(wb, "fe", first_entry_s);
                        buffer_json_member_add_time_t(wb, "le", last_entry_s ? last_entry_s : now_s);

                        if(qm) {
                            if(qm->status & RRDR_DIMENSION_GROUPED) {
                                // buffer_json_member_add_string(wb, "grouped_as_id", string2str(qm->grouped_as.id));
                                buffer_json_member_add_string(wb, "as", string2str(qm->grouped_as.name));
                            }

                            query_target_points_statistics(wb, qt, &qm->query_points);

                            if(options & RRDR_OPTION_DEBUG)
                                jsonwrap_query_metric_plan(wb, qm);
                        }
                    }
                    buffer_json_object_close(wb); // metric
                }
            }
        }
    }

    if(last_host) {
        if(last_rca) {
            if(last_ria) {
                buffer_json_object_close(wb); // dimensions
                buffer_json_object_close(wb); // instance
                last_ria = NULL;
            }
            buffer_json_object_close(wb); // instances
            buffer_json_object_close(wb); // context
            last_rca = NULL;
        }
        buffer_json_object_close(wb); // contexts
        buffer_json_object_close(wb); // host
        last_host = NULL;
    }
    buffer_json_object_close(wb); // hosts
}
