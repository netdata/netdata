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

    for (struct instance *instance = engine->instance_root; instance; instance = instance->next) {
        if (!instance->disabled && (engine->now % instance->config.update_every >=
                                    instance->config.update_every - localhost->rrd_update_every)) {
            instance->scheduled = 1;
            instances_were_scheduled = 1;
            instance->before = engine->now;
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
NETDATA_DOUBLE exporting_calculate_value_from_stored_data(
    struct instance *instance,
    RRDDIM *rd,
    time_t *last_timestamp)
{
    RRDSET *st = rd->rrdset;
#ifdef NETDATA_INTERNAL_CHECKS
    RRDHOST *host = st->rrdhost;
#endif
    time_t after = instance->after;
    time_t before = instance->before;

    // find the edges of the rrd database for this chart
    time_t first_t = rd->tiers[0]->query_ops.oldest_time(rd->tiers[0]->db_metric_handle);
    time_t last_t = rd->tiers[0]->query_ops.latest_time(rd->tiers[0]->db_metric_handle);
    time_t update_every = st->update_every;
    struct rrddim_query_handle handle;

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
            D_EXPORTING,
            "EXPORTING: %s.%s.%s: aligned timeframe %lu to %lu is outside the chart's database range %lu to %lu",
            host->hostname,
            st->id,
            rrddim_id(rd),
            (unsigned long)after,
            (unsigned long)before,
            (unsigned long)first_t,
            (unsigned long)last_t);
        return NAN;
    }

    *last_timestamp = before;

    size_t counter = 0;
    NETDATA_DOUBLE sum = 0;

    for (rd->tiers[0]->query_ops.init(rd->tiers[0]->db_metric_handle, &handle, after, before, TIER_QUERY_FETCH_SUM); !rd->tiers[0]->query_ops.is_finished(&handle);) {
        STORAGE_POINT sp = rd->tiers[0]->query_ops.next_metric(&handle);

        if (unlikely(storage_point_is_empty(sp))) {
            // not collected
            continue;
        }

        sum += sp.sum;
        counter += sp.count;
    }
    rd->tiers[0]->query_ops.finalize(&handle);

    if (unlikely(!counter)) {
        debug(
            D_EXPORTING,
            "EXPORTING: %s.%s.%s: no values stored in database for range %lu to %lu",
            host->hostname,
            st->id,
            rrddim_id(rd),
            (unsigned long)after,
            (unsigned long)before);
        return NAN;
    }

    if (unlikely(EXPORTING_OPTIONS_DATA_SOURCE(instance->config.options) == EXPORTING_SOURCE_DATA_SUM))
        return sum;

    return sum / (NETDATA_DOUBLE)counter;
}

/**
 * Start batch formatting for every connector instance's buffer
 *
 * @param engine an engine data structure.
 */
void start_batch_formatting(struct engine *engine)
{
    for (struct instance *instance = engine->instance_root; instance; instance = instance->next) {
        if (instance->scheduled) {
            uv_mutex_lock(&instance->mutex);
            if (instance->start_batch_formatting && instance->start_batch_formatting(instance) != 0) {
                error("EXPORTING: cannot start batch formatting for %s", instance->config.name);
                disable_instance(instance);
            }
        }
    }
}

/**
 * Start host formatting for every connector instance's buffer
 *
 * @param engine an engine data structure.
 * @param host a data collecting host.
 */
void start_host_formatting(struct engine *engine, RRDHOST *host)
{
    for (struct instance *instance = engine->instance_root; instance; instance = instance->next) {
        if (instance->scheduled) {
            if (rrdhost_is_exportable(instance, host)) {
                if (instance->start_host_formatting && instance->start_host_formatting(instance, host) != 0) {
                    error("EXPORTING: cannot start host formatting for %s", instance->config.name);
                    disable_instance(instance);
                }
            } else {
                instance->skip_host = 1;
            }
        }
    }
}

/**
 * Start chart formatting for every connector instance's buffer
 *
 * @param engine an engine data structure.
 * @param st a chart.
 */
void start_chart_formatting(struct engine *engine, RRDSET *st)
{
    for (struct instance *instance = engine->instance_root; instance; instance = instance->next) {
        if (instance->scheduled && !instance->skip_host) {
            if (rrdset_is_exportable(instance, st)) {
                if (instance->start_chart_formatting && instance->start_chart_formatting(instance, st) != 0) {
                    error("EXPORTING: cannot start chart formatting for %s", instance->config.name);
                    disable_instance(instance);
                }
            } else {
                instance->skip_chart = 1;
            }
        }
    }
}

/**
 * Format metric for every connector instance's buffer
 *
 * @param engine an engine data structure.
 * @param rd a dimension(metric) in the Netdata database.
 */
void metric_formatting(struct engine *engine, RRDDIM *rd)
{
    for (struct instance *instance = engine->instance_root; instance; instance = instance->next) {
        if (instance->scheduled && !instance->skip_host && !instance->skip_chart) {
            if (instance->metric_formatting && instance->metric_formatting(instance, rd) != 0) {
                error("EXPORTING: cannot format metric for %s", instance->config.name);
                disable_instance(instance);
                continue;
            }
            instance->stats.buffered_metrics++;
        }
    }
}

/**
 * End chart formatting for every connector instance's buffer
 *
 * @param engine an engine data structure.
 * @param a chart.
 */
void end_chart_formatting(struct engine *engine, RRDSET *st)
{
    for (struct instance *instance = engine->instance_root; instance; instance = instance->next) {
        if (instance->scheduled && !instance->skip_host && !instance->skip_chart) {
            if (instance->end_chart_formatting && instance->end_chart_formatting(instance, st) != 0) {
                error("EXPORTING: cannot end chart formatting for %s", instance->config.name);
                disable_instance(instance);
                continue;
            }
        }
        instance->skip_chart = 0;
    }
}

/**
 * Format variables for every connector instance's buffer
 *
 * @param engine an engine data structure.
 * @param host a data collecting host.
  */
void variables_formatting(struct engine *engine, RRDHOST *host)
{
    for (struct instance *instance = engine->instance_root; instance; instance = instance->next) {
        if (instance->scheduled && !instance->skip_host && should_send_variables(instance)) {
            if (instance->variables_formatting && instance->variables_formatting(instance, host) != 0){ 
                error("EXPORTING: cannot format variables for %s", instance->config.name);
                disable_instance(instance);
                continue;
            }
            // sum all variables as one metrics
            instance->stats.buffered_metrics++;
        }
    }
}

/**
 * End host formatting for every connector instance's buffer
 *
 * @param engine an engine data structure.
 * @param host a data collecting host.
 */
void end_host_formatting(struct engine *engine, RRDHOST *host)
{
    for (struct instance *instance = engine->instance_root; instance; instance = instance->next) {
        if (instance->scheduled && !instance->skip_host) {
            if (instance->end_host_formatting && instance->end_host_formatting(instance, host) != 0) {
                error("EXPORTING: cannot end host formatting for %s", instance->config.name);
                disable_instance(instance);
                continue;
            }
        }
        instance->skip_host = 0;
    }
}

/**
 * End batch formatting for every connector instance's buffer
 *
 * @param engine an engine data structure.
 */
void end_batch_formatting(struct engine *engine)
{
    for (struct instance *instance = engine->instance_root; instance; instance = instance->next) {
        if (instance->scheduled) {
            if (instance->end_batch_formatting && instance->end_batch_formatting(instance) != 0) {
                error("EXPORTING: cannot end batch formatting for %s", instance->config.name);
                disable_instance(instance);
                continue;
            }
            uv_mutex_unlock(&instance->mutex);
            instance->data_is_ready = 1;
            uv_cond_signal(&instance->cond_var);

            instance->scheduled = 0;
            instance->after = instance->before;
        }
    }
}

/**
 * Prepare buffers
 *
 * Walk through the Netdata database and fill buffers for every scheduled exporting connector instance according to
 * configured rules.
 *
 * @param engine an engine data structure.
 */
void prepare_buffers(struct engine *engine)
{
    netdata_thread_disable_cancelability();
    start_batch_formatting(engine);

    rrd_rdlock();
    RRDHOST *host;
    rrdhost_foreach_read(host)
    {
        rrdhost_rdlock(host);
        start_host_formatting(engine, host);
        RRDSET *st;
        rrdset_foreach_read(st, host)
        {
            rrdset_rdlock(st);
            start_chart_formatting(engine, st);

            RRDDIM *rd;
            rrddim_foreach_read(rd, st)
                metric_formatting(engine, rd);

            end_chart_formatting(engine, st);
            rrdset_unlock(st);
        }
        variables_formatting(engine, host);
        end_host_formatting(engine, host);
        rrdhost_unlock(host);
    }
    rrd_unlock();
    netdata_thread_enable_cancelability();

    end_batch_formatting(engine);
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

    if (instance->labels_buffer)
        buffer_flush(instance->labels_buffer);

    return 0;
}

/**
 * End a batch for a simple connector
 *
 * @param instance an instance data structure.
 * @return Returns 0 on success, 1 on failure.
 */
int simple_connector_end_batch(struct instance *instance)
{
    struct simple_connector_data *simple_connector_data =
        (struct simple_connector_data *)instance->connector_specific_data;
    struct stats *stats = &instance->stats;

    BUFFER *instance_buffer = (BUFFER *)instance->buffer;
    struct simple_connector_buffer *last_buffer = simple_connector_data->last_buffer;

    if (!last_buffer->buffer) {
        last_buffer->buffer = buffer_create(0);
    }

    if (last_buffer->used) {
        // ring buffer is full, reuse the oldest element
        simple_connector_data->first_buffer = simple_connector_data->first_buffer->next;

        stats->data_lost_events++;
        stats->lost_metrics += last_buffer->buffered_metrics;
        stats->lost_bytes += last_buffer->buffered_bytes;
    }

    // swap buffers
    BUFFER *tmp_buffer = last_buffer->buffer;
    last_buffer->buffer = instance_buffer;
    instance->buffer = instance_buffer = tmp_buffer;

    buffer_flush(instance_buffer);

    if (last_buffer->header)
        buffer_flush(last_buffer->header);
    else
        last_buffer->header = buffer_create(0);

    if (instance->prepare_header)
        instance->prepare_header(instance);

    // The stats->buffered_metrics is used in the simple connector batch formatting as a variable for the number
    // of metrics, added in the current iteration, so we are clearing it here. We will use the
    // simple_connector_data->total_buffered_metrics in the worker to show the statistics.
    size_t buffered_metrics = (size_t)stats->buffered_metrics;
    stats->buffered_metrics = 0;

    size_t buffered_bytes = buffer_strlen(last_buffer->buffer);

    last_buffer->buffered_metrics = buffered_metrics;
    last_buffer->buffered_bytes = buffered_bytes;
    last_buffer->used++;

    simple_connector_data->total_buffered_metrics += buffered_metrics;
    stats->buffered_bytes += buffered_bytes;

    simple_connector_data->last_buffer = simple_connector_data->last_buffer->next;

    return 0;
}
