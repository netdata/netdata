// SPDX-License-Identifier: GPL-3.0-or-later

#include "internal.h"

uint64_t query_scope_foreach_host(SIMPLE_PATTERN *scope_hosts_sp, SIMPLE_PATTERN *hosts_sp, foreach_host_cb_t cb, void *data, uint64_t *hard_hash, uint64_t *soft_hash, char *host_uuid_buffer) {
    char uuid[UUID_STR_LEN];
    if(!host_uuid_buffer) host_uuid_buffer = uuid;
    host_uuid_buffer[0] = '\0';

    RRDHOST *host;
    size_t count = 0;
    uint64_t v_hash = 0;
    uint64_t h_hash = 0;

    dfe_start_reentrant(rrdhost_root_index, host) {
        if(host->node_id)
            uuid_unparse_lower(*host->node_id, host_uuid_buffer);

        SIMPLE_PATTERN_RESULT ret = SP_MATCHED_POSITIVE;
        if(scope_hosts_sp) {
            ret = simple_pattern_matches_string_extract(scope_hosts_sp, host->hostname, NULL, 0);
            if(ret == SP_NOT_MATCHED) {
                ret = simple_pattern_matches_extract(scope_hosts_sp, host->machine_guid, NULL, 0);
                if(ret == SP_NOT_MATCHED && *host_uuid_buffer)
                    ret = simple_pattern_matches_extract(scope_hosts_sp, host_uuid_buffer, NULL, 0);
            }
        }

        if(ret == SP_MATCHED_POSITIVE) {
            if(hosts_sp) {
                ret = simple_pattern_matches_string_extract(hosts_sp, host->hostname, NULL, 0);
                if(ret == SP_NOT_MATCHED) {
                    ret = simple_pattern_matches_extract(hosts_sp, host->machine_guid, NULL, 0);
                    if(ret == SP_NOT_MATCHED && *host_uuid_buffer)
                        ret = simple_pattern_matches_extract(hosts_sp, host_uuid_buffer, NULL, 0);
                }
            }

            bool queryable_host = (ret == SP_MATCHED_POSITIVE);

            count++;
            v_hash += dictionary_version(host->rrdctx.contexts);
            h_hash += dictionary_version(host->rrdctx.hub_queue);
            cb(data, host, queryable_host);
        }
    }
    dfe_done(host);

    if(hard_hash)
        *hard_hash = v_hash;

    if(soft_hash)
        *soft_hash = h_hash;

    return count;
}

size_t query_scope_foreach_context(RRDHOST *host, const char *scope_contexts, SIMPLE_PATTERN *scope_contexts_sp, SIMPLE_PATTERN *contexts_sp, foreach_context_cb_t cb, bool queryable_host, void *data) {
    size_t added = 0;

    RRDCONTEXT_ACQUIRED *rca = NULL;

    if(scope_contexts)
        rca = (RRDCONTEXT_ACQUIRED *)dictionary_get_and_acquire_item(host->rrdctx.contexts, scope_contexts);

    if(likely(rca)) {
        // we found it!

        bool queryable_context = queryable_host;
        RRDCONTEXT *rc = rrdcontext_acquired_value(rca);
        if(queryable_context && contexts_sp && !simple_pattern_matches_string(contexts_sp, rc->id))
            queryable_context = false;

        if(cb(data, rca, queryable_context))
            added++;

        rrdcontext_release(rca);
    }
    else {
        // Probably it is a pattern, we need to search for it...
        RRDCONTEXT *rc;
        dfe_start_read(host->rrdctx.contexts, rc) {
            if(scope_contexts_sp && !simple_pattern_matches_string(scope_contexts_sp, rc->id))
                continue;

            bool queryable_context = queryable_host;
            if(queryable_context && contexts_sp && !simple_pattern_matches_string(contexts_sp, rc->id))
                queryable_context = false;

            if(cb(data, (RRDCONTEXT_ACQUIRED *)rc_dfe.item, queryable_context))
                added++;
        }
        dfe_done(rc);
    }

    return added;
}

