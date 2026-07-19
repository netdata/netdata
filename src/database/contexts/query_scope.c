// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdcontext-internal.h"

ssize_t query_scope_foreach_host(SIMPLE_PATTERN *scope_hosts_sp, SIMPLE_PATTERN *hosts_sp,
                                  foreach_host_cb_t cb, void *data,
                                  struct query_versions *versions,
                                  char *host_node_id_str) {
    char uuid[UUID_STR_LEN];
    if(!host_node_id_str) host_node_id_str = uuid;
    host_node_id_str[0] = '\0';

    ssize_t added = 0;
    uint64_t v_hash = 0;
    uint64_t h_hash = 0;
    uint64_t a_hash = 0;
    uint64_t t_hash = 0;

    SIMPLE_PATTERN_INDEX_MATCHES *scope_matches = rrdhost_identity_index_search(scope_hosts_sp);
    SIMPLE_PATTERN_INDEX_MATCHES *host_matches = rrdhost_identity_index_search(hosts_sp);
    if(unlikely(!scope_matches || !host_matches)) {
        simple_pattern_index_matches_free(host_matches);
        simple_pattern_index_matches_free(scope_matches);
        return -1;
    }

    Word_t cursor = 0;
    for(RRDHOST *host = simple_pattern_index_matches_first(scope_matches, &cursor);
        host;
        host = simple_pattern_index_matches_next(scope_matches, &cursor)) {
        if(!UUIDiszero(host->node_id))
            uuid_unparse_lower(host->node_id.uuid, host_node_id_str);
        else
            host_node_id_str[0] = '\0';

        bool queryable_host = simple_pattern_index_matches_contains(host_matches, host);

        v_hash += dictionary_version(host->rrdctx.contexts);
        h_hash += rrdcontext_queue_version(&host->rrdctx.hub_queue);
        a_hash += dictionary_version(host->rrdcalc_root_index);
        t_hash += __atomic_load_n(&host->health_transitions, __ATOMIC_RELAXED);
        ssize_t ret = cb(data, host, queryable_host);
        if(ret < 0) {
            added = ret;
            break;
        }
        added += ret;
    }

    simple_pattern_index_matches_free(host_matches);
    simple_pattern_index_matches_free(scope_matches);

    if(versions) {
        versions->contexts_hard_hash = v_hash;
        versions->contexts_soft_hash = h_hash;
        versions->alerts_hard_hash = a_hash;
        versions->alerts_soft_hash = t_hash;
    }

    return added;
}

ssize_t query_scope_foreach_context(RRDHOST *host, const char *scope_contexts, SIMPLE_PATTERN *scope_contexts_sp,
                                   SIMPLE_PATTERN *contexts_sp, foreach_context_cb_t cb, bool queryable_host, void *data) {
    if(unlikely(!host->rrdctx.contexts))
        return 0;

    ssize_t added = 0;

    RRDCONTEXT_ACQUIRED *rca = NULL;

    if(scope_contexts)
        rca = (RRDCONTEXT_ACQUIRED *)dictionary_get_and_acquire_item(host->rrdctx.contexts, scope_contexts);

    if(likely(rca)) {
        // we found it!

        bool queryable_context = queryable_host;
        RRDCONTEXT *rc = rrdcontext_acquired_value(rca);
        if(queryable_context && contexts_sp && !simple_pattern_matches_string(contexts_sp, rc->id))
            queryable_context = false;

        added = cb(data, rca, queryable_context);

        rrdcontext_release(rca);
    }
    else {
        // Probably it is a pattern, we need to search for it...
        RRDCONTEXT *rc;
        dfe_start_read(host->rrdctx.contexts, rc) {
            if(scope_contexts_sp && !simple_pattern_matches_string(scope_contexts_sp, rc->id))
                continue;

            dfe_unlock(rc);

            bool queryable_context = queryable_host;
            if(queryable_context && contexts_sp && !simple_pattern_matches_string(contexts_sp, rc->id))
                queryable_context = false;

            ssize_t ret = cb(data, (RRDCONTEXT_ACQUIRED *)rc_dfe.item, queryable_context);

            if(ret < 0) {
                added = ret;
                break;
            }

            added += ret;
        }
        dfe_done(rc);
    }

    return added;
}
