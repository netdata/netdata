// SPDX-License-Identifier: GPL-3.0-or-later

#define RRDHOST_INTERNALS
#include "rrd.h"
#include "rrd-metadata.h"
#include "contexts/rrdcontext-context-registry.h"

// Collect metrics metadata from all hosts
RRDSTATS_METADATA rrdstats_metadata_collect(void) {
    RRDSTATS_METADATA metadata = {
        .nodes = { .total = 0, .receiving = 0, .sending = 0, .archived = 0 },
        .metrics = { .collected = 0, .available = 0 },
        .instances = { .collected = 0, .available = 0 },
        .contexts = { .collected = 0, .available = 0, .unique = 0 }
    };

    rrd_rdlock();

    if(!rrdhost_root_index) {
        rrd_rdunlock();
        return metadata;
    }

    RRDHOST *host;
    dfe_start_read(rrdhost_root_index, host) {
        metadata.nodes.total++;

        metadata.metrics.available += __atomic_load_n(&host->rrdctx.metrics_count, __ATOMIC_RELAXED);
        metadata.instances.available += __atomic_load_n(&host->rrdctx.instances_count, __ATOMIC_RELAXED);
        metadata.contexts.available += __atomic_load_n(&host->rrdctx.contexts_count, __ATOMIC_RELAXED);

        if(rrdhost_flag_check(host, RRDHOST_FLAG_STREAM_SENDER_CONNECTED))
            metadata.nodes.sending++;

        if (rrdhost_is_online(host)) {
            metadata.metrics.collected += __atomic_load_n(&host->collected.metrics_count, __ATOMIC_RELAXED);
            metadata.instances.collected += __atomic_load_n(&host->collected.instances_count, __ATOMIC_RELAXED);
            metadata.contexts.collected += __atomic_load_n(&host->collected.contexts_count, __ATOMIC_RELAXED);

            if(host != localhost)
                metadata.nodes.receiving++;
        }
        else
            metadata.nodes.archived++;
    }
    dfe_done(host);

    rrd_rdunlock();
    
    // Get the count of unique contexts from our registry
    metadata.contexts.unique = rrdcontext_context_registry_unique_count();

    return metadata;
}