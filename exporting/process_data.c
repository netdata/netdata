// SPDX-License-Identifier: GPL-3.0-or-later

#include "exporting_engine.h"

/**
 * Normalize chart and dimension names
 *
 * Substitute '_' for any special character except '.'.
 *
 * @param dst where to copy name to.
 * @param src where to copy name from.
 * @param max_len the maximum size of copied name.
 * @return Returns the size of the copied name.
 */
size_t exporting_name_copy(char *dst, const char *src, size_t max_len)
{
    size_t n;

    for (n = 0; *src && n < max_len; dst++, src++, n++) {
        char c = *src;

        if (c != '.' && !isalnum(c))
            *dst = '_';
        else
            *dst = c;
    }
    *dst = '\0';

    return n;
}

/**
 * Mark scheduled instances
 *
 * Any instance can have its own update interval. On every exporting engine update only those instances are picked,
 * which are scheduled for the update.
 *
 * @param engine an engine data structure.
 * @return Returns 1 if there are instances to process
 */
int mark_scheduled_instances(struct engine *engine)
{
    int instances_were_scheduled = 0;

    for (struct connector *connector = engine->connector_root; connector; connector = connector->next) {
        for (struct instance *instance = connector->instance_root; instance; instance = instance->next) {
            if (engine->now % instance->config.update_every < localhost->rrd_update_every) {
                instance->scheduled = 1;
                instances_were_scheduled = 1;
                instance->before = engine->now;
            }
        }
    }

    return instances_were_scheduled;
}

/**
 * Calculate the SUM or AVERAGE of a dimension, for any timeframe
 *
 * May return NAN if the database does not have any value in the give timeframe.
 *
 * @param instance an instance data structure.
 * @param rd a dimension(metric) in the Netdata database.
 * @param last_timestamp the timestamp that should be reported to the exporting connector instance.
 * @return Returns the value, calculated over the given period.
 */
calculated_number exporting_calculate_value_from_stored_data(
    struct instance *instance,
    RRDDIM *rd,
    time_t *last_timestamp)
{
    RRDSET *st = rd->rrdset;
    RRDHOST *host = st->rrdhost;
    time_t after = instance->after;
    time_t before = instance->before;

    // find the edges of the rrd database for this chart
    time_t first_t = rd->state->query_ops.oldest_time(rd);
    time_t last_t = rd->state->query_ops.latest_time(rd);
    time_t update_every = st->update_every;
    struct rrddim_query_handle handle;
    storage_number n;

    // step back a little, to make sure we have complete data collection
    // for all metrics
    after -= update_every * 2;
    before -= update_every * 2;

    // align the time-frame
    after = after - (after % update_every);
    before = before - (before % update_every);

    // for before, loose another iteration
    // the latest point will be reported the next time
    before -= update_every;

    if (unlikely(after > before))
        // this can happen when update_every > before - after
        after = before;

    if (unlikely(after < first_t))
        after = first_t;

    if (unlikely(before > last_t))
        before = last_t;

    if (unlikely(before < first_t || after > last_t)) {
        // the chart has not been updated in the wanted timeframe
        debug(
            D_BACKEND,
            "EXPORTING: %s.%s.%s: aligned timeframe %lu to %lu is outside the chart's database range %lu to %lu",
            host->hostname,
            st->id,
            rd->id,
            (unsigned long)after,
            (unsigned long)before,
            (unsigned long)first_t,
            (unsigned long)last_t);
        return NAN;
    }

    *last_timestamp = before;

    size_t counter = 0;
    calculated_number sum = 0;

    for (rd->state->query_ops.init(rd, &handle, after, before); !rd->state->query_ops.is_finished(&handle);) {
        time_t curr_t;
        n = rd->state->query_ops.next_metric(&handle, &curr_t);

        if (unlikely(!does_storage_number_exist(n))) {
            // not collected
            continue;
        }

        calculated_number value = unpack_storage_number(n);
        sum += value;

        counter++;
    }
    rd->state->query_ops.finalize(&handle);
    if (unlikely(!counter)) {
        debug(
            D_BACKEND,
            "EXPORTING: %s.%s.%s: no values stored in database for range %lu to %lu",
            host->hostname,
            st->id,
            rd->id,
            (unsigned long)after,
            (unsigned long)before);
        return NAN;
    }

    if (unlikely(EXPORTING_OPTIONS_DATA_SOURCE(instance->config.options) == EXPORTING_SOURCE_DATA_SUM))
        return sum;

    return sum / (calculated_number)counter;
}

/**
 * Start batch formatting for every connector instance's buffer
 *
 * @param engine an engine data structure.
 * @return Returns 0 on success, 1 on failure.
 */
int start_batch_formatting(struct engine *engine)
{
    for (struct connector *connector = engine->connector_root; connector; connector = connector->next) {
        for (struct instance *instance = connector->instance_root; instance; instance = instance->next) {
            if (instance->scheduled) {
                uv_mutex_lock(&instance->mutex);
                if (instance->start_batch_formatting && instance->start_batch_formatting(instance) != 0) {
                    error("EXPORTING: cannot start batch formatting for %s", instance->config.name);
                    return 1;
                }
            }
        }
    }

    return 0;
}

/**
 * Start host formatting for every connector instance's buffer
 *
 * @param engine an engine data structure.
 * @param host a data collecting host.
 * @return Returns 0 on success, 1 on failure.
 */
int start_host_formatting(struct engine *engine, RRDHOST *host)
{
    for (struct connector *connector = engine->connector_root; connector; connector = connector->next) {
        for (struct instance *instance = connector->instance_root; instance; instance = instance->next) {
            if (instance->scheduled) {
                if (rrdhost_is_exportable(instance, host)) {
                    if (instance->start_host_formatting && instance->start_host_formatting(instance, host) != 0) {
                        error("EXPORTING: cannot start host formatting for %s", instance->config.name);
                        return 1;
                    }
                } else {
                    instance->skip_host = 1;
                }
            }
        }
    }

    return 0;
}

/**
 * Start chart formatting for every connector instance's buffer
 *
 * @param engine an engine data structure.
 * @param a chart.
 * @return Returns 0 on success, 1 on failure.
 */
int start_chart_formatting(struct engine *engine, RRDSET *st)
{
    for (struct connector *connector = engine->connector_root; connector; connector = connector->next) {
        for (struct instance *instance = connector->instance_root; instance; instance = instance->next) {
            if (instance->scheduled && !instance->skip_host) {
                if (rrdset_is_exportable(instance, st)) {
                    if (instance->start_chart_formatting && instance->start_chart_formatting(instance, st) != 0) {
                        error("EXPORTING: cannot start chart formatting for %s", instance->config.name);
                        return 1;
                    }
                } else {
                    instance->skip_chart = 1;
                }
            }
        }
    }

    return 0;
}

/**
 * Format metric for every connector instance's buffer
 *
 * @param engine an engine data structure.
 * @param rd a dimension(metric) in the Netdata database.
 * @return Returns 0 on success, 1 on failure.
 */
int metric_formatting(struct engine *engine, RRDDIM *rd)
{
    for (struct connector *connector = engine->connector_root; connector; connector = connector->next) {
        for (struct instance *instance = connector->instance_root; instance; instance = instance->next) {
            if (instance->scheduled && !instance->skip_host && !instance->skip_chart) {
                if (instance->metric_formatting && instance->metric_formatting(instance, rd) != 0) {
                    error("EXPORTING: cannot format metric for %s", instance->config.name);
                    return 1;
                }
                instance->stats.chart_buffered_metrics++;
            }
        }
    }

    return 0;
}

/**
 * End chart formatting for every connector instance's buffer
 *
 * @param engine an engine data structure.
 * @param a chart.
 * @return Returns 0 on success, 1 on failure.
 */
int end_chart_formatting(struct engine *engine, RRDSET *st)
{
    for (struct connector *connector = engine->connector_root; connector; connector = connector->next) {
        for (struct instance *instance = connector->instance_root; instance; instance = instance->next) {
            if (instance->scheduled && !instance->skip_host && !instance->skip_chart) {
                if (instance->end_chart_formatting && instance->end_chart_formatting(instance, st) != 0) {
                    error("EXPORTING: cannot end chart formatting for %s", instance->config.name);
                    return 1;
                }
            }
            instance->skip_chart = 0;
        }
    }

    return 0;
}

/**
 * End host formatting for every connector instance's buffer
 *
 * @param engine an engine data structure.
 * @param host a data collecting host.
 * @return Returns 0 on success, 1 on failure.
 */
int end_host_formatting(struct engine *engine, RRDHOST *host)
{
    for (struct connector *connector = engine->connector_root; connector; connector = connector->next) {
        for (struct instance *instance = connector->instance_root; instance; instance = instance->next) {
            if (instance->scheduled && !instance->skip_host) {
                if (instance->end_host_formatting && instance->end_host_formatting(instance, host) != 0) {
                    error("EXPORTING: cannot end host formatting for %s", instance->config.name);
                    return 1;
                }
            }
            instance->skip_host = 0;
        }
    }

    return 0;
}

/**
 * End batch formatting for every connector instance's buffer
 *
 * @param engine an engine data structure.
 * @return Returns 0 on success, 1 on failure.
 */
int end_batch_formatting(struct engine *engine)
{
    for (struct connector *connector = engine->connector_root; connector; connector = connector->next) {
        for (struct instance *instance = connector->instance_root; instance; instance = instance->next) {
            if (instance->scheduled) {
                if (instance->end_batch_formatting && instance->end_batch_formatting(instance) != 0) {
                    error("EXPORTING: cannot end batch formatting for %s", instance->config.name);
                    return 1;
                }
                uv_mutex_unlock(&instance->mutex);
                uv_cond_signal(&instance->cond_var);

                instance->scheduled = 0;
                instance->after = instance->before;
            }
        }
    }

    return 0;
}

/**
 * Prepare buffers
 *
 * Walk through the Netdata database and fill buffers for every scheduled exporting connector instance according to
 * configured rules.
 *
 * @param engine an engine data structure.
 * @return Returns 0 on success, 1 on failure.
 */
int prepare_buffers(struct engine *engine)
{
    netdata_thread_disable_cancelability();
    rrd_rdlock();
    if (start_batch_formatting(engine) != 0)
        return 1;

    RRDHOST *host;
    rrdhost_foreach_read(host)
    {
        rrdhost_rdlock(host);
        if (start_host_formatting(engine, host) != 0)
            return 1;
        RRDSET *st;
        rrdset_foreach_read(st, host)
        {
            rrdset_rdlock(st);
            if (start_chart_formatting(engine, st) != 0)
                return 1;

            RRDDIM *rd;
            rrddim_foreach_read(rd, st)
            {
                if (metric_formatting(engine, rd) != 0)
                    return 1;
            }

            if (end_chart_formatting(engine, st) != 0)
                return 1;
            rrdset_unlock(st);
        }

        if (end_host_formatting(engine, host) != 0)
            return 1;
        rrdhost_unlock(host);
    }

    if (end_batch_formatting(engine) != 0)
        return 1;
    rrd_unlock();
    netdata_thread_enable_cancelability();

    return 0;
}

/**
 * Flush a buffer with host labels
 *
 * @param instance an instance data structure.
 * @param host a data collecting host.
 * @return Always returns 0.
 */
int flush_host_labels(struct instance *instance, RRDHOST *host)
{
    (void)host;

    if (instance->labels)
        buffer_flush(instance->labels);

    return 0;
}

/**
 * Notify workers
 *
 * Notify exporting connector instance working threads that data is ready to send.
 *
 * @param engine an engine data structure.
 * @return Returns 0 on success, 1 on failure.
 */
int notify_workers(struct engine *engine)
{
    (void)engine;

    return 0;
}
