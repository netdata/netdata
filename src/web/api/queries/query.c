// SPDX-License-Identifier: GPL-3.0-or-later

#include "query-internal.h"

// ----------------------------------------------------------------------------
// fill RRDR for the whole chart

#ifdef NETDATA_INTERNAL_CHECKS
static void rrd2rrdr_log_request_response_metadata(RRDR *r
        , RRDR_OPTIONS options __maybe_unused
        , RRDR_TIME_GROUPING group_method
        , bool aligned
        , size_t group
        , time_t resampling_time
        , size_t resampling_group
        , time_t after_wanted
        , time_t after_requested
        , time_t before_wanted
        , time_t before_requested
        , size_t points_requested
        , size_t points_wanted
        //, size_t after_slot
        //, size_t before_slot
        , const char *msg
        ) {

    QUERY_TARGET *qt = r->internal.qt;
    time_t first_entry_s = qt->db.first_time_s;
    time_t last_entry_s = qt->db.last_time_s;

    internal_error(
    true,
    "rrd2rrdr() on %s update every %ld with %s grouping %s (group: %zu, resampling_time: %ld, resampling_group: %zu), "
         "after (got: %ld, want: %ld, req: %ld, db: %ld), "
         "before (got: %ld, want: %ld, req: %ld, db: %ld), "
         "duration (got: %ld, want: %ld, req: %ld, db: %ld), "
         "points (got: %zu, want: %zu, req: %zu), "
         "%s"
         , qt->id
         , qt->window.query_granularity

         // grouping
         , (aligned) ? "aligned" : "unaligned"
         , time_grouping_id2txt(group_method)
         , group
         , resampling_time
         , resampling_group

         // after
         , r->view.after
         , after_wanted
         , after_requested
         , first_entry_s

         // before
         , r->view.before
         , before_wanted
         , before_requested
         , last_entry_s

         // duration
         , (long)(r->view.before - r->view.after + qt->window.query_granularity)
         , (long)(before_wanted - after_wanted + qt->window.query_granularity)
         , (long)before_requested - after_requested
         , (long)((last_entry_s - first_entry_s) + qt->window.query_granularity)

         // points
         , r->rows
         , points_wanted
         , points_requested

         // message
         , msg
    );
}
#endif // NETDATA_INTERNAL_CHECKS

// ----------------------------------------------------------------------------
// query entry point

RRDR *rrd2rrdr_legacy(
        ONEWAYALLOC *owa,
        RRDSET *st, size_t points, time_t after, time_t before,
        RRDR_TIME_GROUPING group_method, time_t resampling_time, RRDR_OPTIONS options, const char *dimensions,
        const char *group_options, time_t timeout_ms, size_t tier, QUERY_SOURCE query_source,
        STORAGE_PRIORITY priority) {

    QUERY_TARGET_REQUEST qtr = {
            .version = 1,
            .st = st,
            .points = points,
            .after = after,
            .before = before,
            .time_group_method = group_method,
            .resampling_time = resampling_time,
            .options = options,
            .dimensions = dimensions,
            .time_group_options = group_options,
            .timeout_ms = timeout_ms,
            .tier = tier,
            .query_source = query_source,
            .priority = priority,
    };

    QUERY_TARGET *qt = query_target_create(&qtr);
    RRDR *r = rrd2rrdr(owa, qt);
    if(!r) {
        query_target_release(qt);
        return NULL;
    }

    r->internal.release_with_rrdr_qt = qt;
    return r;
}

RRDR *rrd2rrdr(ONEWAYALLOC *owa, QUERY_TARGET *qt) {
    if(!qt || !owa)
        return NULL;

    // qt.window members are the WANTED ones.
    // qt.request members are the REQUESTED ones.

    RRDR *r_tmp = rrd2rrdr_group_by_initialize(owa, qt);
    if(!r_tmp)
        return NULL;

    // the RRDR we group-by at
    RRDR *r = (r_tmp->group_by.r) ? r_tmp->group_by.r : r_tmp;

    // the final RRDR to return to callers
    RRDR *last_r = r_tmp;
    while(last_r->group_by.r)
        last_r = last_r->group_by.r;

    if(qt->window.relative)
        last_r->view.flags |= RRDR_RESULT_FLAG_RELATIVE;
    else
        last_r->view.flags |= RRDR_RESULT_FLAG_ABSOLUTE;

    // -------------------------------------------------------------------------
    // assign the processor functions
    rrdr_set_grouping_function(r_tmp, qt->window.time_group_method);

    // allocate any memory required by the grouping method
    r_tmp->time_grouping.create(r_tmp, qt->window.time_group_options);

    // -------------------------------------------------------------------------
    // do the work for each dimension

    time_t max_after = 0, min_before = 0;
    size_t max_rows = 0;

    long dimensions_used = 0, dimensions_nonzero = 0;
    size_t last_db_points_read = 0;
    size_t last_result_points_generated = 0;

    // internal_fatal(released_ops, "QUERY: released_ops should be NULL when the query starts");

    query_progress_set_finish_line(qt->request.transaction, qt->query.used);

    QUERY_ENGINE_OPS **ops = NULL;
    if(qt->query.used)
        ops = onewayalloc_callocz(owa, qt->query.used, sizeof(QUERY_ENGINE_OPS *));

    size_t capacity = MAX(netdata_conf_cpus() / 2, 4);
    size_t max_queries_to_prepare = (qt->query.used > (capacity - 1)) ? (capacity - 1) : qt->query.used;
    size_t queries_prepared = 0;
    while(queries_prepared < max_queries_to_prepare) {
        // preload another query
        ops[queries_prepared] = rrd2rrdr_query_ops_prep(r_tmp, queries_prepared);
        queries_prepared++;
    }

    QUERY_NODE *last_qn = NULL;
    usec_t last_ut = now_monotonic_usec();
    usec_t last_qn_ut = last_ut;

    for(size_t d = 0; d < qt->query.used ; d++) {
        QUERY_METRIC *qm = query_metric(qt, d);
        QUERY_DIMENSION *qd = query_dimension(qt, qm->link.query_dimension_id);
        QUERY_INSTANCE *qi = query_instance(qt, qm->link.query_instance_id);
        QUERY_CONTEXT *qc = query_context(qt, qm->link.query_context_id);
        QUERY_NODE *qn = query_node(qt, qm->link.query_node_id);

        usec_t now_ut = last_ut;
        if(qn != last_qn) {
            if(last_qn)
                last_qn->duration_ut = now_ut - last_qn_ut;

            last_qn = qn;
            last_qn_ut = now_ut;
        }

        if(queries_prepared < qt->query.used) {
            // preload another query
            ops[queries_prepared] = rrd2rrdr_query_ops_prep(r_tmp, queries_prepared);
            queries_prepared++;
        }

        size_t dim_in_rrdr_tmp = (r_tmp != r) ? 0 : d;

        // set the query target dimension options to rrdr
        r_tmp->od[dim_in_rrdr_tmp] = qm->status;

        // reset the grouping for the new dimension
        r_tmp->time_grouping.reset(r_tmp);

        if(ops[d]) {
            rrd2rrdr_query_execute(r_tmp, dim_in_rrdr_tmp, ops[d]);
            r_tmp->od[dim_in_rrdr_tmp] |= RRDR_DIMENSION_QUERIED;

            now_ut = now_monotonic_usec();
            qm->duration_ut = now_ut - last_ut;
            last_ut = now_ut;

            if(r_tmp != r) {
                // copy back whatever got updated from the temporary r

                // the query updates RRDR_DIMENSION_NONZERO
                qm->status = r_tmp->od[dim_in_rrdr_tmp];

                // the query updates these
                r->view.min = r_tmp->view.min;
                r->view.max = r_tmp->view.max;
                r->view.after = r_tmp->view.after;
                r->view.before = r_tmp->view.before;
                r->rows = r_tmp->rows;

                rrd2rrdr_group_by_add_metric(r, qm->grouped_as.first_slot, r_tmp, dim_in_rrdr_tmp,
                                             qt->request.group_by[0].aggregation, &qm->query_points, 0);
            }

            rrd2rrdr_query_ops_release(ops[d]); // reuse this ops allocation
            ops[d] = NULL;

            qi->metrics.queried++;
            qc->metrics.queried++;
            qn->metrics.queried++;

            qd->status |= QUERY_STATUS_QUERIED;
            qm->status |= RRDR_DIMENSION_QUERIED;

            if(qt->request.version >= 2) {
                // we need to make the query points positive now
                // since we will aggregate it across multiple dimensions
                storage_point_make_positive(qm->query_points);
                storage_point_merge_to(qi->query_points, qm->query_points);
                storage_point_merge_to(qc->query_points, qm->query_points);
                storage_point_merge_to(qn->query_points, qm->query_points);
                storage_point_merge_to(qt->query_points, qm->query_points);
            }
        }
        else {
            qi->metrics.failed++;
            qc->metrics.failed++;
            qn->metrics.failed++;

            qd->status |= QUERY_STATUS_FAILED;
            qm->status |= RRDR_DIMENSION_FAILED;

            continue;
        }

        pulse_queries_rrdr_query_completed(
            1,
            r_tmp->stats.db_points_read - last_db_points_read,
            r_tmp->stats.result_points_generated - last_result_points_generated,
            qt->request.query_source);

        last_db_points_read = r_tmp->stats.db_points_read;
        last_result_points_generated = r_tmp->stats.result_points_generated;

        if(qm->status & RRDR_DIMENSION_NONZERO)
            dimensions_nonzero++;

        // verify all dimensions are aligned
        if(unlikely(!dimensions_used)) {
            min_before = r->view.before;
            max_after = r->view.after;
            max_rows = r->rows;
        }
        else {
            if(r->view.after != max_after) {
                internal_error(true, "QUERY: 'after' mismatch between dimensions for chart '%s': max is %zu, dimension '%s' has %zu",
                               rrdinstance_acquired_id(qi->ria), (size_t)max_after, rrdmetric_acquired_id(qd->rma), (size_t)r->view.after);

                r->view.after = (r->view.after > max_after) ? r->view.after : max_after;
            }

            if(r->view.before != min_before) {
                internal_error(true, "QUERY: 'before' mismatch between dimensions for chart '%s': max is %zu, dimension '%s' has %zu",
                               rrdinstance_acquired_id(qi->ria), (size_t)min_before, rrdmetric_acquired_id(qd->rma), (size_t)r->view.before);

                r->view.before = (r->view.before < min_before) ? r->view.before : min_before;
            }

            if(r->rows != max_rows) {
                internal_error(true, "QUERY: 'rows' mismatch between dimensions for chart '%s': max is %zu, dimension '%s' has %zu",
                               rrdinstance_acquired_id(qi->ria), (size_t)max_rows, rrdmetric_acquired_id(qd->rma), (size_t)r->rows);

                r->rows = (r->rows > max_rows) ? r->rows : max_rows;
            }
        }

        dimensions_used++;

        bool cancel = false;
        if (qt->request.interrupt_callback && qt->request.interrupt_callback(qt->request.interrupt_callback_data)) {
            cancel = true;
            nd_log(NDLS_ACCESS, NDLP_NOTICE, "QUERY INTERRUPTED");
        }

        if (qt->request.timeout_ms && ((NETDATA_DOUBLE)(now_ut - qt->timings.received_ut) / 1000.0) > (NETDATA_DOUBLE)qt->request.timeout_ms) {
            cancel = true;
            nd_log(NDLS_ACCESS, NDLP_WARNING, "QUERY CANCELED RUNTIME EXCEEDED %0.2f ms (LIMIT %lld ms)",
                       (NETDATA_DOUBLE)(now_ut - qt->timings.received_ut) / 1000.0, (long long)qt->request.timeout_ms);
        }

        if(cancel) {
            r->view.flags |= RRDR_RESULT_FLAG_CANCEL;

            for(size_t i = d + 1; i < queries_prepared ; i++) {
                if(ops[i]) {
                    query_planer_finalize_remaining_plans(ops[i]);
                    rrd2rrdr_query_ops_release(ops[i]);
                    ops[i] = NULL;
                }
            }

            break;
        }
        else
            query_progress_done_step(qt->request.transaction, 1);
    }

    // free all resources used by the grouping method
    r_tmp->time_grouping.free(r_tmp);

    // get the final RRDR to send to the caller
    r = rrd2rrdr_group_by_finalize(r_tmp);
    
    // apply cardinality limit if requested
    r = rrd2rrdr_cardinality_limit(r);

#ifdef NETDATA_INTERNAL_CHECKS
    if (dimensions_used && !(r->view.flags & RRDR_RESULT_FLAG_CANCEL)) {
        if(r->internal.log)
            rrd2rrdr_log_request_response_metadata(r, qt->window.options, qt->window.time_group_method, qt->window.aligned, qt->window.group, qt->request.resampling_time, qt->window.resampling_group,
                                                   qt->window.after, qt->request.after, qt->window.before, qt->request.before,
                                                   qt->request.points, qt->window.points, /*after_slot, before_slot,*/
                                                   r->internal.log);

        if(r->rows != qt->window.points)
            rrd2rrdr_log_request_response_metadata(r, qt->window.options, qt->window.time_group_method, qt->window.aligned, qt->window.group, qt->request.resampling_time, qt->window.resampling_group,
                                                   qt->window.after, qt->request.after, qt->window.before, qt->request.before,
                                                   qt->request.points, qt->window.points, /*after_slot, before_slot,*/
                                                   "got 'points' is not wanted 'points'");

        if(qt->window.aligned && (r->view.before % query_view_update_every(qt)) != 0)
            rrd2rrdr_log_request_response_metadata(r, qt->window.options, qt->window.time_group_method, qt->window.aligned, qt->window.group, qt->request.resampling_time, qt->window.resampling_group,
                                                   qt->window.after, qt->request.after, qt->window.before, qt->request.before,
                                                   qt->request.points, qt->window.points, /*after_slot, before_slot,*/
                                                   "'before' is not aligned but alignment is required");

        // 'after' should not be aligned, since we start inside the first group
        //if(qt->window.aligned && (r->after % group) != 0)
        //    rrd2rrdr_log_request_response_metadata(r, qt->window.options, qt->window.group_method, qt->window.aligned, qt->window.group, qt->request.resampling_time, qt->window.resampling_group, qt->window.after, after_requested, before_wanted, before_requested, points_requested, points_wanted, after_slot, before_slot, "'after' is not aligned but alignment is required");

        if(r->view.before != qt->window.before)
            rrd2rrdr_log_request_response_metadata(r, qt->window.options, qt->window.time_group_method, qt->window.aligned, qt->window.group, qt->request.resampling_time, qt->window.resampling_group,
                                                   qt->window.after, qt->request.after, qt->window.before, qt->request.before,
                                                   qt->request.points, qt->window.points, /*after_slot, before_slot,*/
                                                   "chart is not aligned to requested 'before'");

        if(r->view.before != qt->window.before)
            rrd2rrdr_log_request_response_metadata(r, qt->window.options, qt->window.time_group_method, qt->window.aligned, qt->window.group, qt->request.resampling_time, qt->window.resampling_group,
                                                   qt->window.after, qt->request.after, qt->window.before, qt->request.before,
                                                   qt->request.points, qt->window.points, /*after_slot, before_slot,*/
                                                   "got 'before' is not wanted 'before'");

        // reported 'after' varies, depending on group
        if(r->view.after != qt->window.after)
            rrd2rrdr_log_request_response_metadata(r, qt->window.options, qt->window.time_group_method, qt->window.aligned, qt->window.group, qt->request.resampling_time, qt->window.resampling_group,
                                                   qt->window.after, qt->request.after, qt->window.before, qt->request.before,
                                                   qt->request.points, qt->window.points, /*after_slot, before_slot,*/
                                                   "got 'after' is not wanted 'after'");

    }
#endif

    // free the query pipelining ops
    for(size_t d = 0; d < qt->query.used ; d++) {
        rrd2rrdr_query_ops_release(ops[d]);
        ops[d] = NULL;
    }
    rrd2rrdr_query_ops_freeall(r);
    // internal_fatal(released_ops, "QUERY: released_ops should be NULL when the query ends");

    onewayalloc_freez(owa, ops);

    if(likely(dimensions_used && (qt->window.options & RRDR_OPTION_NONZERO) && !dimensions_nonzero))
        // when all the dimensions are zero, we should return all of them
        qt->window.options &= ~RRDR_OPTION_NONZERO;

    qt->timings.executed_ut = now_monotonic_usec();

    return r;
}
