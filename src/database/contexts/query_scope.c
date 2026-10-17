// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdcontext-internal.h"

typedef enum {
    QUERY_SCOPE_HOST_OUT_OF_SCOPE = 0,
    QUERY_SCOPE_HOST_IN_SCOPE,          // in scope, but not selected by the hosts pattern
    QUERY_SCOPE_HOST_QUERYABLE,
} QUERY_SCOPE_HOST_MATCH;

// Matches a host by hostname, machine guid or node id. Fills host_node_id_str with the host's node id.
static QUERY_SCOPE_HOST_MATCH query_scope_host_match(RRDHOST *host, SIMPLE_PATTERN *scope_hosts_sp, SIMPLE_PATTERN *hosts_sp,
                                                     char *host_node_id_str) {
    RRDHOST_IDENTITY identity = { 0 };

    if(scope_hosts_sp || hosts_sp)
        identity = rrdhost_identity_acquire(host);

    if(!UUIDiszero(host->node_id))
        uuid_unparse_lower(host->node_id.uuid, host_node_id_str);
    else
        host_node_id_str[0] = '\0';

    SIMPLE_PATTERN_RESULT match = SP_MATCHED_POSITIVE;
    if(scope_hosts_sp) {
        match = simple_pattern_matches_string_extract(scope_hosts_sp, identity.hostname, NULL, 0);
        if(match == SP_NOT_MATCHED) {
            match = simple_pattern_matches_extract(scope_hosts_sp, host->machine_guid, NULL, 0);
            if(match == SP_NOT_MATCHED && *host_node_id_str)
                match = simple_pattern_matches_extract(scope_hosts_sp, host_node_id_str, NULL, 0);
        }
    }

    if(match != SP_MATCHED_POSITIVE) {
        rrdhost_identity_release(&identity);
        return QUERY_SCOPE_HOST_OUT_OF_SCOPE;
    }

    if(hosts_sp) {
        match = simple_pattern_matches_string_extract(hosts_sp, identity.hostname, NULL, 0);
        if(match == SP_NOT_MATCHED) {
            match = simple_pattern_matches_extract(hosts_sp, host->machine_guid, NULL, 0);
            if(match == SP_NOT_MATCHED && *host_node_id_str)
                match = simple_pattern_matches_extract(hosts_sp, host_node_id_str, NULL, 0);
        }
    }

    rrdhost_identity_release(&identity);
    return (match == SP_MATCHED_POSITIVE) ? QUERY_SCOPE_HOST_QUERYABLE : QUERY_SCOPE_HOST_IN_SCOPE;
}

static void query_scope_versions_add(struct query_versions *versions, RRDHOST *host) {
    versions->contexts_hard_hash += dictionary_version(host->rrdctx.contexts);
    versions->contexts_soft_hash += rrdcontext_queue_version(&host->rrdctx.hub_queue);
    versions->alerts_hard_hash += dictionary_version(host->rrdcalc_root_index);
    versions->alerts_soft_hash += __atomic_load_n(&host->health_transitions, __ATOMIC_RELAXED);
}

ssize_t query_scope_foreach_host(SIMPLE_PATTERN *scope_hosts_sp, SIMPLE_PATTERN *hosts_sp,
                                  foreach_host_cb_t cb, void *data,
                                  struct query_versions *versions,
                                  char *host_node_id_str) {
    char uuid[UUID_STR_LEN];
    if(!host_node_id_str) host_node_id_str = uuid;
    host_node_id_str[0] = '\0';

    RRDHOST *host;
    ssize_t added = 0;
    struct query_versions v = { 0 };

    dfe_start_read(rrdhost_root_index, host) {
        QUERY_SCOPE_HOST_MATCH match = query_scope_host_match(host, scope_hosts_sp, hosts_sp, host_node_id_str);
        if(match == QUERY_SCOPE_HOST_OUT_OF_SCOPE)
            continue;

        dfe_unlock(host);

        query_scope_versions_add(&v, host);
        ssize_t ret = cb(data, host, match == QUERY_SCOPE_HOST_QUERYABLE);
        if(ret < 0) {
            added = ret;
            break;
        }
        added += ret;
    }
    dfe_done(host);

    if(versions)
        *versions = v;

    return added;
}

// Pins every queryable host in scope (see metadata_lifetime_lock in rrdhost.h), so the caller can keep
// using them after the walk - from other threads too - until query_scope_hosts_unpin(). A host that
// cannot be pinned is left out: a removal holds it, or is trying to take it at that moment (an attempt
// that fails when the host is already pinned elsewhere). The versions cover every host in scope, as
// query_scope_foreach_host() computes them.
//
// It waits for rrd_rdlock(): call it only from a thread that no rrd_wrlock() holder waits on (see
// rrdhost_apply_by_machine_guid()). The pins MUST be released on the same thread.
size_t query_scope_hosts_pin(SIMPLE_PATTERN *scope_hosts_sp, SIMPLE_PATTERN *hosts_sp,
                             RRDHOST ***hosts, struct query_versions *versions) {
    char uuid[UUID_STR_LEN];
    struct query_versions v = { 0 };
    size_t count = 0;

    rrd_rdlock();

    // hosts are added under rrd_wrlock(), so the index cannot grow while we hold the read lock
    size_t entries = dictionary_entries(rrdhost_root_index);
    RRDHOST **array = mallocz((entries ? entries : 1) * sizeof(*array));

    RRDHOST *host;
    dfe_start_read(rrdhost_root_index, host) {
        QUERY_SCOPE_HOST_MATCH match = query_scope_host_match(host, scope_hosts_sp, hosts_sp, uuid);
        if(match == QUERY_SCOPE_HOST_OUT_OF_SCOPE)
            continue;

        query_scope_versions_add(&v, host);

        if(match == QUERY_SCOPE_HOST_QUERYABLE && count < entries && rrdhost_pin_try(host))
            array[count++] = host;
    }
    dfe_done(host);

    rrd_rdunlock();

    if(versions)
        *versions = v;

    *hosts = array;
    return count;
}

void query_scope_hosts_unpin(RRDHOST **hosts, size_t count) {
    for(size_t i = 0; i < count; i++)
        rrdhost_pin_release(hosts[i]);

    freez(hosts);
}

// A plain context name (no separators, wildcards, negation or escapes) compiles to a pattern
// that matches only that name, so a dictionary miss for it is final.
static bool scope_contexts_is_plain_name(const char *scope_contexts, SIMPLE_PATTERN *scope_contexts_sp) {
    return scope_contexts && *scope_contexts && scope_contexts_sp && !strchr(scope_contexts, '\\') &&
           !simple_pattern_contains_wildcards(scope_contexts, SIMPLE_PATTERN_DEFAULT_WEB_SEPARATORS);
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
    else if(!scope_contexts_is_plain_name(scope_contexts, scope_contexts_sp)) {
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

