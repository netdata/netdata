// SPDX-License-Identifier: GPL-3.0-or-later

#include "function-topology-streaming.h"

#define STREAMING_FUNCTION_UPDATE_EVERY 10


static bool streaming_topology_host_guid(RRDHOST *host, char *dst, size_t dst_size);
static bool streaming_topology_uuid_guid(ND_UUID host_id, char *dst, size_t dst_size);
static void streaming_topology_agent_id_for_host(RRDHOST *host, char *dst, size_t dst_size);
static void streaming_topology_actor_id_from_guid(const char *guid, char *dst, size_t dst_size);
static void streaming_topology_actor_id_for_uuid(ND_UUID host_id, char *dst, size_t dst_size);
static uint32_t *streaming_topology_parent_child_count_get(DICTIONARY *parent_child_count, RRDHOST *host);
static struct streaming_topology_descendant_list *streaming_topology_descendants_get(DICTIONARY *parent_descendants, RRDHOST *host);

enum streaming_topology_received_type {
    STREAMING_TOPOLOGY_RECEIVED_STREAMING = 0,
    STREAMING_TOPOLOGY_RECEIVED_VIRTUAL,
    STREAMING_TOPOLOGY_RECEIVED_STALE,
};

struct streaming_topology_descendant {
    RRDHOST *host;
    ND_UUID source_uuid;
    bool source_local;
    uint8_t type;
};

struct streaming_topology_descendant_list {
    struct streaming_topology_descendant *items;
    size_t used;
    size_t size;
};

struct streaming_topology_options {
    bool info_only;
    char *function_copy;
};

static void streaming_topology_add_host_match(BUFFER *wb, RRDHOST *host) {
    buffer_json_member_add_object(wb, "match");
    {
        buffer_json_member_add_array(wb, "hostnames");
        {
            buffer_json_add_array_item_string(wb, rrdhost_hostname(host));
        }
        buffer_json_array_close(wb);

        char host_guid[UUID_STR_LEN];
        if(streaming_topology_host_guid(host, host_guid, sizeof(host_guid)))
            buffer_json_member_add_string(wb, "netdata_machine_guid", host_guid);

        if(!UUIDiszero(host->node_id))
            buffer_json_member_add_uuid(wb, "netdata_node_id", host->node_id.uuid);
    }
    buffer_json_object_close(wb);
}

static bool streaming_topology_host_guid(RRDHOST *host, char *dst, size_t dst_size) {
    if(!dst || dst_size < UUID_STR_LEN)
        return false;

    dst[0] = '\0';
    if(!host)
        return false;

    if(streaming_topology_uuid_guid(host->host_id, dst, dst_size))
        return true;

    if(host->machine_guid[0]) {
        ND_UUID machine_guid = UUID_ZERO;
        if(!uuid_parse(host->machine_guid, machine_guid.uuid))
            return streaming_topology_uuid_guid(machine_guid, dst, dst_size);
    }

    return false;
}

static bool streaming_topology_uuid_guid(ND_UUID host_id, char *dst, size_t dst_size) {
    if(!dst || dst_size < UUID_STR_LEN)
        return false;

    dst[0] = '\0';
    if(UUIDiszero(host_id))
        return false;

    uuid_unparse_lower(host_id.uuid, dst);
    return true;
}

static void streaming_topology_actor_id_from_guid(const char *guid, char *dst, size_t dst_size) {
    if(!dst || !dst_size)
        return;

    if(guid && *guid)
        snprintf(dst, dst_size, "netdata-machine-guid:%s", guid);
    else
        snprintf(dst, dst_size, "host:unknown");
}

static void streaming_topology_agent_id_for_host(RRDHOST *host, char *dst, size_t dst_size) {
    if(!dst || !dst_size)
        return;

    char host_guid[UUID_STR_LEN];
    if(streaming_topology_host_guid(host, host_guid, sizeof(host_guid)))
        snprintf(dst, dst_size, "%s", host_guid);
    else if(host)
        snprintf(dst, dst_size, "%s", rrdhost_hostname(host));
    else
        dst[0] = '\0';
}

static void streaming_topology_actor_id_for_uuid(ND_UUID host_id, char *dst, size_t dst_size) {
    char guid[UUID_STR_LEN];
    if(streaming_topology_uuid_guid(host_id, guid, sizeof(guid)))
        streaming_topology_actor_id_from_guid(guid, dst, dst_size);
    else
        streaming_topology_actor_id_from_guid(NULL, dst, dst_size);
}

static uint32_t *streaming_topology_parent_child_count_get(DICTIONARY *parent_child_count, RRDHOST *host) {
    char host_guid[UUID_STR_LEN];

    if(!streaming_topology_host_guid(host, host_guid, sizeof(host_guid)))
        return NULL;

    return dictionary_get(parent_child_count, host_guid);
}

static struct streaming_topology_descendant_list *streaming_topology_descendants_get(DICTIONARY *parent_descendants, RRDHOST *host) {
    char host_guid[UUID_STR_LEN];

    if(!streaming_topology_host_guid(host, host_guid, sizeof(host_guid)))
        return NULL;

    return dictionary_get(parent_descendants, host_guid);
}

static struct streaming_topology_descendant_list *streaming_topology_descendants_get_or_create(DICTIONARY *parent_descendants, ND_UUID host_id) {
    char host_guid[UUID_STR_LEN];
    if(!streaming_topology_uuid_guid(host_id, host_guid, sizeof(host_guid)))
        return NULL;

    struct streaming_topology_descendant_list *list = dictionary_get(parent_descendants, host_guid);
    if(list)
        return list;

    struct streaming_topology_descendant_list empty = {0};
    return dictionary_set(parent_descendants, host_guid, &empty, sizeof(empty));
}

static void streaming_topology_descendants_append(
    DICTIONARY *parent_descendants,
    ND_UUID parent_id,
    RRDHOST *host,
    enum streaming_topology_received_type type,
    bool source_local,
    ND_UUID source_uuid
) {
    struct streaming_topology_descendant_list *list = streaming_topology_descendants_get_or_create(parent_descendants, parent_id);
    if(!list)
        return;

    if(list->used == list->size) {
        size_t new_size = list->size ? list->size * 2 : 4;
        list->items = reallocz(list->items, new_size * sizeof(*list->items));
        list->size = new_size;
    }

    list->items[list->used++] = (struct streaming_topology_descendant) {
        .host = host,
        .source_uuid = source_uuid,
        .source_local = source_local,
        .type = (uint8_t)type,
    };
}

static const char *streaming_topology_received_type_to_string(enum streaming_topology_received_type type) {
    switch(type) {
        case STREAMING_TOPOLOGY_RECEIVED_VIRTUAL:
            return "virtual";

        case STREAMING_TOPOLOGY_RECEIVED_STALE:
            return "stale";

        case STREAMING_TOPOLOGY_RECEIVED_STREAMING:
        default:
            return "streaming";
    }
}

static void streaming_topology_actor_id_for_host(RRDHOST *host, char *dst, size_t dst_size) {
    if(!dst || !dst_size)
        return;

    char host_guid[UUID_STR_LEN];
    if(streaming_topology_host_guid(host, host_guid, sizeof(host_guid)))
        streaming_topology_actor_id_from_guid(host_guid, dst, dst_size);
    else if(host)
        snprintf(dst, dst_size, "hostname:%s", rrdhost_hostname(host));
    else
        snprintf(dst, dst_size, "host:unknown");
}

// get streaming_path host_ids, appending localhost only when the path already
// has upstream entries but does not yet include us; callers still use n == 0
// to detect hosts without an active path
static uint16_t streaming_topology_get_path_ids(RRDHOST *host, uint16_t from, ND_UUID *host_ids, uint16_t max) {
    uint16_t n = rrdhost_stream_path_get_host_ids(host, from, host_ids, max);
    uint16_t filtered_n = 0;

    // check if localhost is already in the path
    bool found_localhost = false;
    for(uint16_t i = 0; i < n; i++) {
        if(UUIDiszero(host_ids[i]))
            continue;

        host_ids[filtered_n++] = host_ids[i];
        if(UUIDeq(host_ids[i], localhost->host_id)) {
            found_localhost = true;
        }
    }
    n = filtered_n;

    // append localhost only when callers want the full path (from == 0). The
    // self-append mirrors rrdhost_stream_path_to_json's emit-time semantics.
    // For from > 0 (e.g. parent counting that asks for upstream-only entries)
    // appending self would falsely count the host as its own parent.
    if(from == 0 && !found_localhost && n < max && n > 0)
        host_ids[n++] = localhost->host_id;

    return n;
}

static void streaming_topology_parse_options(const char *function, struct streaming_topology_options *options) {
    if(!options)
        return;

    *options = (struct streaming_topology_options){ 0 };
    if(!function || !*function)
        return;

    options->function_copy = strdupz(function);
    char *words[1024];
    size_t num_words = quoted_strings_splitter_whitespace(options->function_copy, words, 1024);
    for(size_t i = 1; i < num_words; i++) {
        char *param = get_word(words, num_words, i);
        if(strcmp(param, "info") == 0)
            options->info_only = true;
    }
}

static int streaming_topology_return_error(BUFFER *wb, char *function_copy, int status, const char *error) {
    buffer_flush(wb);
    wb->content_type = CT_APPLICATION_JSON;
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);

    buffer_json_member_add_uint64(wb, "status", status);
    buffer_json_member_add_string(wb, "type", "topology");
    buffer_json_member_add_time_t(wb, "update_every", STREAMING_FUNCTION_UPDATE_EVERY);
    buffer_json_member_add_boolean(wb, "has_history", false);
    buffer_json_member_add_string(wb, "help", RRDFUNCTIONS_STREAMING_TOPOLOGY_HELP);
    buffer_json_member_add_string(wb, "error", error);
    buffer_json_finalize(wb);

    freez(function_copy);
    return status;
}

// helpers: emit one summary field and one table column
static void streaming_topology_emit_sf(BUFFER *wb, const char *key, const char *label, const char *source) {
    buffer_json_add_array_item_object(wb);
    buffer_json_member_add_string(wb, "key", key);
    buffer_json_member_add_string(wb, "label", label);
    buffer_json_member_add_array(wb, "sources");
    buffer_json_add_array_item_string(wb, source);
    buffer_json_array_close(wb);
    buffer_json_object_close(wb);
}

static void streaming_topology_emit_col(BUFFER *wb, const char *key, const char *label, const char *type) {
    buffer_json_add_array_item_object(wb);
    buffer_json_member_add_string(wb, "key", key);
    buffer_json_member_add_string(wb, "label", label);
    if(type)
        buffer_json_member_add_string(wb, "type", type);
    buffer_json_object_close(wb);
}

static void streaming_topology_info_tab(BUFFER *wb) {
    buffer_json_member_add_array(wb, "modal_tabs");
    {
        buffer_json_add_array_item_object(wb);
        buffer_json_member_add_string(wb, "id", "info");
        buffer_json_member_add_string(wb, "label", "Info");
        buffer_json_object_close(wb);
    }
    buffer_json_array_close(wb);
}

// streaming path table: shared by all non-parent types
static void streaming_topology_emit_streaming_path_table(BUFFER *wb, uint64_t order) {
    buffer_json_member_add_object(wb, "streaming_path");
    {
        buffer_json_member_add_string(wb, "label", "Streaming Path");
        buffer_json_member_add_string(wb, "source", "data");
        buffer_json_member_add_uint64(wb, "order", order);
        buffer_json_member_add_array(wb, "columns");
        {
            streaming_topology_emit_col(wb, "hostname", "Agent", NULL);
            streaming_topology_emit_col(wb, "hops", "Hops", "number");
            streaming_topology_emit_col(wb, "since", "Since", "timestamp");
            streaming_topology_emit_col(wb, "flags", "Flags", NULL);
        }
        buffer_json_array_close(wb);
    }
    buffer_json_object_close(wb);
}

// retention table: shared by all types
static void streaming_topology_emit_retention_table(BUFFER *wb, uint64_t order, const char *name_label) {
    buffer_json_member_add_object(wb, "retention");
    {
        buffer_json_member_add_string(wb, "label", "Retention");
        buffer_json_member_add_string(wb, "source", "data");
        buffer_json_member_add_uint64(wb, "order", order);
        buffer_json_member_add_array(wb, "columns");
        {
            streaming_topology_emit_col(wb, "name", name_label, "actor_link");
            streaming_topology_emit_col(wb, "db_status", "Status", "badge");
            streaming_topology_emit_col(wb, "db_from", "From", "timestamp");
            streaming_topology_emit_col(wb, "db_to", "To", "timestamp");
            streaming_topology_emit_col(wb, "db_duration", "Duration", "duration");
            streaming_topology_emit_col(wb, "db_metrics", "Metrics", "number");
            streaming_topology_emit_col(wb, "db_instances", "Instances", "number");
            streaming_topology_emit_col(wb, "db_contexts", "Contexts", "number");
        }
        buffer_json_array_close(wb);
    }
    buffer_json_object_close(wb);
}

// parent actor presentation: intrinsic summary + inbound/retention tables
static void streaming_topology_parent_presentation(BUFFER *wb) {
    buffer_json_member_add_array(wb, "summary_fields");
    {
        streaming_topology_emit_sf(wb, "display_name", "Name", "attributes.display_name");
        streaming_topology_emit_sf(wb, "node_type", "Type", "attributes.node_type");
        streaming_topology_emit_sf(wb, "agent_version", "Version", "attributes.agent_version");
        streaming_topology_emit_sf(wb, "os_name", "OS", "attributes.os_name");
        streaming_topology_emit_sf(wb, "architecture", "Arch", "attributes.architecture");
        streaming_topology_emit_sf(wb, "cpu_cores", "CPUs", "attributes.cpu_cores");
        streaming_topology_emit_sf(wb, "child_count", "Children", "attributes.child_count");
        streaming_topology_emit_sf(wb, "health_critical", "Alerts Critical", "attributes.health_critical");
        streaming_topology_emit_sf(wb, "health_warning", "Alerts Warning", "attributes.health_warning");
    }
    buffer_json_array_close(wb);

    buffer_json_member_add_object(wb, "tables");
    {
        // inbound: all nodes this parent has data for (rows = all known hosts)
        buffer_json_member_add_object(wb, "inbound");
        {
            buffer_json_member_add_string(wb, "label", "Inbound");
            buffer_json_member_add_string(wb, "source", "data");
            buffer_json_member_add_boolean(wb, "bullet_source", true);
            buffer_json_member_add_uint64(wb, "order", 0);
            buffer_json_member_add_array(wb, "columns");
            {
                streaming_topology_emit_col(wb, "name", "Node", "actor_link");
                streaming_topology_emit_col(wb, "received_from", "Source", "actor_link");
                streaming_topology_emit_col(wb, "node_type", "Type", "badge");
                streaming_topology_emit_col(wb, "ingest_status", "Ingest", "badge");
                streaming_topology_emit_col(wb, "hops", "Hops", "number");
                streaming_topology_emit_col(wb, "collected_metrics", "Metrics", "number");
                streaming_topology_emit_col(wb, "collected_instances", "Instances", "number");
                streaming_topology_emit_col(wb, "collected_contexts", "Contexts", "number");
                streaming_topology_emit_col(wb, "repl_completion", "Replication", "number");
                streaming_topology_emit_col(wb, "ingest_age", "Age", "duration");
                streaming_topology_emit_col(wb, "ssl", "SSL", "badge");
                streaming_topology_emit_col(wb, "alerts_critical", "Critical", "number");
                streaming_topology_emit_col(wb, "alerts_warning", "Warning", "number");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        // outbound: all nodes this parent streams to its parent (rows = all known hosts)
        buffer_json_member_add_object(wb, "outbound");
        {
            buffer_json_member_add_string(wb, "label", "Outbound");
            buffer_json_member_add_string(wb, "source", "data");
            buffer_json_member_add_uint64(wb, "order", 1);
            buffer_json_member_add_array(wb, "columns");
            {
                streaming_topology_emit_col(wb, "name", "Node", "actor_link");
                streaming_topology_emit_col(wb, "streamed_to", "Destination", "actor_link");
                streaming_topology_emit_col(wb, "node_type", "Type", "badge");
                streaming_topology_emit_col(wb, "stream_status", "Status", "badge");
                streaming_topology_emit_col(wb, "hops", "Hops", "number");
                streaming_topology_emit_col(wb, "ssl", "SSL", "badge");
                streaming_topology_emit_col(wb, "compression", "Compression", "badge");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        streaming_topology_emit_retention_table(wb, 2, "Node");
        streaming_topology_emit_streaming_path_table(wb, 3);
    }
    buffer_json_object_close(wb);

    streaming_topology_info_tab(wb);
}

// child actor presentation: intrinsic summary + streaming path/retention tables
static void streaming_topology_child_presentation(BUFFER *wb) {
    buffer_json_member_add_array(wb, "summary_fields");
    {
        streaming_topology_emit_sf(wb, "display_name", "Name", "attributes.display_name");
        streaming_topology_emit_sf(wb, "node_type", "Type", "attributes.node_type");
        streaming_topology_emit_sf(wb, "agent_version", "Version", "attributes.agent_version");
        streaming_topology_emit_sf(wb, "os_name", "OS", "attributes.os_name");
        streaming_topology_emit_sf(wb, "architecture", "Arch", "attributes.architecture");
        streaming_topology_emit_sf(wb, "cpu_cores", "CPUs", "attributes.cpu_cores");
        streaming_topology_emit_sf(wb, "health_critical", "Alerts Critical", "attributes.health_critical");
        streaming_topology_emit_sf(wb, "health_warning", "Alerts Warning", "attributes.health_warning");
    }
    buffer_json_array_close(wb);

    buffer_json_member_add_object(wb, "tables");
    {
        streaming_topology_emit_streaming_path_table(wb, 0);
        streaming_topology_emit_retention_table(wb, 1, "Parent");
    }
    buffer_json_object_close(wb);

    streaming_topology_info_tab(wb);
}

// vnode actor presentation: minimal summary + streaming path/retention
static void streaming_topology_vnode_presentation(BUFFER *wb) {
    buffer_json_member_add_array(wb, "summary_fields");
    {
        streaming_topology_emit_sf(wb, "display_name", "Name", "attributes.display_name");
        streaming_topology_emit_sf(wb, "node_type", "Type", "attributes.node_type");
        streaming_topology_emit_sf(wb, "ephemerality", "Ephemerality", "attributes.ephemerality");
    }
    buffer_json_array_close(wb);

    buffer_json_member_add_object(wb, "tables");
    {
        streaming_topology_emit_streaming_path_table(wb, 0);
        streaming_topology_emit_retention_table(wb, 1, "Parent");
    }
    buffer_json_object_close(wb);

    streaming_topology_info_tab(wb);
}

// stale actor presentation: identity summary + streaming path/retention
static void streaming_topology_stale_presentation(BUFFER *wb) {
    buffer_json_member_add_array(wb, "summary_fields");
    {
        streaming_topology_emit_sf(wb, "display_name", "Name", "attributes.display_name");
        streaming_topology_emit_sf(wb, "node_type", "Type", "attributes.node_type");
        streaming_topology_emit_sf(wb, "agent_version", "Version", "attributes.agent_version");
        streaming_topology_emit_sf(wb, "os_name", "OS", "attributes.os_name");
        streaming_topology_emit_sf(wb, "architecture", "Arch", "attributes.architecture");
    }
    buffer_json_array_close(wb);

    buffer_json_member_add_object(wb, "tables");
    {
        streaming_topology_emit_streaming_path_table(wb, 0);
        streaming_topology_emit_retention_table(wb, 1, "Parent");
    }
    buffer_json_object_close(wb);

    streaming_topology_info_tab(wb);
}

// Bug C synthesis context — passed through rrdhost_stream_path_visit
// to emit synthetic upstream actors and multi-hop links.
struct streaming_topology_synth_ctx {
    BUFFER *wb;
    DICTIONARY *local_actor_ids;      // actor_id -> sentinel; local RRDHOST-backed actors
    DICTIONARY *emitted_actors;       // actor_id -> sentinel; emitted actor dedup
    DICTIONARY *emitted_links;        // "src|dst" -> sentinel; emitted link dedup
    size_t *actors_total;
    size_t *links_total;
    // for the multi-hop link pass — previous slot info
    bool has_prev;
    char prev_actor_id[256];
    char prev_hostname[256];
    time_t prev_since;
    time_t prev_first_time_t;
};

// Visitor: emit a synthetic "parent" actor for each upstream path entry that
// has no local RRDHOST-backed actor. Local actors are authoritative and are
// always emitted by Phase 3.
static bool streaming_topology_synth_actor_visitor(
    void *userdata, uint16_t index __maybe_unused,
    STRING *hostname,
    ND_UUID host_id, ND_UUID node_id, ND_UUID claim_id __maybe_unused,
    int16_t hops,
    time_t since, time_t first_time_t,
    uint32_t start_time_ms, uint32_t shutdown_time_ms,
    STREAM_CAPABILITIES capabilities,
    uint32_t flags __maybe_unused) {

    struct streaming_topology_synth_ctx *ctx = userdata;

    // Skip the visiting host's own self entry — Phase 3 emits it if needed.
    if(UUIDeq(host_id, localhost->host_id))
        return true;

    char guid[UUID_STR_LEN];
    if(!streaming_topology_uuid_guid(host_id, guid, sizeof(guid)))
        return true;

    char actor_id[256];
    streaming_topology_actor_id_from_guid(guid, actor_id, sizeof(actor_id));

    // Skip local RRDHOST-backed actors and already synthesized actors.
    if(dictionary_get(ctx->local_actor_ids, actor_id) || dictionary_get(ctx->emitted_actors, actor_id))
        return true;

    {
        uint8_t one = 1;
        dictionary_set(ctx->emitted_actors, actor_id, &one, sizeof(one));
    }
    (*ctx->actors_total)++;

    BUFFER *wb = ctx->wb;
    buffer_json_add_array_item_object(wb);
    {
        buffer_json_member_add_string(wb, "actor_id", actor_id);
        buffer_json_member_add_string(wb, "actor_type", "parent");
        buffer_json_member_add_string(wb, "layer", "infra");
        buffer_json_member_add_string(wb, "source", "streaming");

        // Match block — same key names as streaming_topology_add_host_match
        // (function-topology-streaming.c:45) so the cloud-frontend resolves
        // hostnames/GUIDs identically for synthetic and real actors.
        buffer_json_member_add_object(wb, "match");
        {
            buffer_json_member_add_array(wb, "hostnames");
            buffer_json_add_array_item_string(wb, string2str(hostname));
            buffer_json_array_close(wb);

            buffer_json_member_add_string(wb, "netdata_machine_guid", guid);

            if(!UUIDiszero(node_id))
                buffer_json_member_add_uuid(wb, "netdata_node_id", node_id.uuid);
        }
        buffer_json_object_close(wb); // match

        // Limited attributes — synthesized from path data only. No local
        // rrdhost record exists for this remote upstream, so the agent has
        // no direct view of the parent's child_count, retention, OS info,
        // alerts, etc. The merge layer in Cloud will fill these in from the
        // parent's own response (where this actor is the "self" — its
        // agent_id matches this actor_id).
        buffer_json_member_add_object(wb, "attributes");
        {
            buffer_json_member_add_string(wb, "display_name", string2str(hostname));
            buffer_json_member_add_string(wb, "node_type", "parent");
            buffer_json_member_add_string(wb, "severity", "normal");
            buffer_json_member_add_uint64(wb, "child_count", 0);
            buffer_json_member_add_string(wb, "ephemerality", "permanent");
            buffer_json_member_add_int64(wb, "hops", hops);
            buffer_json_member_add_uint64(wb, "since", (uint64_t)since);
            buffer_json_member_add_uint64(wb, "first_time_t", (uint64_t)first_time_t);
            buffer_json_member_add_uint64(wb, "start_time_ms", start_time_ms);
            buffer_json_member_add_uint64(wb, "shutdown_time_ms", shutdown_time_ms);
            stream_capabilities_to_json_array(wb, capabilities, "capabilities");
        }
        buffer_json_object_close(wb); // attributes

        // Labels block — mirror Phase 3's set so the FE facet renderer sees
        // the same field keys for synthetic and real actors. Status fields
        // use path-derived defaults because no local RRDHOST exists.
        buffer_json_member_add_object(wb, "labels");
        {
            buffer_json_member_add_string(wb, "hostname", string2str(hostname));
            buffer_json_member_add_string(wb, "node_type", "parent");
            buffer_json_member_add_string(wb, "severity", "normal");
            buffer_json_member_add_string(wb, "ephemerality", "permanent");
            buffer_json_member_add_string(wb, "ingest_status", "online");
            buffer_json_member_add_string(wb, "stream_status", "online");
            buffer_json_member_add_string(wb, "ml_status", "unknown");
            buffer_json_member_add_string(wb, "display_name", string2str(hostname));
        }
        buffer_json_object_close(wb); // labels

        // The streaming_path field (highlight_path) is left empty for
        // synthesized actors — we don't have the full chain visible from
        // this side. The FE highlight_path still works for cross-actor
        // references because every actor_id is the canonical
        // netdata-machine-guid:<guid> form.
        buffer_json_member_add_array(wb, "streaming_path");
        buffer_json_array_close(wb);

        buffer_json_member_add_object(wb, "tables");
        {
            buffer_json_member_add_array(wb, "streaming_path");
            {
                buffer_json_add_array_item_object(wb);
                {
                    buffer_json_member_add_string(wb, "hostname", string2str(hostname));
                    buffer_json_member_add_int64(wb, "hops", hops);
                    buffer_json_member_add_uint64(wb, "since", (uint64_t)since);
                    // flags omitted: STREAM_PATH_FLAGS bitmap is opaque to
                    // synthesizing code; the table column renders blank for
                    // synthesized rows. Real actors include flags via
                    // rrdhost_stream_path_to_json.
                }
                buffer_json_object_close(wb);
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb); // tables
    }
    buffer_json_object_close(wb); // actor

    return true;
}

// Visitor: emit a streaming link between consecutive path slots. Phase 4
// registers the direct local-host links first; this pass uses emitted_links
// for uniform dedup and adds whatever remains (localhost's own upstream link
// and deeper multi-hop links).
static bool streaming_topology_synth_link_visitor(
    void *userdata, uint16_t index __maybe_unused,
    STRING *hostname,
    ND_UUID host_id, ND_UUID node_id __maybe_unused, ND_UUID claim_id __maybe_unused,
    int16_t hops __maybe_unused,
    time_t since, time_t first_time_t,
    uint32_t start_time_ms __maybe_unused, uint32_t shutdown_time_ms __maybe_unused,
    STREAM_CAPABILITIES capabilities __maybe_unused,
    uint32_t flags __maybe_unused) {

    struct streaming_topology_synth_ctx *ctx = userdata;

    char guid[UUID_STR_LEN];
    if(!streaming_topology_uuid_guid(host_id, guid, sizeof(guid))) {
        ctx->has_prev = false;
        return true;
    }

    char cur_actor_id[256];
    streaming_topology_actor_id_from_guid(guid, cur_actor_id, sizeof(cur_actor_id));

    if(ctx->has_prev) {
        if(dictionary_get(ctx->emitted_actors, ctx->prev_actor_id) &&
           dictionary_get(ctx->emitted_actors, cur_actor_id)) {

            char link_key[600];
            snprintfz(link_key, sizeof(link_key), "%s|%s", ctx->prev_actor_id, cur_actor_id);

            if(!dictionary_get(ctx->emitted_links, link_key)) {
                uint8_t one = 1;
                dictionary_set(ctx->emitted_links, link_key, &one, sizeof(one));
                (*ctx->links_total)++;

                BUFFER *wb = ctx->wb;
                buffer_json_add_array_item_object(wb);
                {
                    buffer_json_member_add_string(wb, "layer", "infra");
                    buffer_json_member_add_string(wb, "protocol", "streaming");
                    buffer_json_member_add_string(wb, "link_type", "streaming");
                    buffer_json_member_add_string(wb, "src_actor_id", ctx->prev_actor_id);
                    buffer_json_member_add_string(wb, "dst_actor_id", cur_actor_id);
                    buffer_json_member_add_string(wb, "state", "online");
                    // discovered_at / last_seen derive from STREAM_PATH
                    // timestamps so the merge layer can reconcile views from
                    // multiple agents reporting the same upstream link.
                    buffer_json_member_add_datetime_rfc3339(wb, "discovered_at",
                        ((uint64_t)(ctx->prev_first_time_t ? ctx->prev_first_time_t
                                                            : ctx->prev_since)) * USEC_PER_SEC, true);
                    buffer_json_member_add_datetime_rfc3339(wb, "last_seen",
                        ((uint64_t)(since ? since : ctx->prev_since)) * USEC_PER_SEC, true);

                    // port_name mirrors Phase 4: it is the source hostname.
                    // Here the source is the previous stream-path slot.
                    buffer_json_member_add_object(wb, "dst");
                    {
                        buffer_json_member_add_object(wb, "attributes");
                        {
                            buffer_json_member_add_string(wb, "port_name", ctx->prev_hostname);
                        }
                        buffer_json_object_close(wb);
                    }
                    buffer_json_object_close(wb);
                }
                buffer_json_object_close(wb); // link
            }
        }
    }

    ctx->has_prev = true;
    snprintfz(ctx->prev_actor_id, sizeof(ctx->prev_actor_id), "%s", cur_actor_id);
    snprintfz(ctx->prev_hostname, sizeof(ctx->prev_hostname), "%s", string2str(hostname));
    ctx->prev_since = since;
    ctx->prev_first_time_t = first_time_t;
    return true;
}

int function_streaming_topology(BUFFER *wb, const char *function, BUFFER *payload __maybe_unused, const char *source __maybe_unused) {
    time_t now = now_realtime_sec();
    usec_t now_ut = now_realtime_usec();

    struct streaming_topology_options options = { 0 };
    streaming_topology_parse_options(function, &options);
    bool info_only = options.info_only;
    char *function_copy = options.function_copy;

    buffer_flush(wb);
    wb->content_type = CT_APPLICATION_JSON;
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);

    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_string(wb, "type", "topology");
    buffer_json_member_add_time_t(wb, "update_every", STREAMING_FUNCTION_UPDATE_EVERY);
    buffer_json_member_add_boolean(wb, "has_history", false);
    buffer_json_member_add_string(wb, "help", RRDFUNCTIONS_STREAMING_TOPOLOGY_HELP);
    buffer_json_member_add_array(wb, "accepted_params");
    {
        buffer_json_add_array_item_string(wb, "info");
    }
    buffer_json_array_close(wb);
    buffer_json_member_add_array(wb, "required_params");
    buffer_json_array_close(wb);

    // --- presentation metadata ---
    buffer_json_member_add_object(wb, "presentation");
    {
        buffer_json_member_add_object(wb, "actor_types");
        {
            buffer_json_member_add_object(wb, "parent");
            {
                buffer_json_member_add_string(wb, "label", "Netdata Parent");
                buffer_json_member_add_string(wb, "color_slot", "primary");
                buffer_json_member_add_double(wb, "opacity", 1.0);
                buffer_json_member_add_boolean(wb, "border", true);
                buffer_json_member_add_string(wb, "role", "actor");
                buffer_json_member_add_boolean(wb, "size_by_links", true);
                buffer_json_member_add_boolean(wb, "show_port_bullets", true);
                streaming_topology_parent_presentation(wb);
            }
            buffer_json_object_close(wb);

            buffer_json_member_add_object(wb, "child");
            {
                buffer_json_member_add_string(wb, "label", "Netdata Child");
                buffer_json_member_add_string(wb, "color_slot", "primary");
                buffer_json_member_add_double(wb, "opacity", 1.0);
                buffer_json_member_add_boolean(wb, "border", false);
                buffer_json_member_add_string(wb, "role", "actor");
                buffer_json_member_add_boolean(wb, "size_by_links", false);
                buffer_json_member_add_boolean(wb, "show_port_bullets", false);
                streaming_topology_child_presentation(wb);
            }
            buffer_json_object_close(wb);

            buffer_json_member_add_object(wb, "vnode");
            {
                buffer_json_member_add_string(wb, "label", "Virtual Node");
                buffer_json_member_add_string(wb, "color_slot", "warning");
                buffer_json_member_add_double(wb, "opacity", 1.0);
                buffer_json_member_add_boolean(wb, "border", false);
                buffer_json_member_add_string(wb, "role", "actor");
                buffer_json_member_add_boolean(wb, "size_by_links", false);
                buffer_json_member_add_boolean(wb, "show_port_bullets", false);
                streaming_topology_vnode_presentation(wb);
            }
            buffer_json_object_close(wb);

            buffer_json_member_add_object(wb, "stale");
            {
                buffer_json_member_add_string(wb, "label", "Stale Node");
                buffer_json_member_add_string(wb, "color_slot", "dim");
                buffer_json_member_add_double(wb, "opacity", 0.5);
                buffer_json_member_add_boolean(wb, "border", false);
                buffer_json_member_add_string(wb, "role", "actor");
                buffer_json_member_add_boolean(wb, "size_by_links", false);
                buffer_json_member_add_boolean(wb, "show_port_bullets", false);
                streaming_topology_stale_presentation(wb);
            }
            buffer_json_object_close(wb);
        }
        buffer_json_object_close(wb); // actor_types

        buffer_json_member_add_object(wb, "link_types");
        {
            buffer_json_member_add_object(wb, "streaming");
            {
                buffer_json_member_add_string(wb, "label", "Streaming");
                buffer_json_member_add_string(wb, "color_slot", "primary");
                buffer_json_member_add_double(wb, "width", 2);
                buffer_json_member_add_boolean(wb, "dash", false);
                buffer_json_member_add_double(wb, "opacity", 1.0);
            }
            buffer_json_object_close(wb);

            buffer_json_member_add_object(wb, "virtual");
            {
                buffer_json_member_add_string(wb, "label", "Virtual origin");
                buffer_json_member_add_string(wb, "color_slot", "warning");
                buffer_json_member_add_double(wb, "width", 1);
                buffer_json_member_add_boolean(wb, "dash", true);
                buffer_json_member_add_double(wb, "opacity", 0.7);
            }
            buffer_json_object_close(wb);

            buffer_json_member_add_object(wb, "stale");
            {
                buffer_json_member_add_string(wb, "label", "Stale data");
                buffer_json_member_add_string(wb, "color_slot", "dim");
                buffer_json_member_add_double(wb, "width", 1);
                buffer_json_member_add_boolean(wb, "dash", true);
                buffer_json_member_add_double(wb, "opacity", 0.4);
            }
            buffer_json_object_close(wb);
        }
        buffer_json_object_close(wb); // link_types

        buffer_json_member_add_array(wb, "port_fields");
        {
            buffer_json_add_array_item_object(wb);
            buffer_json_member_add_string(wb, "key", "type");
            buffer_json_member_add_string(wb, "label", "Type");
            buffer_json_object_close(wb);
        }
        buffer_json_array_close(wb); // port_fields

        buffer_json_member_add_object(wb, "port_types");
        {
            buffer_json_member_add_object(wb, "streaming");
            {
                buffer_json_member_add_string(wb, "label", "Streaming child");
                buffer_json_member_add_string(wb, "color_slot", "primary");
                buffer_json_member_add_double(wb, "opacity", 1.0);
            }
            buffer_json_object_close(wb);

            buffer_json_member_add_object(wb, "virtual");
            {
                buffer_json_member_add_string(wb, "label", "Virtual node");
                buffer_json_member_add_string(wb, "color_slot", "warning");
                buffer_json_member_add_double(wb, "opacity", 1.0);
            }
            buffer_json_object_close(wb);

            buffer_json_member_add_object(wb, "stale");
            {
                buffer_json_member_add_string(wb, "label", "Stale node");
                buffer_json_member_add_string(wb, "color_slot", "dim");
                buffer_json_member_add_double(wb, "opacity", 0.5);
            }
            buffer_json_object_close(wb);
        }
        buffer_json_object_close(wb); // port_types

        buffer_json_member_add_object(wb, "legend");
        {
            buffer_json_member_add_array(wb, "actors");
            {
                buffer_json_add_array_item_object(wb);
                buffer_json_member_add_string(wb, "type", "parent");
                buffer_json_member_add_string(wb, "label", "Parent");
                buffer_json_object_close(wb);

                buffer_json_add_array_item_object(wb);
                buffer_json_member_add_string(wb, "type", "child");
                buffer_json_member_add_string(wb, "label", "Child");
                buffer_json_object_close(wb);

                buffer_json_add_array_item_object(wb);
                buffer_json_member_add_string(wb, "type", "vnode");
                buffer_json_member_add_string(wb, "label", "Virtual Node");
                buffer_json_object_close(wb);

                buffer_json_add_array_item_object(wb);
                buffer_json_member_add_string(wb, "type", "stale");
                buffer_json_member_add_string(wb, "label", "Stale Node");
                buffer_json_object_close(wb);
            }
            buffer_json_array_close(wb); // actors

            buffer_json_member_add_array(wb, "links");
            {
                buffer_json_add_array_item_object(wb);
                buffer_json_member_add_string(wb, "type", "streaming");
                buffer_json_member_add_string(wb, "label", "Streaming");
                buffer_json_object_close(wb);

                buffer_json_add_array_item_object(wb);
                buffer_json_member_add_string(wb, "type", "virtual");
                buffer_json_member_add_string(wb, "label", "Virtual origin");
                buffer_json_object_close(wb);

                buffer_json_add_array_item_object(wb);
                buffer_json_member_add_string(wb, "type", "stale");
                buffer_json_member_add_string(wb, "label", "Stale data");
                buffer_json_object_close(wb);
            }
            buffer_json_array_close(wb); // links

            buffer_json_member_add_array(wb, "ports");
            {
                buffer_json_add_array_item_object(wb);
                buffer_json_member_add_string(wb, "type", "streaming");
                buffer_json_member_add_string(wb, "label", "Streaming child");
                buffer_json_object_close(wb);

                buffer_json_add_array_item_object(wb);
                buffer_json_member_add_string(wb, "type", "virtual");
                buffer_json_member_add_string(wb, "label", "Virtual node");
                buffer_json_object_close(wb);

                buffer_json_add_array_item_object(wb);
                buffer_json_member_add_string(wb, "type", "stale");
                buffer_json_member_add_string(wb, "label", "Stale node");
                buffer_json_object_close(wb);
            }
            buffer_json_array_close(wb); // ports
        }
        buffer_json_object_close(wb); // legend

        buffer_json_member_add_string(wb, "actor_click_behavior", "highlight_path");
    }
    buffer_json_object_close(wb); // presentation

    if(!info_only) {
        // --- Phase 1: build parent_child_count dictionary from streaming_paths ---
        // A node is a parent if any other node's streaming_path contains it at position > 0
        DICTIONARY *parent_child_count = dictionary_create_advanced(
            DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
            NULL, sizeof(uint32_t));
        DICTIONARY *parent_descendants = dictionary_create_advanced(
            DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
            NULL, sizeof(struct streaming_topology_descendant_list));
        // local_actor_ids: every actor backed by a local RRDHOST.
        // emitted_actors / emitted_links: dedup sets for actors/links actually
        // written to the response.
        // The dictionary stores a 1-byte sentinel so dictionary_get() returns
        // a non-NULL value for present keys. With value_len=0 the stored value
        // is NULL and dictionary_get() can't distinguish present vs absent.
        DICTIONARY *local_actor_ids = dictionary_create_advanced(
            DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
            NULL, sizeof(uint8_t));
        DICTIONARY *emitted_actors = dictionary_create_advanced(
            DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
            NULL, sizeof(uint8_t));
        DICTIONARY *emitted_links = dictionary_create_advanced(
            DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
            NULL, sizeof(uint8_t));

        if(!parent_child_count || !parent_descendants || !local_actor_ids || !emitted_actors || !emitted_links) {
            if(emitted_links)
                dictionary_destroy(emitted_links);
            if(emitted_actors)
                dictionary_destroy(emitted_actors);
            if(local_actor_ids)
                dictionary_destroy(local_actor_ids);
            if(parent_descendants)
                dictionary_destroy(parent_descendants);
            if(parent_child_count)
                dictionary_destroy(parent_child_count);

            return streaming_topology_return_error(wb, function_copy,
                HTTP_RESP_INTERNAL_SERVER_ERROR,
                "failed to allocate streaming topology dictionaries");
        }
        else {

            {
                RRDHOST *host;
                dfe_start_read(rrdhost_root_index, host) {
                    char host_actor_id[256];
                    streaming_topology_actor_id_for_host(host, host_actor_id, sizeof(host_actor_id));
                    uint8_t one = 1;
                    dictionary_set(local_actor_ids, host_actor_id, &one, sizeof(one));
                }
                dfe_done(host);
            }

            {
                RRDHOST *host;
                dfe_start_read(rrdhost_root_index, host) {
                    // get all path entries at position > 0 (parents in the chain)
                    ND_UUID path_ids[128];
                    uint16_t n = streaming_topology_get_path_ids(host, 1, path_ids, 128);
                    for(uint16_t i = 0; i < n; i++) {
                        char guid[UUID_STR_LEN];
                        if(!streaming_topology_uuid_guid(path_ids[i], guid, sizeof(guid)))
                            continue;

                        uint32_t *count = dictionary_get(parent_child_count, guid);
                        if(count)
                            (*count)++;
                        else {
                            uint32_t one = 1;
                            dictionary_set(parent_child_count, guid, &one, sizeof(one));
                        }
                    }

                    ND_UUID full_path_ids[128];
                    uint16_t full_path_n = streaming_topology_get_path_ids(host, 0, full_path_ids, 128);
                    ND_UUID empty_uuid = {};

                    if(rrdhost_is_virtual(host))
                        continue;

                    if(full_path_n > 0) {
                        for(uint16_t i = 0; i < full_path_n; i++) {
                            // Skip the localhost slot here — Bug A's
                            // live-state walk below populates
                            // parent_descendants[localhost] authoritatively
                            // from rrdhost_status(). If we appended here too
                            // we would either double-count or mis-tag offline
                            // children as STREAMING.
                            if(UUIDeq(full_path_ids[i], localhost->host_id))
                                continue;

                            bool source_local = (i == 0);
                            ND_UUID source_uuid = source_local ? empty_uuid : full_path_ids[i - 1];
                            streaming_topology_descendants_append(parent_descendants,
                                full_path_ids[i], host, STREAMING_TOPOLOGY_RECEIVED_STREAMING, source_local, source_uuid);
                        }
                    }
                }
                dfe_done(host);
            }

            // Bug A fix: localhost live-state classification and descendants.
            // On an apex agent, the path-based walk above can miss localhost
            // as a parent because each child's stored path on us still only
            // contains the child itself until a sparse trigger (retention
            // boundary, node_id update) fires on that child. localhost is
            // appended to the wire format only at JSON-emit time, not stored.
            // We walk rrdhost_root_index once and authoritatively populate
            // parent_child_count[localhost] and parent_descendants[localhost].
            // The path walk above skips localhost so this is the single writer
            // for localhost descendants.
            {
                char localhost_guid[UUID_STR_LEN];
                if(streaming_topology_uuid_guid(localhost->host_id, localhost_guid, sizeof(localhost_guid))) {
                    uint32_t live_count = 0;
                    ND_UUID empty_uuid_for_live = {};
                    RRDHOST *h;
                    dfe_start_read(rrdhost_root_index, h) {
                        if(h == localhost)
                            continue;

                        if(rrdhost_is_virtual(h)) {
                            streaming_topology_descendants_append(parent_descendants,
                                localhost->host_id, h,
                                STREAMING_TOPOLOGY_RECEIVED_VIRTUAL, true, empty_uuid_for_live);
                            continue;
                        }

                        RRDHOST_STATUS hs;
                        rrdhost_status(h, now, &hs, RRDHOST_STATUS_ALL);

                        if(hs.ingest.type == RRDHOST_INGEST_TYPE_CHILD &&
                           (hs.ingest.status == RRDHOST_INGEST_STATUS_ONLINE ||
                            hs.ingest.status == RRDHOST_INGEST_STATUS_REPLICATING)) {
                            live_count++;

                            streaming_topology_descendants_append(parent_descendants,
                                localhost->host_id, h,
                                STREAMING_TOPOLOGY_RECEIVED_STREAMING, false, empty_uuid_for_live);
                        }
                        else {
                            streaming_topology_descendants_append(parent_descendants,
                                localhost->host_id, h,
                                STREAMING_TOPOLOGY_RECEIVED_STALE, false, empty_uuid_for_live);
                        }
                    }
                    dfe_done(h);

                    uint32_t *existing = dictionary_get(parent_child_count, localhost_guid);
                    if(existing)
                        *existing = live_count;
                    else if(live_count > 0)
                        dictionary_set(parent_child_count, localhost_guid, &live_count, sizeof(live_count));
                }
            }

            buffer_json_member_add_object(wb, "data");
            {
                size_t actors_total = 0;
                size_t links_total = 0;

            buffer_json_member_add_string(wb, "schema_version", "2.0");
            buffer_json_member_add_string(wb, "source", "streaming");
            buffer_json_member_add_string(wb, "layer", "infra");
            char localhost_agent_id[256];
            streaming_topology_agent_id_for_host(localhost, localhost_agent_id, sizeof(localhost_agent_id));
            buffer_json_member_add_string(wb, "agent_id", localhost_agent_id);
            buffer_json_member_add_datetime_rfc3339(wb, "collected_at", now_ut, true);

            // --- Phase 3: emit actors ---
            buffer_json_member_add_array(wb, "actors");
            {
                RRDHOST *host;
                dfe_start_read(rrdhost_root_index, host) {
                    RRDHOST_STATUS s;
                    rrdhost_status(host, now, &s, RRDHOST_STATUS_ALL);
                    const char *hostname = rrdhost_hostname(host);
                    char host_actor_id[256];
                    streaming_topology_actor_id_for_host(host, host_actor_id, sizeof(host_actor_id));

                    // classify node type by role
                    // stale = no connections ever (ARCHIVED status with 0 connections)
                    // vnode, parent, child determined by role in topology
                    const char *node_type;
                    if(rrdhost_is_virtual(host))
                        node_type = "vnode";
                    else if(host != localhost && s.ingest.status == RRDHOST_INGEST_STATUS_ARCHIVED)
                        node_type = "stale";
                    else {
                        uint32_t *cc = streaming_topology_parent_child_count_get(parent_child_count, host);
                        node_type = (cc && *cc > 0) ? "parent" : "child";
                    }

                    uint32_t child_count = 0;
                    {
                        uint32_t *cc = streaming_topology_parent_child_count_get(parent_child_count, host);
                        if(cc) child_count = *cc;
                    }

                    // compute severity (same logic as table function)
                    const char *severity = "normal";
                    if(!rrdhost_option_check(host, RRDHOST_OPTION_EPHEMERAL_HOST)) {
                        switch(s.ingest.status) {
                            case RRDHOST_INGEST_STATUS_OFFLINE:
                            case RRDHOST_INGEST_STATUS_ARCHIVED:
                                severity = "critical";
                                break;
                            default:
                                break;
                        }
                        if(strcmp(severity, "normal") == 0) {
                            switch(s.stream.status) {
                                case RRDHOST_STREAM_STATUS_OFFLINE:
                                    if(s.stream.reason != STREAM_HANDSHAKE_SP_NO_DESTINATION)
                                        severity = "warning";
                                    break;
                                default:
                                    break;
                            }
                        }
                    }

                    actors_total++;
                    {
                        uint8_t one = 1;
                        dictionary_set(emitted_actors, host_actor_id, &one, sizeof(one));
                    }
                    buffer_json_add_array_item_object(wb);
                    {
                        buffer_json_member_add_string(wb, "actor_id", host_actor_id);
                        buffer_json_member_add_string(wb, "actor_type", node_type);
                        buffer_json_member_add_string(wb, "layer", "infra");
                        buffer_json_member_add_string(wb, "source", "streaming");
                        streaming_topology_add_host_match(wb, host);

                        buffer_json_member_add_object(wb, "attributes");
                        {
                            // intrinsic identity
                            buffer_json_member_add_string(wb, "display_name", hostname);
                            buffer_json_member_add_string(wb, "node_type", node_type);
                            buffer_json_member_add_string(wb, "severity", severity);
                            buffer_json_member_add_uint64(wb, "child_count", child_count);
                            buffer_json_member_add_string(wb, "ephemerality", rrdhost_option_check(host, RRDHOST_OPTION_EPHEMERAL_HOST) ? "ephemeral" : "permanent");
                            buffer_json_member_add_string(wb, "agent_name", rrdhost_program_name(host));
                            buffer_json_member_add_string(wb, "agent_version", rrdhost_program_version(host));

                            // system info (intrinsic hardware/OS fields)
                            rrdhost_system_info_to_json_object_fields(wb, s.host->system_info);

                            // health/alerts (intrinsic to the node running health)
                            buffer_json_member_add_string(wb, "health_status", rrdhost_health_status_to_string(s.health.status));
                            if(s.health.status == RRDHOST_HEALTH_STATUS_RUNNING) {
                                buffer_json_member_add_uint64(wb, "health_critical", s.health.alerts.critical);
                                buffer_json_member_add_uint64(wb, "health_warning", s.health.alerts.warning);
                                buffer_json_member_add_uint64(wb, "health_clear", s.health.alerts.clear);
                            }
                            else {
                                buffer_json_member_add_uint64(wb, "health_critical", 0);
                                buffer_json_member_add_uint64(wb, "health_warning", 0);
                                buffer_json_member_add_uint64(wb, "health_clear", 0);
                            }
                        }
                        buffer_json_object_close(wb); // attributes

                        buffer_json_member_add_object(wb, "labels");
                        {
                            // topology-specific labels (used by facets and tables)
                            buffer_json_member_add_string(wb, "hostname", hostname);
                            buffer_json_member_add_string(wb, "node_type", node_type);
                            buffer_json_member_add_string(wb, "severity", severity);
                            buffer_json_member_add_string(wb, "ephemerality", rrdhost_option_check(host, RRDHOST_OPTION_EPHEMERAL_HOST) ? "ephemeral" : "permanent");
                            buffer_json_member_add_string(wb, "ingest_status", rrdhost_ingest_status_to_string(s.ingest.status));
                            buffer_json_member_add_string(wb, "stream_status", rrdhost_streaming_status_to_string(s.stream.status));
                            buffer_json_member_add_string(wb, "ml_status", rrdhost_ml_status_to_string(s.ml.status));
                            buffer_json_member_add_string(wb, "display_name", hostname);

                            // Host labels are nested to avoid collisions with reserved
                            // topology label keys such as hostname/node_type/severity.
                            buffer_json_member_add_object(wb, "host_labels");
                            {
                                rrdlabels_to_buffer_json_members(host->rrdlabels, wb);
                            }
                            buffer_json_object_close(wb); // host_labels
                        }
                        buffer_json_object_close(wb); // labels

                        // streaming_path: array of actor_ids for highlight_path
                        {
                            ND_UUID path_ids[128];
                            uint16_t path_n = streaming_topology_get_path_ids(host, 0, path_ids, 128);
                            buffer_json_member_add_array(wb, "streaming_path");
                            for(uint16_t pi = 0; pi < path_n; pi++) {
                                char path_actor_id[256];
                                streaming_topology_actor_id_for_uuid(path_ids[pi], path_actor_id, sizeof(path_actor_id));
                                buffer_json_add_array_item_string(wb, path_actor_id);
                            }
                            buffer_json_array_close(wb);
                        }

                        // received_nodes: all nodes this parent receives data for (drives bullet count + type)
                        if(child_count > 0) {
                            buffer_json_member_add_array(wb, "received_nodes");
                            struct streaming_topology_descendant_list *received_nodes =
                                streaming_topology_descendants_get(parent_descendants, host);
                            if(received_nodes) {
                                for(size_t i = 0; i < received_nodes->used; i++) {
                                    struct streaming_topology_descendant *descendant = &received_nodes->items[i];
                                    RRDHOST *rn_host = descendant->host;

                                    if(rn_host == host)
                                        continue;

                                    buffer_json_add_array_item_object(wb);
                                    buffer_json_member_add_string(wb, "name", rrdhost_hostname(rn_host));
                                    buffer_json_member_add_string(wb, "type",
                                        streaming_topology_received_type_to_string((enum streaming_topology_received_type)descendant->type));
                                    buffer_json_object_close(wb);
                                }
                            }
                            buffer_json_array_close(wb);
                        }

                        // --- per-actor tables ---
                        buffer_json_member_add_object(wb, "tables");
                        {
                            bool is_observer = (host == localhost);

                            // PARENT tables: inbound, outbound, retention
                            if(child_count > 0) {
                                // inbound table: ALL nodes this parent has data for
                                if(is_observer) {
                                    buffer_json_member_add_array(wb, "inbound");
                                    {
                                        RRDHOST *ih;
                                        dfe_start_read(rrdhost_root_index, ih) {
                                            char ih_actor_id[256];
                                            streaming_topology_actor_id_for_host(ih, ih_actor_id, sizeof(ih_actor_id));

                                            RRDHOST_STATUS ihs;
                                            rrdhost_status(ih, now, &ihs, RRDHOST_STATUS_ALL);

                                            // determine node type
                                            const char *ih_node_type;
                                            if(rrdhost_is_virtual(ih))
                                                ih_node_type = "vnode";
                                            else if(ih != localhost && ihs.ingest.status == RRDHOST_INGEST_STATUS_ARCHIVED)
                                                ih_node_type = "stale";
                                            else {
                                                uint32_t *cc = streaming_topology_parent_child_count_get(parent_child_count, ih);
                                                ih_node_type = (cc && *cc > 0) ? "parent" : "child";
                                            }

                                            // determine source: who sends this host's data to us
                                            const char *src_hostname = NULL;
                                            char src_actor_id[256] = "";
                                            if(ih == host) {
                                                src_hostname = "local";
                                            }
                                            else if(rrdhost_is_virtual(ih)) {
                                                src_hostname = "local";
                                            }
                                            else {
                                                ND_UUID ih_path[128];
                                                uint16_t ih_pn = streaming_topology_get_path_ids(ih, 0, ih_path, 128);
                                                char src_guid[UUID_STR_LEN] = "";
                                                for(uint16_t pi = 0; pi < ih_pn; pi++) {
                                                    if(UUIDeq(ih_path[pi], host->host_id) && pi > 0) {
                                                        uuid_unparse_lower(ih_path[pi - 1].uuid, src_guid);
                                                        streaming_topology_actor_id_from_guid(src_guid, src_actor_id, sizeof(src_actor_id));
                                                        RRDHOST *src = rrdhost_find_by_guid(src_guid);
                                                        src_hostname = src ? rrdhost_hostname(src) : src_guid;
                                                        break;
                                                    }
                                                }
                                                if(!src_hostname) src_hostname = "unknown";
                                            }

                                            buffer_json_add_array_item_object(wb);
                                            buffer_json_member_add_string(wb, "name", rrdhost_hostname(ih));
                                            buffer_json_member_add_string(wb, "name_id", ih_actor_id);
                                            buffer_json_member_add_string(wb, "received_from", src_hostname);
                                            if(src_actor_id[0])
                                                buffer_json_member_add_string(wb, "received_from_id", src_actor_id);
                                            buffer_json_member_add_string(wb, "node_type", ih_node_type);
                                            buffer_json_member_add_string(wb, "ingest_status", rrdhost_ingest_status_to_string(ihs.ingest.status));
                                            buffer_json_member_add_int64(wb, "hops", ihs.ingest.hops);
                                            buffer_json_member_add_uint64(wb, "collected_metrics", ihs.ingest.collected.metrics);
                                            buffer_json_member_add_uint64(wb, "collected_instances", ihs.ingest.collected.instances);
                                            buffer_json_member_add_uint64(wb, "collected_contexts", ihs.ingest.collected.contexts);
                                            buffer_json_member_add_double(wb, "repl_completion", ihs.ingest.replication.completion);
                                            buffer_json_member_add_time_t(wb, "ingest_age", ihs.ingest.since ? ihs.now - ihs.ingest.since : 0);
                                            buffer_json_member_add_string(wb, "ssl", ihs.ingest.ssl ? "SSL" : "PLAIN");
                                            buffer_json_member_add_uint64(wb, "alerts_critical",
                                                ihs.health.status == RRDHOST_HEALTH_STATUS_RUNNING ? ihs.health.alerts.critical : 0);
                                            buffer_json_member_add_uint64(wb, "alerts_warning",
                                                ihs.health.status == RRDHOST_HEALTH_STATUS_RUNNING ? ihs.health.alerts.warning : 0);
                                            buffer_json_object_close(wb);
                                        }
                                        dfe_done(ih);
                                    }
                                    buffer_json_array_close(wb); // inbound
                                }
                                else {
                                    // non-observer parent: use precomputed descendants
                                    buffer_json_member_add_array(wb, "inbound");
                                    {
                                        struct streaming_topology_descendant_list *inbound_nodes =
                                            streaming_topology_descendants_get(parent_descendants, host);
                                        if(inbound_nodes) {
                                            for(size_t i = 0; i < inbound_nodes->used; i++) {
                                                struct streaming_topology_descendant *descendant = &inbound_nodes->items[i];
                                                RRDHOST *ih = descendant->host;
                                                const char *src_hostname = NULL;
                                                char src_actor_id[256] = "";
                                                RRDHOST_STATUS ihs;

                                                if(descendant->source_local)
                                                    src_hostname = "local";
                                                else if(!UUIDiszero(descendant->source_uuid)) {
                                                    char src_guid[UUID_STR_LEN];
                                                    uuid_unparse_lower(descendant->source_uuid.uuid, src_guid);
                                                    streaming_topology_actor_id_from_guid(src_guid, src_actor_id, sizeof(src_actor_id));
                                                    RRDHOST *src = rrdhost_find_by_guid(src_guid);
                                                    src_hostname = src ? rrdhost_hostname(src) : src_guid;
                                                }
                                                else
                                                    src_hostname = "unknown";

                                                char ih_actor_id[256];
                                                streaming_topology_actor_id_for_host(ih, ih_actor_id, sizeof(ih_actor_id));

                                                rrdhost_status(ih, now, &ihs, RRDHOST_STATUS_ALL);

                                                const char *ih_node_type;
                                                if(rrdhost_is_virtual(ih))
                                                    ih_node_type = "vnode";
                                                else if(ih != localhost && ihs.ingest.status == RRDHOST_INGEST_STATUS_ARCHIVED)
                                                    ih_node_type = "stale";
                                                else {
                                                    uint32_t *cc = streaming_topology_parent_child_count_get(parent_child_count, ih);
                                                    ih_node_type = (cc && *cc > 0) ? "parent" : "child";
                                                }

                                                buffer_json_add_array_item_object(wb);
                                                buffer_json_member_add_string(wb, "name", rrdhost_hostname(ih));
                                                buffer_json_member_add_string(wb, "name_id", ih_actor_id);
                                                buffer_json_member_add_string(wb, "received_from", src_hostname);
                                                if(src_actor_id[0])
                                                    buffer_json_member_add_string(wb, "received_from_id", src_actor_id);
                                                buffer_json_member_add_string(wb, "node_type", ih_node_type);
                                                buffer_json_member_add_string(wb, "ingest_status", rrdhost_ingest_status_to_string(ihs.ingest.status));
                                                buffer_json_member_add_int64(wb, "hops", ihs.ingest.hops);
                                                buffer_json_member_add_uint64(wb, "collected_metrics", ihs.ingest.collected.metrics);
                                                buffer_json_member_add_uint64(wb, "collected_instances", ihs.ingest.collected.instances);
                                                buffer_json_member_add_uint64(wb, "collected_contexts", ihs.ingest.collected.contexts);
                                                buffer_json_member_add_double(wb, "repl_completion", ihs.ingest.replication.completion);
                                                buffer_json_member_add_time_t(wb, "ingest_age", ihs.ingest.since ? ihs.now - ihs.ingest.since : 0);
                                                buffer_json_member_add_string(wb, "ssl", ihs.ingest.ssl ? "SSL" : "PLAIN");
                                                buffer_json_member_add_uint64(wb, "alerts_critical",
                                                    ihs.health.status == RRDHOST_HEALTH_STATUS_RUNNING ? ihs.health.alerts.critical : 0);
                                                buffer_json_member_add_uint64(wb, "alerts_warning",
                                                    ihs.health.status == RRDHOST_HEALTH_STATUS_RUNNING ? ihs.health.alerts.warning : 0);
                                                buffer_json_object_close(wb);
                                            }
                                        }
                                    }
                                    buffer_json_array_close(wb); // inbound
                                }

                                // retention table: ALL nodes with DB retention (including localhost)
                                if(is_observer) {
                                    buffer_json_member_add_array(wb, "retention");
                                    {
                                        RRDHOST *rh;
                                        dfe_start_read(rrdhost_root_index, rh) {
                                            RRDHOST_STATUS rs;
                                            rrdhost_status(rh, now, &rs, RRDHOST_STATUS_ALL);

                                            if(!rs.db.first_time_s && !rs.db.last_time_s)
                                                continue;

                                            char rh_actor_id[256];
                                            streaming_topology_actor_id_for_host(rh, rh_actor_id, sizeof(rh_actor_id));

                                            buffer_json_add_array_item_object(wb);
                                            buffer_json_member_add_string(wb, "name", rrdhost_hostname(rh));
                                            buffer_json_member_add_string(wb, "name_id", rh_actor_id);
                                            buffer_json_member_add_string(wb, "db_status", rrdhost_db_status_to_string(rs.db.status));
                                            buffer_json_member_add_uint64(wb, "db_from", rs.db.first_time_s * MSEC_PER_SEC);
                                            buffer_json_member_add_uint64(wb, "db_to", rs.db.last_time_s * MSEC_PER_SEC);
                                            if(rs.db.first_time_s && rs.db.last_time_s && rs.db.last_time_s > rs.db.first_time_s)
                                                buffer_json_member_add_uint64(wb, "db_duration", rs.db.last_time_s - rs.db.first_time_s);
                                            else
                                                buffer_json_member_add_uint64(wb, "db_duration", 0);
                                            buffer_json_member_add_uint64(wb, "db_metrics", rs.db.metrics);
                                            buffer_json_member_add_uint64(wb, "db_instances", rs.db.instances);
                                            buffer_json_member_add_uint64(wb, "db_contexts", rs.db.contexts);
                                            buffer_json_object_close(wb);
                                        }
                                        dfe_done(rh);
                                    }
                                    buffer_json_array_close(wb); // retention
                                }
                            }

                            // outbound table: ALL nodes this actor streams to its parent
                            if(is_observer && s.stream.status != RRDHOST_STREAM_STATUS_DISABLED) {
                                bool stream_connected = (s.stream.status == RRDHOST_STREAM_STATUS_ONLINE ||
                                                         s.stream.status == RRDHOST_STREAM_STATUS_REPLICATING);

                                // find our streaming destination (only when connected)
                                const char *dst_hostname = NULL;
                                char dst_actor_id[256] = "";
                                if(stream_connected) {
                                    ND_UUID path_ids[16];
                                    uint16_t n_ids = rrdhost_stream_path_get_host_ids(host, 0, path_ids, 16);
                                    for(uint16_t pi = 0; pi < n_ids; pi++) {
                                        if(!UUIDeq(path_ids[pi], host->host_id)) {
                                            char guid[UUID_STR_LEN];
                                            uuid_unparse_lower(path_ids[pi].uuid, guid);
                                            streaming_topology_actor_id_from_guid(guid, dst_actor_id, sizeof(dst_actor_id));
                                            RRDHOST *dst_host = rrdhost_find_by_guid(guid);
                                            if(dst_host)
                                                dst_hostname = rrdhost_hostname(dst_host);
                                            break;
                                        }
                                    }
                                    if(!dst_hostname) dst_hostname = s.stream.peers.peer.ip;
                                }

                                buffer_json_member_add_array(wb, "outbound");
                                {
                                    RRDHOST *oh;
                                    dfe_start_read(rrdhost_root_index, oh) {
                                        char oh_actor_id[256];
                                        streaming_topology_actor_id_for_host(oh, oh_actor_id, sizeof(oh_actor_id));

                                        RRDHOST_STATUS ohs;
                                        rrdhost_status(oh, now, &ohs, RRDHOST_STATUS_ALL);

                                        // determine node type
                                        const char *oh_node_type;
                                        if(rrdhost_is_virtual(oh))
                                            oh_node_type = "vnode";
                                        else if(ohs.ingest.status == RRDHOST_INGEST_STATUS_ARCHIVED)
                                            oh_node_type = "stale";
                                        else {
                                            uint32_t *cc = streaming_topology_parent_child_count_get(parent_child_count, oh);
                                            oh_node_type = (cc && *cc > 0) ? "parent" : "child";
                                        }

                                        RRDHOST_STREAMING_STATUS oh_ss = (oh == host) ? s.stream.status : ohs.stream.status;
                                        bool oh_streaming = (oh_ss == RRDHOST_STREAM_STATUS_ONLINE || oh_ss == RRDHOST_STREAM_STATUS_REPLICATING);

                                        buffer_json_add_array_item_object(wb);
                                        buffer_json_member_add_string(wb, "name", rrdhost_hostname(oh));
                                        buffer_json_member_add_string(wb, "name_id", oh_actor_id);
                                        if(dst_hostname && oh_streaming) {
                                            buffer_json_member_add_string(wb, "streamed_to", dst_hostname);
                                            if(dst_actor_id[0])
                                                buffer_json_member_add_string(wb, "streamed_to_id", dst_actor_id);
                                        }
                                        buffer_json_member_add_string(wb, "node_type", oh_node_type);
                                        buffer_json_member_add_string(wb, "stream_status", rrdhost_streaming_status_to_string(oh_ss));

                                        if(oh == host && oh_streaming) {
                                            buffer_json_member_add_uint64(wb, "hops", s.stream.hops);
                                            buffer_json_member_add_string(wb, "ssl", s.stream.ssl ? "SSL" : "PLAIN");
                                            buffer_json_member_add_string(wb, "compression", s.stream.compression ? "COMPRESSED" : "UNCOMPRESSED");
                                        }

                                        buffer_json_object_close(wb);
                                    }
                                    dfe_done(oh);
                                }
                                buffer_json_array_close(wb); // outbound
                            }
                            else if(!is_observer && child_count > 0) {
                                // non-observer parent: use precomputed descendants
                                // find this parent's outbound destination (next hop in its own path)
                                const char *dst_hostname = NULL;

                                buffer_json_member_add_array(wb, "outbound");
                                {
                                    char dst_actor_id[256] = "";
                                    char dst_hostname_fallback[UUID_STR_LEN] = "";
                                    ND_UUID host_path[128];
                                    uint16_t host_pn = streaming_topology_get_path_ids(host, 0, host_path, 128);
                                    for(uint16_t pi = 0; pi < host_pn; pi++) {
                                        if(UUIDeq(host_path[pi], host->host_id) && pi + 1 < host_pn) {
                                            uuid_unparse_lower(host_path[pi + 1].uuid, dst_hostname_fallback);
                                            streaming_topology_actor_id_from_guid(dst_hostname_fallback, dst_actor_id, sizeof(dst_actor_id));
                                            RRDHOST *dst = rrdhost_find_by_guid(dst_hostname_fallback);
                                            dst_hostname = dst ? rrdhost_hostname(dst) : dst_hostname_fallback;
                                            break;
                                        }
                                    }

                                    struct streaming_topology_descendant_list *outbound_nodes =
                                        streaming_topology_descendants_get(parent_descendants, host);
                                    if(outbound_nodes) {
                                        for(size_t i = 0; i < outbound_nodes->used; i++) {
                                            RRDHOST *oh = outbound_nodes->items[i].host;
                                            char oh_actor_id[256];
                                            RRDHOST_STATUS ohs;
                                            streaming_topology_actor_id_for_host(oh, oh_actor_id, sizeof(oh_actor_id));

                                            rrdhost_status(oh, now, &ohs, RRDHOST_STATUS_ALL);

                                            const char *oh_node_type;
                                            if(rrdhost_is_virtual(oh))
                                                oh_node_type = "vnode";
                                            else if(ohs.ingest.status == RRDHOST_INGEST_STATUS_ARCHIVED)
                                                oh_node_type = "stale";
                                            else {
                                                uint32_t *cc = streaming_topology_parent_child_count_get(parent_child_count, oh);
                                                oh_node_type = (cc && *cc > 0) ? "parent" : "child";
                                            }

                                            RRDHOST_STREAMING_STATUS oh_ss = (oh == host) ? s.stream.status : ohs.stream.status;
                                            bool oh_streaming = (oh_ss == RRDHOST_STREAM_STATUS_ONLINE || oh_ss == RRDHOST_STREAM_STATUS_REPLICATING);

                                            buffer_json_add_array_item_object(wb);
                                            buffer_json_member_add_string(wb, "name", rrdhost_hostname(oh));
                                            buffer_json_member_add_string(wb, "name_id", oh_actor_id);
                                            if(dst_hostname && oh_streaming) {
                                                buffer_json_member_add_string(wb, "streamed_to", dst_hostname);
                                                if(dst_actor_id[0])
                                                    buffer_json_member_add_string(wb, "streamed_to_id", dst_actor_id);
                                            }
                                            buffer_json_member_add_string(wb, "node_type", oh_node_type);
                                            buffer_json_member_add_string(wb, "stream_status", rrdhost_streaming_status_to_string(oh_ss));
                                            if(oh == host && oh_streaming) {
                                                buffer_json_member_add_uint64(wb, "hops", s.stream.hops);
                                                buffer_json_member_add_string(wb, "ssl", s.stream.ssl ? "SSL" : "PLAIN");
                                                buffer_json_member_add_string(wb, "compression", s.stream.compression ? "COMPRESSED" : "UNCOMPRESSED");
                                            }
                                            buffer_json_object_close(wb);
                                        }
                                    }
                                }
                                buffer_json_array_close(wb); // outbound
                            }

                            // streaming_path table: per-hop data (uses the public API)
                            rrdhost_stream_path_to_json(wb, host, "streaming_path", false);

                            // retention table for non-parent actors: rows = observers (parents)
                            // in single-observer mode, 1 row from localhost
                            if(child_count == 0) {
                                buffer_json_member_add_array(wb, "retention");
                                {
                                    char observer_actor_id[256];
                                    streaming_topology_actor_id_for_host(localhost, observer_actor_id, sizeof(observer_actor_id));

                                    buffer_json_add_array_item_object(wb);
                                    buffer_json_member_add_string(wb, "name", rrdhost_hostname(localhost));
                                    buffer_json_member_add_string(wb, "name_id", observer_actor_id);
                                    buffer_json_member_add_string(wb, "db_status", rrdhost_db_status_to_string(s.db.status));
                                    buffer_json_member_add_uint64(wb, "db_from", s.db.first_time_s * MSEC_PER_SEC);
                                    buffer_json_member_add_uint64(wb, "db_to", s.db.last_time_s * MSEC_PER_SEC);
                                    if(s.db.first_time_s && s.db.last_time_s && s.db.last_time_s > s.db.first_time_s)
                                        buffer_json_member_add_uint64(wb, "db_duration", s.db.last_time_s - s.db.first_time_s);
                                    else
                                        buffer_json_member_add_uint64(wb, "db_duration", 0);
                                    buffer_json_member_add_uint64(wb, "db_metrics", s.db.metrics);
                                    buffer_json_member_add_uint64(wb, "db_instances", s.db.instances);
                                    buffer_json_member_add_uint64(wb, "db_contexts", s.db.contexts);
                                    buffer_json_object_close(wb);
                                }
                                buffer_json_array_close(wb); // retention
                            }
                        }
                        buffer_json_object_close(wb); // tables
                    }
                    buffer_json_object_close(wb); // actor
                }
                dfe_done(host);
            }

            // --- Bug C synthesis: emit "parent" actors for upstream entries
            // not represented by a host in rrdhost_root_index. The agent may
            // have STREAM_PATH metadata (hostname, hops, since, capabilities)
            // about upstream parents that are not locally registered as
            // RRDHOSTs (e.g., on a child viewing its own topology, the parent
            // is in localhost->stream.path.array but not in rrdhost_root_index).
            // This pass walks every host's stored path and emits a "parent"
            // actor for each unique upstream entry, with attributes sourced
            // directly from the path data. The merge layer fills missing
            // attributes from the parent's own response.
            {
                struct streaming_topology_synth_ctx synth = {
                    .wb = wb,
                    .local_actor_ids = local_actor_ids,
                    .emitted_actors = emitted_actors,
                    .emitted_links = emitted_links,
                    .actors_total = &actors_total,
                    .links_total = NULL,
                    .has_prev = false,
                };

                RRDHOST *sh;
                dfe_start_read(rrdhost_root_index, sh) {
                    // Walk from slot 1: slot 0 of any host's stored path is
                    // the host itself (origin) and is already emitted by
                    // Phase 3 if it is in rrdhost_root_index. Synthesizing
                    // parent actors only makes sense for upstream entries
                    // (slot >= 1).
                    rrdhost_stream_path_visit(sh, 1, streaming_topology_synth_actor_visitor, &synth);
                }
                dfe_done(sh);
            }

            buffer_json_array_close(wb); // actors

            // --- Phase 4: emit links from streaming_path ---
            // nodes with an active path get streaming/virtual links
            // nodes without a path (stale/offline) get a stale link to localhost
            char localhost_actor_id[256];
            streaming_topology_actor_id_for_host(localhost, localhost_actor_id, sizeof(localhost_actor_id));

            buffer_json_member_add_array(wb, "links");
            {
                RRDHOST *host;
                dfe_start_read(rrdhost_root_index, host) {
                    // skip localhost — it's the root of the tree
                    if(host == localhost)
                        continue;

                    RRDHOST_STATUS s;
                    rrdhost_status(host, now, &s, RRDHOST_STATUS_ALL);

                    char host_actor_id[256];
                    streaming_topology_actor_id_for_host(host, host_actor_id, sizeof(host_actor_id));

                    bool is_vnode = rrdhost_is_virtual(host);

                    // determine link target and type from streaming_path
                    const char *link_type = NULL;
                    char target_actor_id[256] = "";

                    ND_UUID link_ids[2];
                    uint16_t link_n = streaming_topology_get_path_ids(host, 0, link_ids, 2);

                    if(is_vnode) {
                        // vnodes do not stream; they are collected by localhost.
                        snprintfz(target_actor_id, sizeof(target_actor_id), "%s", localhost_actor_id);
                        link_type = "virtual";
                    }
                    else if(link_n >= 2) {
                        // children/parents: path[1] is the direct parent
                        streaming_topology_actor_id_for_uuid(link_ids[1], target_actor_id, sizeof(target_actor_id));
                        link_type = "streaming";
                    }
                    else {
                        // no active path — stale link to localhost
                        snprintfz(target_actor_id, sizeof(target_actor_id), "%s", localhost_actor_id);
                        link_type = "stale";
                    }

                    const char *hostname = rrdhost_hostname(host);

                    char link_key[600];
                    snprintfz(link_key, sizeof(link_key), "%s|%s", host_actor_id, target_actor_id);
                    if(dictionary_get(emitted_links, link_key))
                        continue;

                    {
                        uint8_t one = 1;
                        dictionary_set(emitted_links, link_key, &one, sizeof(one));
                    }

                    links_total++;
                    buffer_json_add_array_item_object(wb);
                    {
                        buffer_json_member_add_string(wb, "layer", "infra");
                        buffer_json_member_add_string(wb, "protocol", "streaming");
                        buffer_json_member_add_string(wb, "link_type", link_type);
                        buffer_json_member_add_string(wb, "src_actor_id", host_actor_id);
                        buffer_json_member_add_string(wb, "dst_actor_id", target_actor_id);
                        buffer_json_member_add_string(wb, "state", rrdhost_ingest_status_to_string(s.ingest.status));
                        buffer_json_member_add_datetime_rfc3339(wb, "discovered_at",
                            ((uint64_t)(s.ingest.since ? s.ingest.since : now)) * USEC_PER_SEC, true);
                        buffer_json_member_add_datetime_rfc3339(wb, "last_seen", now_ut, true);

                        buffer_json_member_add_object(wb, "dst");
                        {
                            buffer_json_member_add_object(wb, "attributes");
                            {
                                buffer_json_member_add_string(wb, "port_name", hostname);
                            }
                            buffer_json_object_close(wb);
                        }
                        buffer_json_object_close(wb);

                        buffer_json_member_add_object(wb, "metrics");
                        {
                            buffer_json_member_add_uint64(wb, "hops", s.ingest.hops);
                            if(strcmp(link_type, "virtual") != 0) {
                                buffer_json_member_add_uint64(wb, "connections", s.host->stream.rcv.status.connections);
                                buffer_json_member_add_uint64(wb, "replication_instances", s.ingest.replication.instances);
                                buffer_json_member_add_double(wb, "replication_completion", s.ingest.replication.completion);
                                buffer_json_member_add_uint64(wb, "collected_metrics", s.ingest.collected.metrics);
                                buffer_json_member_add_uint64(wb, "collected_instances", s.ingest.collected.instances);
                                buffer_json_member_add_uint64(wb, "collected_contexts", s.ingest.collected.contexts);
                            }
                        }
                        buffer_json_object_close(wb); // metrics
                    }
                    buffer_json_object_close(wb); // link
                }
                dfe_done(host);
            }

            // --- Bug C link synthesis: emit streaming links for consecutive
            // path slots not already emitted by Phase 4. Phase 4 writes one
            // direct link per local non-localhost actor and registers it in
            // emitted_links; this pass adds localhost's own upstream link and
            // any deeper multi-hop links.
            // discovered_at / last_seen derive from STREAM_PATH timestamps so
            // the merge layer can reconcile views from multiple agents.
            {
                struct streaming_topology_synth_ctx synth = {
                    .wb = wb,
                    .local_actor_ids = local_actor_ids,
                    .emitted_actors = emitted_actors,
                    .emitted_links = emitted_links,
                    .actors_total = NULL,
                    .links_total = &links_total,
                    .has_prev = false,
                };

                RRDHOST *lh;
                dfe_start_read(rrdhost_root_index, lh) {
                    synth.has_prev = false;
                    rrdhost_stream_path_visit(lh, 0, streaming_topology_synth_link_visitor, &synth);
                }
                dfe_done(lh);
            }

            buffer_json_array_close(wb); // links

            buffer_json_member_add_object(wb, "stats");
            {
                buffer_json_member_add_uint64(wb, "actors_total", actors_total);
                buffer_json_member_add_uint64(wb, "links_total", links_total);
            }
            buffer_json_object_close(wb); // stats
        }
            buffer_json_object_close(wb); // data

            struct streaming_topology_descendant_list *descendants;
            dfe_start_write(parent_descendants, descendants) {
                freez(descendants->items);
                descendants->items = NULL;
                descendants->used = 0;
                descendants->size = 0;
            }
            dfe_done(descendants);
            dictionary_destroy(parent_descendants);
            dictionary_destroy(parent_child_count);
            dictionary_destroy(local_actor_ids);
            dictionary_destroy(emitted_actors);
            dictionary_destroy(emitted_links);
        }
    }

    buffer_json_member_add_time_t(wb, "expires", now_realtime_sec() + STREAMING_FUNCTION_UPDATE_EVERY);
    buffer_json_finalize(wb);
    freez(function_copy);
    return HTTP_RESP_OK;
}
