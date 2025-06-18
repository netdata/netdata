// SPDX-License-Identifier: GPL-3.0-or-later

#include "api_v2_contexts.h"
#include "../rrdlabels-aggregated.h"
#include "aclk/aclk_capas.h"
#include "web/mcp/mcp.h"
#include "libnetdata/json/json-keys.h"

// ----------------------------------------------------------------------------
// /api/v2/contexts API

// Enum for all match types - using bitmask to track multiple matches
typedef enum {
    SEARCH_MATCH_NONE = 0,
    SEARCH_MATCH_CONTEXT_ID = (1 << 0),
    SEARCH_MATCH_CONTEXT_TITLE = (1 << 1),
    SEARCH_MATCH_CONTEXT_UNITS = (1 << 2),
    SEARCH_MATCH_CONTEXT_FAMILY = (1 << 3),
    SEARCH_MATCH_INSTANCE = (1 << 4),
    SEARCH_MATCH_DIMENSION = (1 << 5),
    SEARCH_MATCH_LABEL = (1 << 6),
} SEARCH_MATCH_TYPE;

struct function_v2_entry {
    size_t size;
    size_t used;
    size_t *node_ids;
    STRING *help;
    STRING *tags;
    HTTP_ACCESS access;
    int priority;
    uint32_t version;
};

struct context_v2_entry {
    size_t count;
    STRING *id; // DO NOT FREE THIS, IT IS NOT DUP'd
    STRING *title;
    STRING *family;
    STRING *units;
    uint32_t priority;
    time_t first_time_s;
    time_t last_time_s;
    size_t nodes;
    size_t instances;
    RRD_FLAGS flags;
    DICTIONARY *instances_dict;
    DICTIONARY *dimensions_dict;
    RRDLABELS_AGGREGATED *labels_aggregated;

    RRDCONTEXT *rc; // THIS IS TEMPORARY, WHILE REFERENCED, NOT TO BE CLEANED
    
    // For search results
    SEARCH_MATCH_TYPE matched_types;  // Bitmask of all match types
    DICTIONARY *matched_instances;
    DICTIONARY *matched_dimensions;
    RRDLABELS_AGGREGATED *matched_labels;
};

struct category_entry {
    size_t count;
    DICTIONARY *contexts; // Dictionary to hold context names
};

static void rrdcontext_categorize_and_output(BUFFER *wb, DICTIONARY *contexts_dict, size_t cardinality_limit) {
    size_t total_contexts = dictionary_entries(contexts_dict);
    
    // First pass: count categories
    DICTIONARY *categories = dictionary_create(DICT_OPTION_SINGLE_THREADED);
    
    struct context_v2_entry *z;
    dfe_start_read(contexts_dict, z) {
        const char *context_name = string2str(z->id);
        char category[256];
        const char *first_dot = strchr(context_name, '.');
        if (first_dot) {
            const char *second_dot = strchr(first_dot + 1, '.');
            if (second_dot) {
                // Use up to second dot as category
                size_t prefix_len = second_dot - context_name;
                if (prefix_len > sizeof(category) - 1) prefix_len = sizeof(category) - 1;
                memcpy(category, context_name, prefix_len);
                category[prefix_len] = '\0';
            } else {
                // Only one dot, use up to first dot
                size_t prefix_len = first_dot - context_name;
                if (prefix_len > sizeof(category) - 1) prefix_len = sizeof(category) - 1;
                memcpy(category, context_name, prefix_len);
                category[prefix_len] = '\0';
            }
        } else {
            strncpyz(category, context_name, sizeof(category) - 1);
        }
        
        struct category_entry *entry = dictionary_get(categories, category);
        if (!entry) {
            struct category_entry new_entry = {0};
            new_entry.contexts = dictionary_create(DICT_OPTION_SINGLE_THREADED);
            entry = dictionary_set(categories, category, &new_entry, sizeof(struct category_entry));
        }
        entry->count++;
        // Store the context name for sampling
        dictionary_set(entry->contexts, context_name, NULL, 0);
    }
    dfe_done(z);
    
    // Calculate how many samples per category
    size_t num_categories = dictionary_entries(categories);
    size_t samples_per_category = 3; // Default to 3
    if (num_categories > 0 && cardinality_limit > 0) {
        samples_per_category = cardinality_limit / num_categories;
        if (samples_per_category < 3) samples_per_category = 3;
    }
    
    // Add info object first
    buffer_json_member_add_object(wb, "__info__");
    buffer_json_member_add_string(wb, "status", "categorized");
    buffer_json_member_add_uint64(wb, "total_contexts", total_contexts);
    buffer_json_member_add_uint64(wb, "categories", num_categories);
    buffer_json_member_add_uint64(wb, "samples_per_category", samples_per_category);
    buffer_json_member_add_string(wb, "help", "Results grouped by category with samples. Use 'metrics' parameter with specific patterns like 'system.*' to get full details for a category.");
    buffer_json_object_close(wb);
    
    // Output categorized contexts
    struct category_entry *cat_entry;
    dfe_start_read(categories, cat_entry) {
        buffer_json_member_add_array(wb, cat_entry_dfe.name);
        
        size_t samples_shown = 0;
        void *ctx_name;
        dfe_start_read(cat_entry->contexts, ctx_name) {
            // Only show samples_per_category - 1 if we need to add "... more"
            size_t max_to_show = (cat_entry->count > samples_per_category) ? samples_per_category - 1 : cat_entry->count;
            if (samples_shown < max_to_show) {
                buffer_json_add_array_item_string(wb, ctx_name_dfe.name);
                samples_shown++;
            } else {
                break;
            }
        }
        dfe_done(ctx_name);
        
        // Add "... and X more" only if we have more contexts than the limit
        if (cat_entry->count > samples_per_category) {
            char msg[100];
            snprintf(msg, sizeof(msg), "... and %zu more", cat_entry->count - samples_shown);
            buffer_json_add_array_item_string(wb, msg);
        }
        
        buffer_json_array_close(wb);
    }
    dfe_done(cat_entry);
    
    // Cleanup
    dfe_start_write(categories, cat_entry) {
        dictionary_destroy(cat_entry->contexts);
    }
    dfe_done(cat_entry);
    dictionary_destroy(categories);
}

static inline bool full_text_search_string(FTS_INDEX *fts, SIMPLE_PATTERN *q, STRING *ptr) {
    fts->searches++;
    fts->string_searches++;
    return simple_pattern_matches_string(q, ptr);
}

static inline bool full_text_search_char(FTS_INDEX *fts, SIMPLE_PATTERN *q, char *ptr) {
    fts->searches++;
    fts->char_searches++;
    return simple_pattern_matches(q, ptr);
}

// Structure to hold search results
struct fts_search_results {
    SEARCH_MATCH_TYPE matched_types;  // Bitmask of all match types
    DICTIONARY *matched_instances;
    DICTIONARY *matched_dimensions;
    RRDLABELS_AGGREGATED *matched_labels;
};

static void rrdcontext_to_json_v2_full_text_search(struct rrdcontext_to_json_v2_data *ctl, RRDCONTEXT *rc, SIMPLE_PATTERN *q, struct fts_search_results *results) {
    // Initialize results
    results->matched_types = SEARCH_MATCH_NONE;
    results->matched_instances = NULL;
    results->matched_dimensions = NULL;
    results->matched_labels = NULL;
    
    // Check context-level matches
    // Always search ID - it's the primary identifier
    if(unlikely(full_text_search_string(&ctl->q.fts, q, rc->id))) {
        results->matched_types |= SEARCH_MATCH_CONTEXT_ID;
    }
    
    // Only search family if the option is enabled
    if((ctl->options & CONTEXTS_OPTION_FAMILY) && unlikely(full_text_search_string(&ctl->q.fts, q, rc->family))) {
        results->matched_types |= SEARCH_MATCH_CONTEXT_FAMILY;
    }
    
    // Only search title if the option is enabled
    if((ctl->options & CONTEXTS_OPTION_TITLES) && unlikely(full_text_search_string(&ctl->q.fts, q, rc->title))) {
        results->matched_types |= SEARCH_MATCH_CONTEXT_TITLE;
    }
    
    // Only search units if the option is enabled
    if((ctl->options & CONTEXTS_OPTION_UNITS) && unlikely(full_text_search_string(&ctl->q.fts, q, rc->units))) {
        results->matched_types |= SEARCH_MATCH_CONTEXT_UNITS;
    }

    
    RRDINSTANCE *ri;
    dfe_start_read(rc->rrdinstances, ri) {
        if(ctl->window.enabled && !query_matches_retention(ctl->window.after, ctl->window.before, ri->first_time_s, (ri->flags & RRD_FLAG_COLLECTED) ? ctl->now : ri->last_time_s, 0))
            continue;

        // Check instance name match only if instances option is enabled
        if((ctl->options & CONTEXTS_OPTION_INSTANCES) && 
           (unlikely(full_text_search_string(&ctl->q.fts, q, ri->id)) ||
            (ri->name != ri->id && full_text_search_string(&ctl->q.fts, q, ri->name)))) {
            if(!results->matched_instances)
                results->matched_instances = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE, NULL, 0);
            dictionary_set(results->matched_instances, string2str(ri->name), NULL, 0);
            results->matched_types |= SEARCH_MATCH_INSTANCE;
        }

        // Check dimensions only if dimensions option is enabled
        if(ctl->options & CONTEXTS_OPTION_DIMENSIONS) {
            RRDMETRIC *rm;
            dfe_start_read(ri->rrdmetrics, rm) {
                if(ctl->window.enabled && !query_matches_retention(ctl->window.after, ctl->window.before, rm->first_time_s, (rm->flags & RRD_FLAG_COLLECTED) ? ctl->now : rm->last_time_s, 0))
                    continue;

                if(unlikely(full_text_search_string(&ctl->q.fts, q, rm->id)) ||
                   (rm->name != rm->id && full_text_search_string(&ctl->q.fts, q, rm->name))) {
                    if(!results->matched_dimensions)
                        results->matched_dimensions = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE, NULL, 0);
                    dictionary_set(results->matched_dimensions, string2str(rm->name), NULL, 0);
                    results->matched_types |= SEARCH_MATCH_DIMENSION;
                }
            }
            dfe_done(rm);
        }

        // Check labels only if labels option is enabled
        if(ctl->options & CONTEXTS_OPTION_LABELS) {
            size_t label_searches = 0;
            RRDLABELS *labels = rrdinstance_labels(ri);
            if(unlikely(rrdlabels_entries(labels))) {
                results->matched_labels = rrdlabels_full_text_search(labels, q, results->matched_labels, &label_searches);
                
                if(results->matched_labels) {
                    results->matched_types |= SEARCH_MATCH_LABEL;
                }
                
                ctl->q.fts.searches += label_searches;
                ctl->q.fts.char_searches += label_searches;
            }
        }

        // We don't check alerts anymore as they are less relevant for the search
    }
    dfe_done(ri);
}

static ssize_t rrdcontext_to_json_v2_add_context(void *data, RRDCONTEXT_ACQUIRED *rca, bool queryable_context __maybe_unused) {
    struct rrdcontext_to_json_v2_data *ctl = data;

    RRDCONTEXT *rc = rrdcontext_acquired_value(rca);

    if(ctl->window.enabled && !query_matches_retention(ctl->window.after, ctl->window.before, rc->first_time_s, (rc->flags & RRD_FLAG_COLLECTED) ? ctl->now : rc->last_time_s, 0))
        return 0; // continue to next context

    struct fts_search_results search_results = {0};
    
    if((ctl->mode & CONTEXTS_V2_SEARCH) && ctl->q.pattern) {
        rrdcontext_to_json_v2_full_text_search(ctl, rc, ctl->q.pattern, &search_results);

        if(search_results.matched_types == SEARCH_MATCH_NONE)
            return 0; // continue to next context
    }

    if(ctl->mode & CONTEXTS_V2_ALERTS) {
        if(!rrdcontext_matches_alert(ctl, rc))
            return 0; // continue to next context
    }

    if(ctl->contexts.dict) {
        struct context_v2_entry t = {
            .count = 1,
            .id = rc->id,
            .title = string_dup(rc->title),
            .family = string_dup(rc->family),
            .units = string_dup(rc->units),
            .priority = rc->priority,
            .first_time_s = rc->first_time_s,
            .last_time_s = rc->last_time_s,
            .flags = rc->flags,
            .nodes = 1,
            .instances = dictionary_entries(rc->rrdinstances),
            .instances_dict = NULL,
            .dimensions_dict = NULL,
            .rc = rc,
            // Store search results
            .matched_types = search_results.matched_types,
            .matched_instances = search_results.matched_instances,
            .matched_dimensions = search_results.matched_dimensions,
            .matched_labels = search_results.matched_labels,
        };

        dictionary_set(ctl->contexts.dict, string2str(rc->id), &t, sizeof(struct context_v2_entry));
    }

    return 1;
}

void buffer_json_node_add_v2_mcp(BUFFER *wb, RRDHOST *host, size_t ni __maybe_unused) {
    buffer_json_member_add_string(wb, "machine_guid", host->machine_guid);

    if(!UUIDiszero(host->node_id))
        buffer_json_member_add_uuid(wb, "node_id", host->node_id.uuid);

    buffer_json_member_add_string(wb, "hostname", rrdhost_hostname(host));

    buffer_json_member_add_string(wb, "relationship",
                                  host == localhost ? "localhost" :
                                  (rrdhost_is_virtual(host) ? "virtual" : "child"));

    buffer_json_member_add_boolean(wb, "connected", rrdhost_is_online(host));
}

static void rrdhost_receiver_to_json(BUFFER *wb, RRDHOST_STATUS *s, const char *key, CONTEXTS_OPTIONS options) {
    buffer_json_member_add_object(wb, key);
    {
        buffer_json_member_add_uint64(wb, "id", s->ingest.id);
        buffer_json_member_add_int64(wb, "hops", s->ingest.hops);
        buffer_json_member_add_string(wb, "type", rrdhost_ingest_type_to_string(s->ingest.type));
        buffer_json_member_add_string(wb, "status", rrdhost_ingest_status_to_string(s->ingest.status));
        buffer_json_member_add_time_t_formatted(wb, "since", s->ingest.since, options & CONTEXTS_OPTION_RFC3339);
        buffer_json_member_add_time_t(wb, "age", s->now - s->ingest.since);
        buffer_json_member_add_uint64(wb, "metrics", s->ingest.collected.metrics);
        buffer_json_member_add_uint64(wb, "instances", s->ingest.collected.instances);
        buffer_json_member_add_uint64(wb, "contexts", s->ingest.collected.contexts);

        if(s->ingest.type == RRDHOST_INGEST_TYPE_CHILD) {
            if(s->ingest.status == RRDHOST_INGEST_STATUS_OFFLINE)
                buffer_json_member_add_string(wb, "reason", stream_handshake_error_to_string(s->ingest.reason));

            if(s->ingest.status == RRDHOST_INGEST_STATUS_REPLICATING) {
                buffer_json_member_add_object(wb, "replication");
                {
                    buffer_json_member_add_boolean(wb, "in_progress", s->ingest.replication.in_progress);
                    buffer_json_member_add_double(wb, "completion", s->ingest.replication.completion);
                    buffer_json_member_add_uint64(wb, "instances", s->ingest.replication.instances);
                }
                buffer_json_object_close(wb); // replication
            }

            if(s->ingest.status == RRDHOST_INGEST_STATUS_REPLICATING || s->ingest.status == RRDHOST_INGEST_STATUS_ONLINE) {
                buffer_json_member_add_object(wb, "source");
                {
                    char buf[1024 + 1];
                    snprintfz(buf, sizeof(buf) - 1, "[%s]:%d%s", s->ingest.peers.local.ip, s->ingest.peers.local.port, s->ingest.ssl ? ":SSL" : "");
                    buffer_json_member_add_string(wb, "local", buf);

                    snprintfz(buf, sizeof(buf) - 1, "[%s]:%d%s", s->ingest.peers.peer.ip, s->ingest.peers.peer.port, s->ingest.ssl ? ":SSL" : "");
                    buffer_json_member_add_string(wb, "remote", buf);

                    stream_capabilities_to_json_array(wb, s->ingest.capabilities, "capabilities");
                }
                buffer_json_object_close(wb); // source
            }
        }
    }
    buffer_json_object_close(wb); // collection
}

static void rrdhost_sender_to_json(BUFFER *wb, RRDHOST_STATUS *s, const char *key, CONTEXTS_OPTIONS options) {
    if(s->stream.status == RRDHOST_STREAM_STATUS_DISABLED)
        return;

    buffer_json_member_add_object(wb, key);
    {
        buffer_json_member_add_uint64(wb, "id", s->stream.id);
        buffer_json_member_add_uint64(wb, "hops", s->stream.hops);
        buffer_json_member_add_string(wb, "status", rrdhost_streaming_status_to_string(s->stream.status));
        buffer_json_member_add_time_t_formatted(wb, "since", s->stream.since, options & CONTEXTS_OPTION_RFC3339);
        buffer_json_member_add_time_t(wb, "age", s->now - s->stream.since);

        if (s->stream.status == RRDHOST_STREAM_STATUS_OFFLINE)
            buffer_json_member_add_string(wb, "reason", stream_handshake_error_to_string(s->stream.reason));

        buffer_json_member_add_object(wb, "replication");
        {
            buffer_json_member_add_boolean(wb, "in_progress", s->stream.replication.in_progress);
            buffer_json_member_add_double(wb, "completion", s->stream.replication.completion);
            buffer_json_member_add_uint64(wb, "instances", s->stream.replication.instances);
        }
        buffer_json_object_close(wb); // replication

        buffer_json_member_add_object(wb, "destination");
        {
            char buf[1024 + 1];
            snprintfz(buf, sizeof(buf) - 1, "[%s]:%d%s", s->stream.peers.local.ip, s->stream.peers.local.port, s->stream.ssl ? ":SSL" : "");
            buffer_json_member_add_string(wb, "local", buf);

            snprintfz(buf, sizeof(buf) - 1, "[%s]:%d%s", s->stream.peers.peer.ip, s->stream.peers.peer.port, s->stream.ssl ? ":SSL" : "");
            buffer_json_member_add_string(wb, "remote", buf);

            stream_capabilities_to_json_array(wb, s->stream.capabilities, "capabilities");

            buffer_json_member_add_object(wb, "traffic");
            {
                buffer_json_member_add_boolean(wb, "compression", s->stream.compression);
                buffer_json_member_add_uint64(wb, "data", s->stream.sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_DATA]);
                buffer_json_member_add_uint64(wb, "metadata", s->stream.sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_METADATA]);
                buffer_json_member_add_uint64(wb, "functions", s->stream.sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_FUNCTIONS]);
                buffer_json_member_add_uint64(wb, "replication", s->stream.sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_REPLICATION]);
            }
            buffer_json_object_close(wb); // traffic

            buffer_json_member_add_array(wb, "parents");
            rrdhost_stream_parents_to_json(wb, s);
            buffer_json_array_close(wb); // parents

            rrdhost_stream_path_to_json(wb, s->host, STREAM_PATH_JSON_MEMBER, false);
        }
        buffer_json_object_close(wb); // destination
    }
    buffer_json_object_close(wb); // streaming
}

void agent_capabilities_to_json(BUFFER *wb, RRDHOST *host, const char *key) {
    buffer_json_member_add_array(wb, key);

    struct capability *capas = aclk_get_node_instance_capas(host);
    for(struct capability *capa = capas; capa->name ;capa++) {
        buffer_json_add_array_item_object(wb);
        {
            buffer_json_member_add_string(wb, "name", capa->name);
            buffer_json_member_add_uint64(wb, "version", capa->version);
            buffer_json_member_add_boolean(wb, "enabled", capa->enabled);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_array_close(wb);
    freez(capas);
}

static inline void host_dyncfg_to_json_v2(BUFFER *wb, const char *key, RRDHOST_STATUS *s) {
    buffer_json_member_add_object(wb, key);
    {
        buffer_json_member_add_string(wb, "status", rrdhost_dyncfg_status_to_string(s->dyncfg.status));
    }
    buffer_json_object_close(wb); // health

}

static inline void rrdhost_health_to_json_v2(BUFFER *wb, const char *key, RRDHOST_STATUS *s) {
    buffer_json_member_add_object(wb, key);
    {
        buffer_json_member_add_string(wb, "status", rrdhost_health_status_to_string(s->health.status));
        if (s->health.status == RRDHOST_HEALTH_STATUS_RUNNING || s->health.status == RRDHOST_HEALTH_STATUS_INITIALIZING) {
            buffer_json_member_add_object(wb, "alerts");
            {
                buffer_json_member_add_uint64(wb, "critical", s->health.alerts.critical);
                buffer_json_member_add_uint64(wb, "warning", s->health.alerts.warning);
                buffer_json_member_add_uint64(wb, "clear", s->health.alerts.clear);
                buffer_json_member_add_uint64(wb, "undefined", s->health.alerts.undefined);
                buffer_json_member_add_uint64(wb, "uninitialized", s->health.alerts.uninitialized);
            }
            buffer_json_object_close(wb); // alerts
        }
    }
    buffer_json_object_close(wb); // health
}

static void rrdcontext_to_json_v2_rrdhost(BUFFER *wb, RRDHOST *host, struct rrdcontext_to_json_v2_data *ctl, size_t node_id) {
    buffer_json_add_array_item_object(wb); // this node

    if(ctl->options & CONTEXTS_OPTION_MCP)
        buffer_json_node_add_v2_mcp(wb, host, node_id);
    else
        buffer_json_node_add_v2(wb, host, node_id, 0,
                            (ctl->mode & CONTEXTS_V2_AGENTS) && !(ctl->mode & CONTEXTS_V2_NODE_INSTANCES));

    if(ctl->mode & (CONTEXTS_V2_NODES_INFO | CONTEXTS_V2_NODES_STREAM_PATH | CONTEXTS_V2_NODE_INSTANCES)) {
        RRDHOST_STATUS s;
        rrdhost_status(host, ctl->now, &s, RRDHOST_STATUS_ALL);

        if (ctl->mode & (CONTEXTS_V2_NODES_INFO | CONTEXTS_V2_NODES_STREAM_PATH)) {
            buffer_json_member_add_string(wb, "v", rrdhost_program_version(host));

            host_labels2json(host, wb, "labels");
            rrdhost_system_info_to_json_v2(wb, host->system_info);

            // created      - the node is created but never connected to cloud
            // unreachable  - not currently connected
            // stale        - connected but not having live data
            // reachable    - connected with live data
            // pruned       - not connected for some time and has been removed
            buffer_json_member_add_string(wb, "state", rrdhost_is_online(host) ? "reachable" : "stale");
        }

        if (ctl->mode & (CONTEXTS_V2_NODES_INFO)) {
            rrdhost_health_to_json_v2(wb, "health", &s);
            agent_capabilities_to_json(wb, host, "capabilities");
        }

        if (ctl->mode & (CONTEXTS_V2_NODES_STREAM_PATH)) {
            rrdhost_stream_path_to_json(wb, host, STREAM_PATH_JSON_MEMBER, false);
        }

        if (ctl->mode & (CONTEXTS_V2_NODE_INSTANCES)) {
            buffer_json_member_add_array(wb, "instances");
            buffer_json_add_array_item_object(wb); // this instance
            {
                buffer_json_agent_status_id(wb, 0, 0);

                buffer_json_member_add_object(wb, "db");
                {
                    buffer_json_member_add_string(wb, "status", rrdhost_db_status_to_string(s.db.status));
                    buffer_json_member_add_string(wb, "liveness", rrdhost_db_liveness_to_string(s.db.liveness));
                    buffer_json_member_add_string(wb, "mode", rrd_memory_mode_name(s.db.mode));
                    buffer_json_member_add_time_t_formatted(wb, "first_time", s.db.first_time_s, ctl->options & CONTEXTS_OPTION_RFC3339);
                    buffer_json_member_add_time_t_formatted(wb, "last_time", s.db.last_time_s, ctl->options & CONTEXTS_OPTION_RFC3339);

                    buffer_json_member_add_uint64(wb, "metrics", s.db.metrics);
                    buffer_json_member_add_uint64(wb, "instances", s.db.instances);
                    buffer_json_member_add_uint64(wb, "contexts", s.db.contexts);
                }
                buffer_json_object_close(wb);

                rrdhost_receiver_to_json(wb, &s, "ingest", ctl->options);
                rrdhost_sender_to_json(wb, &s, "stream", ctl->options);

                buffer_json_member_add_object(wb, "ml");
                buffer_json_member_add_string(wb, "status", rrdhost_ml_status_to_string(s.ml.status));
                buffer_json_member_add_string(wb, "type", rrdhost_ml_type_to_string(s.ml.type));
                if (s.ml.status == RRDHOST_ML_STATUS_RUNNING) {
                    buffer_json_member_add_object(wb, "metrics");
                    {
                        buffer_json_member_add_uint64(wb, "anomalous", s.ml.metrics.anomalous);
                        buffer_json_member_add_uint64(wb, "normal", s.ml.metrics.normal);
                        buffer_json_member_add_uint64(wb, "trained", s.ml.metrics.trained);
                        buffer_json_member_add_uint64(wb, "pending", s.ml.metrics.pending);
                        buffer_json_member_add_uint64(wb, "silenced", s.ml.metrics.silenced);
                    }
                    buffer_json_object_close(wb); // metrics
                }
                buffer_json_object_close(wb); // ml

                rrdhost_health_to_json_v2(wb, "health", &s);

                host_functions2json(host, wb); // functions
                agent_capabilities_to_json(wb, host, "capabilities");

                host_dyncfg_to_json_v2(wb, "dyncfg", &s);
            }
            buffer_json_object_close(wb); // this instance
            buffer_json_array_close(wb); // instances
        }
    }
    buffer_json_object_close(wb); // this node
}

static ssize_t rrdcontext_to_json_v2_add_host(void *data, RRDHOST *host, bool queryable_host) {
    if(!queryable_host || !host->rrdctx.contexts)
        // the host matches the 'scope_host' but does not match the 'host' patterns
        // or the host does not have any contexts
        return 0; // continue to next host

    struct rrdcontext_to_json_v2_data *ctl = data;

    if(ctl->window.enabled && !rrdhost_matches_window(host, ctl->window.after, ctl->window.before, ctl->now))
        // the host does not have data in the requested window
        return 0; // continue to next host

    if(ctl->request->timeout_ms && now_monotonic_usec() > ctl->timings.received_ut + ctl->request->timeout_ms * USEC_PER_MS)
        // timed out
        return -2; // stop the query

    if(ctl->request->interrupt_callback && ctl->request->interrupt_callback(ctl->request->interrupt_callback_data))
        // interrupted
        return -1; // stop the query

    bool host_matched = (ctl->mode & (CONTEXTS_V2_NODES | CONTEXTS_V2_FUNCTIONS | CONTEXTS_V2_ALERTS)) && !ctl->contexts.pattern && !ctl->contexts.scope_pattern && !ctl->window.enabled;
    bool do_contexts = (ctl->mode & (CONTEXTS_V2_CONTEXTS | CONTEXTS_V2_SEARCH | CONTEXTS_V2_ALERTS)) || ctl->contexts.pattern || ctl->contexts.scope_pattern;

    if(do_contexts) {
        ssize_t added = query_scope_foreach_context(
                host, ctl->request->scope_contexts,
                ctl->contexts.scope_pattern, ctl->contexts.pattern,
                rrdcontext_to_json_v2_add_context, queryable_host, ctl);

        if(unlikely(added < 0))
            return -1; // stop the query

        if(added)
            host_matched = true;
    }
    else if(!host_matched && ctl->window.enabled) {
        time_t first_time_s = host->retention.first_time_s;
        time_t last_time_s = host->retention.last_time_s;
        if(rrdhost_is_online(host))
            last_time_s = ctl->now; // if the host is online, use the current time as the last time

        if(query_matches_retention(ctl->window.after, ctl->window.before, first_time_s, last_time_s, 0))
            host_matched = true;
    }

    if(!host_matched)
        return 0;

    if(ctl->mode & CONTEXTS_V2_FUNCTIONS) {
        struct function_v2_entry t = {
            .used = 1,
            .size = 1,
            .node_ids = &ctl->nodes.ni,
            .help = NULL,
            .tags = NULL,
            .access = HTTP_ACCESS_ALL,
            .priority = RRDFUNCTIONS_PRIORITY_DEFAULT,
            .version = RRDFUNCTIONS_VERSION_DEFAULT,
        };
        host_functions_to_dict(host, ctl->functions.dict, &t, sizeof(t), &t.help, &t.tags, &t.access, &t.priority, &t.version);
    }

    if(ctl->mode & (CONTEXTS_V2_NODES | CONTEXTS_V2_FUNCTIONS | CONTEXTS_V2_ALERTS)) {
        struct contexts_v2_node t = {
            .ni = ctl->nodes.ni++,
            .host = host,
        };

        dictionary_set(ctl->nodes.dict, host->machine_guid, &t, sizeof(struct contexts_v2_node));
    }

    return 1;
}

static void buffer_json_contexts_v2_mode_to_array(BUFFER *wb, const char *key, CONTEXTS_V2_MODE mode) {
    buffer_json_member_add_array(wb, key);

    if(mode & CONTEXTS_V2_VERSIONS)
        buffer_json_add_array_item_string(wb, "versions");

    if(mode & CONTEXTS_V2_AGENTS)
        buffer_json_add_array_item_string(wb, "agents");

    if(mode & CONTEXTS_V2_AGENTS_INFO)
        buffer_json_add_array_item_string(wb, "agents-info");

    if(mode & CONTEXTS_V2_NODES)
        buffer_json_add_array_item_string(wb, "nodes");

    if(mode & CONTEXTS_V2_NODES_INFO)
        buffer_json_add_array_item_string(wb, "nodes-info");

    if(mode & CONTEXTS_V2_NODES_STREAM_PATH)
        buffer_json_add_array_item_string(wb, "nodes-stream-path");

    if(mode & CONTEXTS_V2_NODE_INSTANCES)
        buffer_json_add_array_item_string(wb, "nodes-instances");

    if(mode & CONTEXTS_V2_CONTEXTS)
        buffer_json_add_array_item_string(wb, "contexts");

    if(mode & CONTEXTS_V2_SEARCH)
        buffer_json_add_array_item_string(wb, "search");

    if(mode & CONTEXTS_V2_ALERTS)
        buffer_json_add_array_item_string(wb, "alerts");

    if(mode & CONTEXTS_V2_ALERT_TRANSITIONS)
        buffer_json_add_array_item_string(wb, "alert_transitions");

    buffer_json_array_close(wb);
}

void buffer_json_query_timings(BUFFER *wb, const char *key, struct query_timings *timings) {
    timings->finished_ut = now_monotonic_usec();
    if(!timings->executed_ut)
        timings->executed_ut = timings->finished_ut;
    if(!timings->preprocessed_ut)
        timings->preprocessed_ut = timings->received_ut;
    buffer_json_member_add_object(wb, key);
    buffer_json_member_add_double(wb, "prep_ms", (NETDATA_DOUBLE)(timings->preprocessed_ut - timings->received_ut) / USEC_PER_MS);
    buffer_json_member_add_double(wb, "query_ms", (NETDATA_DOUBLE)(timings->executed_ut - timings->preprocessed_ut) / USEC_PER_MS);
    buffer_json_member_add_double(wb, "output_ms", (NETDATA_DOUBLE)(timings->finished_ut - timings->executed_ut) / USEC_PER_MS);
    buffer_json_member_add_double(wb, "total_ms", (NETDATA_DOUBLE)(timings->finished_ut - timings->received_ut) / USEC_PER_MS);
    buffer_json_member_add_double(wb, "cloud_ms", (NETDATA_DOUBLE)(timings->finished_ut - timings->received_ut) / USEC_PER_MS);
    buffer_json_object_close(wb);
}

void buffer_json_cloud_timings(BUFFER *wb, const char *key, struct query_timings *timings) {
    if(!timings->finished_ut)
        timings->finished_ut = now_monotonic_usec();

    buffer_json_member_add_object(wb, key);
    buffer_json_member_add_double(wb, "routing_ms", 0.0);
    buffer_json_member_add_double(wb, "node_max_ms", 0.0);
    buffer_json_member_add_double(wb, "total_ms", (NETDATA_DOUBLE)(timings->finished_ut - timings->received_ut) / USEC_PER_MS);
    buffer_json_object_close(wb);
}

static void functions_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    struct function_v2_entry *t = value;

    // it is initialized with a static reference - we need to mallocz() the array
    size_t *v = t->node_ids;
    t->node_ids = mallocz(sizeof(size_t));
    *t->node_ids = *v;
    t->size = 1;
    t->used = 1;
}

static bool functions_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value, void *data __maybe_unused) {
    struct function_v2_entry *t = old_value, *n = new_value;
    size_t *v = n->node_ids;

    if(t->used >= t->size) {
        t->node_ids = reallocz(t->node_ids, t->size * 2 * sizeof(size_t));
        t->size *= 2;
    }

    t->node_ids[t->used++] = *v;

    return true;
}

static void functions_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    struct function_v2_entry *t = value;
    freez(t->node_ids);
}

static void contexts_cleanup(struct context_v2_entry *n) {
    string_freez(n->title);
    string_freez(n->units);
    string_freez(n->family);

    dictionary_destroy(n->instances_dict);
    dictionary_destroy(n->dimensions_dict);
    rrdlabels_aggregated_destroy(n->labels_aggregated);

    // Clean up n's search results
    dictionary_destroy(n->matched_instances);
    dictionary_destroy(n->matched_dimensions);
    rrdlabels_aggregated_destroy(n->matched_labels);
}

static bool contexts_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value, void *data __maybe_unused) {
    struct rrdcontext_to_json_v2_data *ctl = data;
    struct context_v2_entry *o = old_value;
    struct context_v2_entry *n = new_value;

    o->count++;

    o->flags |= n->flags;
    o->nodes += n->nodes;
    o->instances += n->instances;

    if (ctl->options & CONTEXTS_OPTION_TITLES) {
        if(o->title != n->title) {
            if((o->flags & RRD_FLAG_COLLECTED) && !(n->flags & RRD_FLAG_COLLECTED))
                // keep old
                    ;
            else if(!(o->flags & RRD_FLAG_COLLECTED) && (n->flags & RRD_FLAG_COLLECTED)) {
                // keep new
                SWAP(o->title, n->title);
            }
            else {
                // merge
                STRING *old_title = o->title;
                o->title = string_2way_merge(o->title, n->title);
                string_freez(old_title);
                // n->title will be freed below
            }
        }
    }

    if (ctl->options & CONTEXTS_OPTION_FAMILY) {
        if(o->family != n->family) {
            if((o->flags & RRD_FLAG_COLLECTED) && !(n->flags & RRD_FLAG_COLLECTED))
                // keep old
                    ;
            else if(!(o->flags & RRD_FLAG_COLLECTED) && (n->flags & RRD_FLAG_COLLECTED)) {
                // keep new
                SWAP(o->family, n->family);
            }
            else {
                // merge
                STRING *old_family = o->family;
                o->family = string_2way_merge(o->family, n->family);
                string_freez(old_family);
                // n->family will be freed below
            }
        }
    }

    if (ctl->options & CONTEXTS_OPTION_UNITS) {
        if(o->units != n->units) {
            if((o->flags & RRD_FLAG_COLLECTED) && !(n->flags & RRD_FLAG_COLLECTED))
                // keep old
                    ;
            else if(!(o->flags & RRD_FLAG_COLLECTED) && (n->flags & RRD_FLAG_COLLECTED)) {
                // keep new
                SWAP(o->units, n->units);
            }
            else {
                // keep old
                ;
            }
        }
    }

    if (ctl->options & CONTEXTS_OPTION_PRIORITIES) {
        if(o->priority != n->priority) {
            if((o->flags & RRD_FLAG_COLLECTED) && !(n->flags & RRD_FLAG_COLLECTED))
                // keep o
                    ;
            else if(!(o->flags & RRD_FLAG_COLLECTED) && (n->flags & RRD_FLAG_COLLECTED))
                // keep n
                    o->priority = n->priority;
            else
                // keep the min
                    o->priority = MIN(o->priority, n->priority);
        }
    }

    if (ctl->options & CONTEXTS_OPTION_RETENTION) {
        if(o->first_time_s && n->first_time_s)
            o->first_time_s = MIN(o->first_time_s, n->first_time_s);
        else if(!o->first_time_s)
            o->first_time_s = n->first_time_s;

        if(o->last_time_s && n->last_time_s)
            o->last_time_s = MAX(o->last_time_s, n->last_time_s);
        else if(!o->last_time_s)
            o->last_time_s = n->last_time_s;
    }

    if (ctl->mode & CONTEXTS_V2_SEARCH) {
        // Merge search results
        o->matched_types |= n->matched_types;

        // For search result dictionaries, we need to merge them
        if(n->matched_instances && !o->matched_instances) {
            SWAP(o->matched_instances, n->matched_instances);
        }
        else if(n->matched_instances && o->matched_instances) {
            // Merge entries from n to o
            void *entry;
            dfe_start_read(n->matched_instances, entry) {
                dictionary_set(o->matched_instances, entry_dfe.name, NULL, 0);
            }
            dfe_done(entry);
        }

        if(n->matched_dimensions && !o->matched_dimensions) {
            SWAP(o->matched_dimensions, n->matched_dimensions);
        }
        else if(n->matched_dimensions && o->matched_dimensions) {
            // Merge entries from n to o
            void *entry;
            dfe_start_read(n->matched_dimensions, entry) {
                dictionary_set(o->matched_dimensions, entry_dfe.name, NULL, 0);
            }
            dfe_done(entry);
        }

        if(n->matched_labels && !o->matched_labels) {
            SWAP(o->matched_labels, n->matched_labels);
        }
        else if(n->matched_labels && o->matched_labels) {
            // Merge n into o
            rrdlabels_aggregated_merge(o->matched_labels, n->matched_labels);
        }
    }

    contexts_cleanup(n);

    // for the react callback to use
    o->rc = n->rc;

    return true;
}

static void contexts_react_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data) {
    struct context_v2_entry *t = value;
    struct rrdcontext_to_json_v2_data *ctl = data;

    // Only populate dictionaries if they exist (meaning the options are enabled)
    if(!(ctl->mode & CONTEXTS_V2_CONTEXTS) || !(ctl->options & (CONTEXTS_OPTION_INSTANCES | CONTEXTS_OPTION_DIMENSIONS | CONTEXTS_OPTION_LABELS)))
        return;

    // Initialize dictionaries for the new features if the corresponding options are set
    if(ctl->options & CONTEXTS_OPTION_INSTANCES && !t->instances_dict) {
        t->instances_dict = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE, NULL, 0);
    }

    if(ctl->options & CONTEXTS_OPTION_DIMENSIONS && !t->dimensions_dict) {
        t->dimensions_dict = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE, NULL, 0);
    }

    if(ctl->options & CONTEXTS_OPTION_LABELS && !t->labels_aggregated) {
        t->labels_aggregated = rrdlabels_aggregated_create();
    }

    RRDCONTEXT *rc = t->rc;

    // Collect instances, dimensions, and labels if requested
    RRDINSTANCE *ri;
    dfe_start_read(rc->rrdinstances, ri) {
        if(ctl->window.enabled && !query_matches_retention(ctl->window.after, ctl->window.before, ri->first_time_s, (ri->flags & RRD_FLAG_COLLECTED) ? ctl->now : ri->last_time_s, (time_t)ri->update_every_s))
            continue;

        // Add instance name to instances dictionary
        if(t->instances_dict) {
            dictionary_set(t->instances_dict, string2str(ri->name), NULL, 0);
        }

        // Collect dimensions from this instance
        if(t->dimensions_dict) {
            RRDMETRIC *rm;
            dfe_start_read(ri->rrdmetrics, rm) {
                if(ctl->window.enabled && !query_matches_retention(ctl->window.after, ctl->window.before, rm->first_time_s, (rm->flags & RRD_FLAG_COLLECTED) ? ctl->now : rm->last_time_s, (time_t)ri->update_every_s))
                    continue;

                dictionary_set(t->dimensions_dict, string2str(rm->name), NULL, 0);
            }
            dfe_done(rm);
        }

        // Collect labels from this instance
        if(t->labels_aggregated) {
            RRDLABELS *labels = rrdinstance_labels(ri);
            rrdlabels_aggregated_add_from_rrdlabels(t->labels_aggregated, labels);
        }
    }
    dfe_done(ri);
}

static void contexts_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    struct context_v2_entry *z = value;
    contexts_cleanup(z);
}

static void contexts_v2_search_results_to_json(BUFFER *wb, struct rrdcontext_to_json_v2_data *ctl) {
    size_t contexts_count = 0;
    size_t contexts_limit = ctl->request->cardinality_limit;
    size_t total_contexts = dictionary_entries(ctl->contexts.dict);
    
    // Calculate per-context item limit: MIN(cardinality_limit/total_contexts, 3)
    // But use the contexts that will be shown, not total
    size_t contexts_to_show = (contexts_limit && total_contexts > contexts_limit) ? contexts_limit : total_contexts;
    size_t per_context_limit = 3;  // Default minimum
    if (contexts_limit && contexts_to_show > 0) {
        size_t calculated_limit = contexts_limit / contexts_to_show;
        if (calculated_limit > per_context_limit)
            per_context_limit = calculated_limit;
    }

    buffer_json_member_add_object(wb, "contexts");
    
    struct context_v2_entry *z;
    dfe_start_read(ctl->contexts.dict, z) {
        // Check if we've reached the limit
        if (contexts_limit && contexts_count >= contexts_limit) {
            // Add a special entry indicating truncation
            buffer_json_member_add_object(wb, "__truncated__");
            buffer_json_member_add_uint64(wb, "total_contexts", total_contexts);
            buffer_json_member_add_uint64(wb, "returned", contexts_count);
            buffer_json_member_add_uint64(wb, "remaining", total_contexts - contexts_count);
            buffer_json_object_close(wb);
            break;
        }

        buffer_json_member_add_object(wb, string2str(z->id));
        {
            // Always show title, family, units in search results for context
            if (z->matched_types & SEARCH_MATCH_CONTEXT_TITLE)
                buffer_json_member_add_string(wb, "title", string2str(z->title));

            if (z->matched_types & SEARCH_MATCH_CONTEXT_FAMILY)
                buffer_json_member_add_string(wb, "family", string2str(z->family));

            if (z->matched_types & SEARCH_MATCH_CONTEXT_UNITS)
                buffer_json_member_add_string(wb, "units", string2str(z->units));
            
            // Output what matched as an array
            if (!(ctl->options & CONTEXTS_OPTION_MCP)) {
                buffer_json_member_add_array(wb, "matched");
                if (z->matched_types & SEARCH_MATCH_CONTEXT_ID)
                    buffer_json_add_array_item_string(wb, "id");
                if (z->matched_types & SEARCH_MATCH_CONTEXT_TITLE)
                    buffer_json_add_array_item_string(wb, "title");
                if (z->matched_types & SEARCH_MATCH_CONTEXT_UNITS)
                    buffer_json_add_array_item_string(wb, "units");
                if (z->matched_types & SEARCH_MATCH_CONTEXT_FAMILY)
                    buffer_json_add_array_item_string(wb, "families");
                if (z->matched_types & SEARCH_MATCH_INSTANCE)
                    buffer_json_add_array_item_string(wb, "instances");
                if (z->matched_types & SEARCH_MATCH_DIMENSION)
                    buffer_json_add_array_item_string(wb, "dimensions");
                if (z->matched_types & SEARCH_MATCH_LABEL)
                    buffer_json_add_array_item_string(wb, "labels");
                buffer_json_array_close(wb);
            }

            // Add instances array if any matched
            if(z->matched_instances && dictionary_entries(z->matched_instances) > 0) {
                buffer_json_member_add_array(wb, "instances");
                void *entry;
                size_t count = 0;
                size_t total = dictionary_entries(z->matched_instances);
                
                dfe_start_read(z->matched_instances, entry) {
                    if (per_context_limit && total > per_context_limit && count >= per_context_limit - 1) {
                        char msg[100];
                        snprintf(msg, sizeof(msg), "... %zu instances more", total - count);
                        buffer_json_add_array_item_string(wb, msg);
                        break;
                    }
                    buffer_json_add_array_item_string(wb, entry_dfe.name);
                    count++;
                }
                dfe_done(entry);
                buffer_json_array_close(wb);
            }
            
            // Add dimensions array if any matched
            if(z->matched_dimensions && dictionary_entries(z->matched_dimensions) > 0) {
                buffer_json_member_add_array(wb, "dimensions");
                void *entry;
                size_t count = 0;
                size_t total = dictionary_entries(z->matched_dimensions);
                
                dfe_start_read(z->matched_dimensions, entry) {
                    if (per_context_limit && total > per_context_limit && count >= per_context_limit - 1) {
                        char msg[100];
                        snprintf(msg, sizeof(msg), "... %zu dimensions more", total - count);
                        buffer_json_add_array_item_string(wb, msg);
                        break;
                    }
                    buffer_json_add_array_item_string(wb, entry_dfe.name);
                    count++;
                }
                dfe_done(entry);
                buffer_json_array_close(wb);
            }
            
            // Add labels if any matched
            if(z->matched_labels) {
                rrdlabels_aggregated_to_buffer_json(
                    z->matched_labels, wb, "labels", per_context_limit);
            }
        }
        buffer_json_object_close(wb);
        
        contexts_count++;
    }
    dfe_done(z);

    buffer_json_object_close(wb); // contexts
    
    // Add info about cardinality limit if it was reached
    if (contexts_limit && total_contexts > contexts_limit && (ctl->options & CONTEXTS_OPTION_MCP)) {
        buffer_json_member_add_string(wb, "info", "Cardinality limit reached. Use cardinality_limit parameter to see more results.");
    }
}

static void contexts_v2_contexts_to_json(BUFFER *wb, struct rrdcontext_to_json_v2_data *ctl) {
    size_t contexts_count = 0;
    size_t contexts_limit = ctl->request->cardinality_limit;
    size_t total_contexts = dictionary_entries(ctl->contexts.dict);

    bool contexts_is_object = false;

    // If we have more contexts than the limit and MCP option is set, use categorized output
    if (contexts_limit && total_contexts > contexts_limit && (ctl->options & CONTEXTS_OPTION_MCP)) {
        buffer_json_member_add_object(wb, "contexts");
        rrdcontext_categorize_and_output(wb, ctl->contexts.dict, contexts_limit);
        buffer_json_object_close(wb);
        if (ctl->options & CONTEXTS_OPTION_MCP) {
            buffer_json_member_add_string(wb, "info", MCP_INFO_TOO_MANY_CONTEXTS_GROUPED_IN_CATEGORIES);
        }
    } else {
        if (ctl->options & (CONTEXTS_OPTION_TITLES | CONTEXTS_OPTION_FAMILY | CONTEXTS_OPTION_UNITS |
                           CONTEXTS_OPTION_PRIORITIES | CONTEXTS_OPTION_RETENTION | CONTEXTS_OPTION_LIVENESS |
                           CONTEXTS_OPTION_DIMENSIONS | CONTEXTS_OPTION_LABELS | CONTEXTS_OPTION_INSTANCES)) {
            contexts_is_object = true;
            buffer_json_member_add_object(wb, "contexts");
        } else
            buffer_json_member_add_array(wb, "contexts");

        struct context_v2_entry *z;
        dfe_start_read(ctl->contexts.dict, z) {
            // Check if we've reached the limit
            if (contexts_limit && contexts_count >= contexts_limit) {
                // Add a special entry indicating truncation
                if (contexts_is_object) {
                    buffer_json_member_add_object(wb, "__truncated__");
                    buffer_json_member_add_uint64(wb, "total_contexts", total_contexts);
                    buffer_json_member_add_uint64(wb, "returned", contexts_count);
                    buffer_json_member_add_uint64(wb, "remaining", total_contexts - contexts_count);
                    buffer_json_object_close(wb);
                } else {
                    char msg[100];
                    snprintf(msg, sizeof(msg), "... %zu contexts more", total_contexts - contexts_count);
                    buffer_json_add_array_item_string(wb, msg);
                }
                break;
            }

            bool collected = z->flags & RRD_FLAG_COLLECTED;

            if (contexts_is_object) {
                buffer_json_member_add_object(wb, string2str(z->id));
                {
                    if (ctl->options & CONTEXTS_OPTION_TITLES)
                        buffer_json_member_add_string(wb, "title", string2str(z->title));

                    if (ctl->options & CONTEXTS_OPTION_FAMILY)
                        buffer_json_member_add_string(wb, "family", string2str(z->family));

                    if (ctl->options & CONTEXTS_OPTION_UNITS)
                        buffer_json_member_add_string(wb, "units", string2str(z->units));

                    if (ctl->options & CONTEXTS_OPTION_PRIORITIES)
                        buffer_json_member_add_uint64(wb, "priority", z->priority);

                    if (ctl->options & CONTEXTS_OPTION_RETENTION) {
                        buffer_json_member_add_time_t_formatted(wb, "first_entry", z->first_time_s, ctl->options & CONTEXTS_OPTION_RFC3339);
                        buffer_json_member_add_time_t_formatted(wb, "last_entry", collected ? ctl->now : z->last_time_s, ctl->options & CONTEXTS_OPTION_RFC3339);
                    }

                    if (ctl->options & CONTEXTS_OPTION_LIVENESS)
                        buffer_json_member_add_boolean(wb, "live", collected);

                    // Add dimensions sub-object if requested
                    if ((ctl->options & CONTEXTS_OPTION_DIMENSIONS) && z->dimensions_dict) {
                        buffer_json_member_add_array(wb, "dimensions");
                        void *entry;
                        size_t count = 0;
                        size_t total = dictionary_entries(z->dimensions_dict);
                        size_t limit = ctl->request->cardinality_limit;

                        dfe_start_read(z->dimensions_dict, entry) {
                            if (limit && count >= limit - 1 && total > limit) {
                                // Add remaining count message
                                char msg[100];
                                snprintf(msg, sizeof(msg), "... %zu dimensions more", total - count);
                                buffer_json_add_array_item_string(wb, msg);
                                break;
                            }
                            buffer_json_add_array_item_string(wb, entry_dfe.name);
                            count++;
                        }
                        dfe_done(entry);
                        buffer_json_array_close(wb);
                    }

                    // Add labels sub-object if requested
                    if ((ctl->options & CONTEXTS_OPTION_LABELS) && z->labels_aggregated) {
                        rrdlabels_aggregated_to_buffer_json(
                            z->labels_aggregated, wb, "labels", ctl->request->cardinality_limit);
                    }

                    // Add instances sub-object if requested
                    if ((ctl->options & CONTEXTS_OPTION_INSTANCES) && z->instances_dict) {
                        buffer_json_member_add_array(wb, "instances");
                        void *entry;
                        size_t count = 0;
                        size_t total = dictionary_entries(z->instances_dict);
                        size_t limit = ctl->request->cardinality_limit;

                        dfe_start_read(z->instances_dict, entry) {
                            if (limit && count >= limit - 1 && total > limit) {
                                // Add remaining count message
                                char msg[100];
                                snprintf(msg, sizeof(msg), "... %zu instances more", total - count);
                                buffer_json_add_array_item_string(wb, msg);
                                break;
                            }
                            buffer_json_add_array_item_string(wb, entry_dfe.name);
                            count++;
                        }
                        dfe_done(entry);
                        buffer_json_array_close(wb);
                    }
                }
                buffer_json_object_close(wb);
            } else {
                buffer_json_add_array_item_string(wb, string2str(z->id));
            }

            contexts_count++;
        }
        dfe_done(z);

        if(contexts_is_object) {
            buffer_json_object_close(wb); // contexts

            if (ctl->options & CONTEXTS_OPTION_MCP) {
                buffer_json_member_add_string(wb, "info", MCP_INFO_CONTEXT_NEXT_STEPS);
            }
        }
        else {
            buffer_json_array_close(wb);

            if (ctl->options & CONTEXTS_OPTION_MCP) {
                buffer_json_member_add_string(wb, "info", MCP_INFO_CONTEXT_ARRAY_RESPONSE);
            }
        }
    }
}

int rrdcontext_to_json_v2(BUFFER *wb, struct api_v2_contexts_request *req, CONTEXTS_V2_MODE mode) {
    int resp = HTTP_RESP_OK;
    bool run = true;

    if(mode & CONTEXTS_V2_ALERTS) {
        req->options &= ~CONTEXTS_OPTION_CONFIGURATIONS;
    }

    if(mode & CONTEXTS_V2_ALERT_TRANSITIONS) {
        req->options &= ~CONTEXTS_OPTION_INSTANCES;
    }

    struct rrdcontext_to_json_v2_data ctl = {
            .wb = wb,
            .request = req,
            .mode = mode,
            .options = req->options,
            .versions = { 0 },
            .nodes.scope_pattern = string_to_simple_pattern(req->scope_nodes),
            .nodes.pattern = string_to_simple_pattern(req->nodes),
            .contexts.pattern = string_to_simple_pattern(req->contexts),
            .contexts.scope_pattern = string_to_simple_pattern(req->scope_contexts),
            .q.pattern = string_to_simple_pattern_nocase_substring(req->q),
            .alerts.alert_name_pattern = string_to_simple_pattern(req->alerts.alert),
            .window = {
                    .enabled = false,
                    .relative = false,
                    .after = req->after,
                    .before = req->before,
            },
            .timings = {
                    .received_ut = now_monotonic_usec(),
            }
    };

    bool debug = ctl.options & CONTEXTS_OPTION_DEBUG;
    
    // Initialize JSON keys based on options
    json_keys_init((ctl.options & CONTEXTS_OPTION_JSON_LONG_KEYS) ? JSON_KEYS_OPTION_LONG_KEYS : 0);

    if(mode & (CONTEXTS_V2_NODES | CONTEXTS_V2_FUNCTIONS | CONTEXTS_V2_ALERTS)) {
        ctl.nodes.dict = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
                                                    NULL, sizeof(struct contexts_v2_node));
    }

    if(mode & (CONTEXTS_V2_CONTEXTS | CONTEXTS_V2_SEARCH)) {
        ctl.contexts.dict = dictionary_create_advanced(
                DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL,
                sizeof(struct context_v2_entry));

        dictionary_register_conflict_callback(ctl.contexts.dict, contexts_conflict_callback, &ctl);
        dictionary_register_react_callback(ctl.contexts.dict, contexts_react_callback, &ctl);
        dictionary_register_delete_callback(ctl.contexts.dict, contexts_delete_callback, &ctl);
    }

    if(mode & CONTEXTS_V2_FUNCTIONS) {
        ctl.functions.dict = dictionary_create_advanced(
                DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL,
                sizeof(struct function_v2_entry));

        dictionary_register_insert_callback(ctl.functions.dict, functions_insert_callback, &ctl);
        dictionary_register_conflict_callback(ctl.functions.dict, functions_conflict_callback, &ctl);
        dictionary_register_delete_callback(ctl.functions.dict, functions_delete_callback, &ctl);
    }

    if(mode & CONTEXTS_V2_ALERTS) {
        if(!rrdcontexts_v2_init_alert_dictionaries(&ctl, req)) {
            resp = HTTP_RESP_NOT_FOUND;
            goto cleanup;
        }
    }

    if(req->after || req->before) {
        ctl.window.relative = rrdr_relative_window_to_absolute_query(
            &ctl.window.after, &ctl.window.before, &ctl.now, false);

        ctl.window.enabled = !(mode & CONTEXTS_V2_ALERT_TRANSITIONS);
    }
    else
        ctl.now = now_realtime_sec();

    buffer_json_initialize(wb, "\"", "\"", 0, true,
                           ((req->options & CONTEXTS_OPTION_MINIFY) && !(req->options & CONTEXTS_OPTION_DEBUG)) ? BUFFER_JSON_OPTIONS_MINIFY : BUFFER_JSON_OPTIONS_DEFAULT);

    if(!(req->options & CONTEXTS_OPTION_MCP))
        buffer_json_member_add_uint64(wb, "api", 2);

    if(req->options & CONTEXTS_OPTION_DEBUG) {
        buffer_json_member_add_object(wb, "request");
        {
            buffer_json_contexts_v2_mode_to_array(wb, "mode", mode);
            contexts_options_to_buffer_json_array(wb, "options", req->options);

            buffer_json_member_add_object(wb, "scope");
            {
                buffer_json_member_add_string(wb, "scope_nodes", req->scope_nodes);
                if (mode & (CONTEXTS_V2_CONTEXTS | CONTEXTS_V2_SEARCH | CONTEXTS_V2_ALERTS))
                    buffer_json_member_add_string(wb, "scope_contexts", req->scope_contexts);
            }
            buffer_json_object_close(wb);

            buffer_json_member_add_object(wb, "selectors");
            {
                buffer_json_member_add_string(wb, "nodes", req->nodes);

                if (mode & (CONTEXTS_V2_CONTEXTS | CONTEXTS_V2_SEARCH | CONTEXTS_V2_ALERTS))
                    buffer_json_member_add_string(wb, "contexts", req->contexts);

                if(mode & (CONTEXTS_V2_ALERTS | CONTEXTS_V2_ALERT_TRANSITIONS)) {
                    buffer_json_member_add_object(wb, "alerts");

                    if(mode & CONTEXTS_V2_ALERTS)
                        contexts_alerts_status_to_buffer_json_array(wb, "status", req->alerts.status);

                    if(mode & CONTEXTS_V2_ALERT_TRANSITIONS) {
                        buffer_json_member_add_string(wb, "context", req->contexts);
                        buffer_json_member_add_uint64(wb, "anchor_gi", req->alerts.global_id_anchor);
                        buffer_json_member_add_uint64(wb, "last", req->alerts.last);
                    }

                    buffer_json_member_add_string(wb, "alert", req->alerts.alert);
                    buffer_json_member_add_string(wb, "transition", req->alerts.transition);
                    buffer_json_object_close(wb); // alerts
                }
            }
            buffer_json_object_close(wb); // selectors

            buffer_json_member_add_object(wb, "filters");
            {
                if (mode & CONTEXTS_V2_SEARCH)
                    buffer_json_member_add_string(wb, "q", req->q);

                buffer_json_member_add_time_t_formatted(wb, "after", req->after, ctl.options & CONTEXTS_OPTION_RFC3339);
                buffer_json_member_add_time_t_formatted(wb, "before", req->before, ctl.options & CONTEXTS_OPTION_RFC3339);
            }
            buffer_json_object_close(wb); // filters

            if(mode & CONTEXTS_V2_ALERT_TRANSITIONS) {
                buffer_json_member_add_object(wb, "facets");
                {
                    for (int i = 0; i < ATF_TOTAL_ENTRIES; i++) {
                        buffer_json_member_add_string(wb, alert_transition_facets[i].query_param, req->alerts.facets[i]);
                    }
                }
                buffer_json_object_close(wb); // facets
            }
        }
        buffer_json_object_close(wb);
    }

    ssize_t ret = 0;
    if(run)
        ret = query_scope_foreach_host(ctl.nodes.scope_pattern, ctl.nodes.pattern,
                             rrdcontext_to_json_v2_add_host, &ctl,
                             &ctl.versions, ctl.q.host_node_id_str);

    if(unlikely(ret < 0)) {
        buffer_flush(wb);

        if(ret == -2) {
            buffer_strcat(wb, "query timeout");
            resp = HTTP_RESP_GATEWAY_TIMEOUT;
        }
        else {
            buffer_strcat(wb, "query interrupted");
            resp = HTTP_RESP_CLIENT_CLOSED_REQUEST;
        }
        goto cleanup;
    }

    ctl.timings.executed_ut = now_monotonic_usec();

    if(mode & CONTEXTS_V2_ALERT_TRANSITIONS) {
        contexts_v2_alert_transitions_to_json(wb, &ctl, debug);
    }
    else {
        if (mode & CONTEXTS_V2_NODES) {
            buffer_json_member_add_array(wb, "nodes");
            struct contexts_v2_node *t;
            dfe_start_read(ctl.nodes.dict, t) {
                rrdcontext_to_json_v2_rrdhost(wb, t->host, &ctl, t->ni);
            }
            dfe_done(t);
            buffer_json_array_close(wb);
        }

        if (mode & CONTEXTS_V2_FUNCTIONS) {
            buffer_json_member_add_array(wb, "functions");
            {
                struct function_v2_entry *t;
                dfe_start_read(ctl.functions.dict, t) {
                    buffer_json_add_array_item_object(wb);
                    {
                        const char *name = t_dfe.name ? strstr(t_dfe.name, RRDFUNCTIONS_VERSION_SEPARATOR) : NULL;
                        if(name)
                            name += sizeof(RRDFUNCTIONS_VERSION_SEPARATOR) - 1;
                        else
                            name = t_dfe.name;

                        buffer_json_member_add_string(wb, "name", name);
                        buffer_json_member_add_string(wb, "help", string2str(t->help));

                        if (!(ctl.options & CONTEXTS_OPTION_MCP)) {
                            buffer_json_member_add_array(wb, "ni");
                            {
                                for (size_t i = 0; i < t->used; i++)
                                    buffer_json_add_array_item_uint64(wb, t->node_ids[i]);
                            }
                            buffer_json_array_close(wb);

                            buffer_json_member_add_uint64(wb, "priority", t->priority);
                            buffer_json_member_add_uint64(wb, "version", t->version);
                        }
                        buffer_json_member_add_string(wb, "tags", string2str(t->tags));
                        http_access2buffer_json_array(wb, "access", t->access);
                    }
                    buffer_json_object_close(wb);
                }
                dfe_done(t);
            }
            buffer_json_array_close(wb);
        }

        if (mode & CONTEXTS_V2_SEARCH) {
            contexts_v2_search_results_to_json(wb, &ctl);
        }
        else if (mode & CONTEXTS_V2_CONTEXTS) {
            contexts_v2_contexts_to_json(wb, &ctl);
        }

        if (mode & CONTEXTS_V2_ALERTS)
            contexts_v2_alerts_to_json(wb, &ctl, debug);

        if (mode & CONTEXTS_V2_SEARCH) {
            buffer_json_member_add_object(wb, "searches");
            {
                buffer_json_member_add_uint64(wb, "strings", ctl.q.fts.string_searches);
                buffer_json_member_add_uint64(wb, "char", ctl.q.fts.char_searches);
                buffer_json_member_add_uint64(wb, "total", ctl.q.fts.searches);
            }
            buffer_json_object_close(wb);
        }

        if (mode & CONTEXTS_V2_VERSIONS)
            version_hashes_api_v2(wb, &ctl.versions);

        if (mode & CONTEXTS_V2_AGENTS)
            buffer_json_agents_v2(wb, &ctl.timings, ctl.now, mode & (CONTEXTS_V2_AGENTS_INFO), true, ctl.options);
    }

    if(!(ctl.options & CONTEXTS_OPTION_MCP))
        buffer_json_cloud_timings(wb, "timings", &ctl.timings);

    buffer_json_finalize(wb);

cleanup:
    dictionary_destroy(ctl.nodes.dict);
    dictionary_destroy(ctl.contexts.dict);
    dictionary_destroy(ctl.functions.dict);
    rrdcontexts_v2_alerts_cleanup(&ctl);
    simple_pattern_free(ctl.nodes.scope_pattern);
    simple_pattern_free(ctl.nodes.pattern);
    simple_pattern_free(ctl.contexts.pattern);
    simple_pattern_free(ctl.contexts.scope_pattern);
    simple_pattern_free(ctl.q.pattern);
    simple_pattern_free(ctl.alerts.alert_name_pattern);

    json_keys_reset();
    return resp;
}
