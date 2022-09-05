// SPDX-License-Identifier: GPL-3.0-or-later

#include "exporting_engine.h"


bool exporting_labels_filter_callback(const char *name, const char *value, RRDLABEL_SRC ls, void *data) {
    (void)name;
    (void)value;
    struct instance *instance = (struct instance *)data;
    return should_send_label(instance, ls);
}

/**
 * Check if the connector instance should export the host metrics
 *
 * @param instance an exporting connector instance.
 * @param host a data collecting host.
 * @return Returns 1 if the connector instance should export the host metrics
 */
int rrdhost_is_exportable(struct instance *instance, RRDHOST *host)
{
    if (host->exporting_flags == NULL)
        host->exporting_flags = callocz(instance->engine->instance_num, sizeof(size_t));

    RRDHOST_FLAGS *flags = &host->exporting_flags[instance->index];

    if (unlikely((*flags & (RRDHOST_FLAG_EXPORTING_SEND | RRDHOST_FLAG_EXPORTING_DONT_SEND)) == 0)) {
        const char *host_name = (host == localhost) ? "localhost" : rrdhost_hostname(host);

        if (!instance->config.hosts_pattern || simple_pattern_matches(instance->config.hosts_pattern, host_name)) {
            *flags |= RRDHOST_FLAG_EXPORTING_SEND;
            info("enabled exporting of host '%s' for instance '%s'", host_name, instance->config.name);
        } else {
            *flags |= RRDHOST_FLAG_EXPORTING_DONT_SEND;
            info("disabled exporting of host '%s' for instance '%s'", host_name, instance->config.name);
        }
    }

    if (likely(*flags & RRDHOST_FLAG_EXPORTING_SEND))
        return 1;
    else
        return 0;
}

/**
 * Check if the connector instance should export the chart
 *
 * @param instance an exporting connector instance.
 * @param st a chart.
 * @return Returns 1 if the connector instance should export the chart
 */
int rrdset_is_exportable(struct instance *instance, RRDSET *st)
{
#ifdef NETDATA_INTERNAL_CHECKS
    RRDHOST *host = st->rrdhost;
#endif

    // Do not export anomaly rates charts.
    if (st->state && st->state->is_ar_chart)
        return 0;

    if (st->exporting_flags == NULL)
        st->exporting_flags = callocz(instance->engine->instance_num, sizeof(size_t));

    RRDSET_FLAGS *flags = &st->exporting_flags[instance->index];

    if(unlikely(*flags & RRDSET_FLAG_EXPORTING_IGNORE))
        return 0;

    if(unlikely(!(*flags & RRDSET_FLAG_EXPORTING_SEND))) {
        // we have not checked this chart
        if(simple_pattern_matches(instance->config.charts_pattern, rrdset_id(st)) || simple_pattern_matches(instance->config.charts_pattern, rrdset_name(st)))
            *flags |= RRDSET_FLAG_EXPORTING_SEND;
        else {
            *flags |= RRDSET_FLAG_EXPORTING_IGNORE;
            debug(D_EXPORTING, "EXPORTING: not sending chart '%s' of host '%s', because it is disabled for exporting.", rrdset_id(st), rrdhost_hostname(host));
            return 0;
        }
    }

    if(unlikely(!rrdset_is_available_for_exporting_and_alarms(st))) {
        debug(D_EXPORTING, "EXPORTING: not sending chart '%s' of host '%s', because it is not available for exporting.", rrdset_id(st), rrdhost_hostname(host));
        return 0;
    }

    if(unlikely(st->rrd_memory_mode == RRD_MEMORY_MODE_NONE && !(EXPORTING_OPTIONS_DATA_SOURCE(instance->config.options) == EXPORTING_SOURCE_DATA_AS_COLLECTED))) {
        debug(D_EXPORTING, "EXPORTING: not sending chart '%s' of host '%s' because its memory mode is '%s' and the exporting engine requires database access.", rrdset_id(st), rrdhost_hostname(host), rrd_memory_mode_name(host->rrd_memory_mode));
        return 0;
    }

    return 1;
}
