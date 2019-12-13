// SPDX-License-Identifier: GPL-3.0-or-later

#include "exporting_engine.h"

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
        host->exporting_flags = callocz(instance->connector->engine->instance_num, sizeof(size_t));

    RRDHOST_FLAGS *flags = &host->exporting_flags[instance->index];

    if (unlikely((*flags & (RRDHOST_FLAG_BACKEND_SEND | RRDHOST_FLAG_BACKEND_DONT_SEND)) == 0)) {
        char *host_name = (host == localhost) ? "localhost" : host->hostname;

        if (!instance->config.hosts_pattern || simple_pattern_matches(instance->config.hosts_pattern, host_name)) {
            *flags |= RRDHOST_FLAG_BACKEND_SEND;
            info("enabled exporting of host '%s' for instance '%s'", host_name, instance->config.name);
        } else {
            *flags |= RRDHOST_FLAG_BACKEND_DONT_SEND;
            info("disabled exporting of host '%s' for instance '%s'", host_name, instance->config.name);
        }
    }

    if (likely(*flags & RRDHOST_FLAG_BACKEND_SEND))
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
    RRDHOST *host = st->rrdhost;

    if (st->exporting_flags == NULL)
        st->exporting_flags = callocz(instance->connector->engine->instance_num, sizeof(size_t));

    RRDSET_FLAGS *flags = &st->exporting_flags[instance->index];

    if(unlikely(*flags & RRDSET_FLAG_BACKEND_IGNORE))
        return 0;

    if(unlikely(!(*flags & RRDSET_FLAG_BACKEND_SEND))) {
        // we have not checked this chart
        if(simple_pattern_matches(instance->config.charts_pattern, st->id) || simple_pattern_matches(instance->config.charts_pattern, st->name))
            *flags |= RRDSET_FLAG_BACKEND_SEND;
        else {
            *flags |= RRDSET_FLAG_BACKEND_IGNORE;
            debug(D_BACKEND, "BACKEND: not sending chart '%s' of host '%s', because it is disabled for backends.", st->id, host->hostname);
            return 0;
        }
    }

    if(unlikely(!rrdset_is_available_for_backends(st))) {
        debug(D_BACKEND, "BACKEND: not sending chart '%s' of host '%s', because it is not available for backends.", st->id, host->hostname);
        return 0;
    }

    if(unlikely(st->rrd_memory_mode == RRD_MEMORY_MODE_NONE && !(EXPORTING_OPTIONS_DATA_SOURCE(instance->config.options) == EXPORTING_SOURCE_DATA_AS_COLLECTED))) {
        debug(D_BACKEND, "BACKEND: not sending chart '%s' of host '%s' because its memory mode is '%s' and the backend requires database access.", st->id, host->hostname, rrd_memory_mode_name(host->rrd_memory_mode));
        return 0;
    }

    return 1;
}
