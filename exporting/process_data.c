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
 * @return Returns 0 on success, 1 on failure.
 */
int mark_scheduled_instances(struct engine *engine)
{
    (void)engine;

    return 0;
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
            if (connector->start_batch_formatting) {
                if (connector->start_batch_formatting(instance) != 0) {
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
 * @return Returns 0 on success, 1 on failure.
 */
int start_host_formatting(struct engine *engine)
{
    for (struct connector *connector = engine->connector_root; connector; connector = connector->next) {
        for (struct instance *instance = connector->instance_root; instance; instance = instance->next) {
            if (connector->start_host_formatting) {
                if (connector->start_host_formatting(instance) != 0) {
                    error("EXPORTING: cannot start host formatting for %s", instance->config.name);
                    return 1;
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
 * @return Returns 0 on success, 1 on failure.
 */
int start_chart_formatting(struct engine *engine)
{
    for (struct connector *connector = engine->connector_root; connector; connector = connector->next) {
        for (struct instance *instance = connector->instance_root; instance; instance = instance->next) {
            if (connector->start_chart_formatting) {
                if (connector->start_chart_formatting(instance) != 0) {
                    error("EXPORTING: cannot start chart formatting for %s", instance->config.name);
                    return 1;
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
 * @return Returns 0 on success, 1 on failure.
 */
int metric_formatting(struct engine *engine, RRDDIM *rd)
{
    for (struct connector *connector = engine->connector_root; connector; connector = connector->next) {
        for (struct instance *instance = connector->instance_root; instance; instance = instance->next) {
            if (connector->metric_formatting) {
                if (connector->metric_formatting(instance, rd) != 0) {
                    error("EXPORTING: cannot format metric for %s", instance->config.name);
                    return 1;
                }
            }
        }
    }

    return 0;
}

/**
 * End chart formatting for every connector instance's buffer
 *
 * @param engine an engine data structure.
 * @return Returns 0 on success, 1 on failure.
 */
int end_chart_formatting(struct engine *engine)
{
    for (struct connector *connector = engine->connector_root; connector; connector = connector->next) {
        for (struct instance *instance = connector->instance_root; instance; instance = instance->next) {
            if (connector->end_chart_formatting) {
                if (connector->end_chart_formatting(instance) != 0) {
                    error("EXPORTING: cannot end chart formatting for %s", instance->config.name);
                    return 1;
                }
            }
        }
    }

    return 0;
}


/**
 * End host formatting for every connector instance's buffer
 *
 * @param engine an engine data structure.
 * @return Returns 0 on success, 1 on failure.
 */
int end_host_formatting(struct engine *engine)
{
    for (struct connector *connector = engine->connector_root; connector; connector = connector->next) {
        for (struct instance *instance = connector->instance_root; instance; instance = instance->next) {
            if (connector->end_host_formatting) {
                if (connector->end_host_formatting(instance) != 0) {
                    error("EXPORTING: cannot end host formatting for %s", instance->config.name);
                    return 1;
                }
            }
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
            if (connector->end_batch_formatting) {
                if (connector->end_batch_formatting(instance) != 0) {
                    error("EXPORTING: cannot end batch formatting for %s", instance->config.name);
                    return 1;
                }
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
    // rrd_rdlock();
    if (start_batch_formatting(engine) != 0)
        return 1;

    RRDHOST *host;
    rrdhost_foreach_read(host)
    {
        // rrdhost_rdlock(host);
        if (start_host_formatting(engine) != 0)
            return 1;
        RRDSET *st;
        rrdset_foreach_read(st, host)
        {
            // rrdset_rdlock(st);
            if (start_chart_formatting(engine) != 0)
                return 1;

            RRDDIM *rd;
            rrddim_foreach_read(rd, st)
            {
                if (metric_formatting(engine, rd) != 0)
                    return 1;
            }

            if (end_chart_formatting(engine) != 0)
                return 1;
            // rrdset_unlock(st);
        }

        if (end_host_formatting(engine) != 0)
            return 1;
        // rrdhost_unlock(host);
    }

    if (end_batch_formatting(engine) != 0)
        return 1;
    // rrd_unlock();
    netdata_thread_enable_cancelability();

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
