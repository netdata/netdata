// SPDX-License-Identifier: GPL-3.0-or-later

#include "function-streaming.h"

#include <ctype.h>

#define STREAMING_FUNCTION_UPDATE_EVERY 10

#define GROUP_BY_COLUMN(name, descr) \
    buffer_json_member_add_object(wb, name);\
    {\
        buffer_json_member_add_string(wb, "name", descr);\
        buffer_json_member_add_array(wb, "columns");\
        {\
            buffer_json_add_array_item_string(wb, name);\
        }\
        buffer_json_array_close(wb);\
    }\
    buffer_json_object_close(wb);

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

struct streaming_topology_filters {
    bool info_only;
    const char *node_type;
    const char *ingest_status;
    const char *stream_status;
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

    // append localhost if not found (same as rrdhost_stream_path_to_json)
    if(!found_localhost && n < max && n > 0)
        host_ids[n++] = localhost->host_id;

    return n;
}

static void streaming_topology_parse_filters(const char *function, struct streaming_topology_filters *filters) {
    if(!filters)
        return;

    *filters = (struct streaming_topology_filters){ 0 };
    if(!function || !*function)
        return;

    filters->function_copy = strdupz(function);
    char *words[1024];
    size_t num_words = quoted_strings_splitter_whitespace(filters->function_copy, words, 1024);
    for(size_t i = 1; i < num_words; i++) {
        char *param = get_word(words, num_words, i);
        if(strcmp(param, "info") == 0)
            filters->info_only = true;
        else if(strncmp(param, "node_type:", 10) == 0)
            filters->node_type = param + 10;
        else if(strncmp(param, "ingest_status:", 14) == 0)
            filters->ingest_status = param + 14;
        else if(strncmp(param, "stream_status:", 14) == 0)
            filters->stream_status = param + 14;
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

// check if a value appears in a comma-separated list (NULL list matches everything)
static bool value_in_csv(const char *csv, const char *value) {
    if(!csv || !*csv) return true;
    if(!value || !*value) return false;
    size_t vlen = strlen(value);
    const char *p = csv;
    while(*p) {
        const char *comma = strchr(p, ',');
        size_t len = comma ? (size_t)(comma - p) : strlen(p);
        while(len && isspace((unsigned char)*p)) {
            p++;
            len--;
        }
        while(len && isspace((unsigned char)p[len - 1]))
            len--;
        if(len == vlen && strncmp(p, value, len) == 0)
            return true;
        if(!comma) break;
        p = comma + 1;
    }
    return false;
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
            streaming_topology_emit_col(wb, "since", "Since", "number");
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

int function_streaming_topology(BUFFER *wb, const char *function, BUFFER *payload __maybe_unused, const char *source __maybe_unused) {
    time_t now = now_realtime_sec();
    usec_t now_ut = now_realtime_usec();

    struct streaming_topology_filters filters = { 0 };
    streaming_topology_parse_filters(function, &filters);
    bool info_only = filters.info_only;
    const char *filter_node_type = filters.node_type;
    const char *filter_ingest_status = filters.ingest_status;
    const char *filter_stream_status = filters.stream_status;
    char *function_copy = filters.function_copy;

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
        buffer_json_add_array_item_string(wb, "node_type");
        buffer_json_add_array_item_string(wb, "ingest_status");
        buffer_json_add_array_item_string(wb, "stream_status");
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

        if(!parent_child_count || !parent_descendants) {
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

                    if(rrdhost_is_virtual(host)) {
                        if(full_path_n > 0) {
                            streaming_topology_descendants_append(parent_descendants,
                                full_path_ids[0], host, STREAMING_TOPOLOGY_RECEIVED_VIRTUAL, true, empty_uuid);

                            for(uint16_t i = 1; i < full_path_n; i++) {
                                streaming_topology_descendants_append(parent_descendants,
                                    full_path_ids[i], host, STREAMING_TOPOLOGY_RECEIVED_STREAMING, false, full_path_ids[i - 1]);
                            }
                        }
                    }
                    else if(full_path_n > 0) {
                        for(uint16_t i = 0; i < full_path_n; i++) {
                            bool source_local = (i == 0);
                            ND_UUID source_uuid = source_local ? empty_uuid : full_path_ids[i - 1];
                            streaming_topology_descendants_append(parent_descendants,
                                full_path_ids[i], host, STREAMING_TOPOLOGY_RECEIVED_STREAMING, source_local, source_uuid);
                        }
                    }
                    else if(host != localhost) {
                        streaming_topology_descendants_append(parent_descendants,
                            localhost->host_id, host, STREAMING_TOPOLOGY_RECEIVED_STALE, false, empty_uuid);
                    }
                }
                dfe_done(host);
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

                    // apply filters
                    const char *ingest_status_str = rrdhost_ingest_status_to_string(s.ingest.status);
                    const char *stream_status_str = rrdhost_streaming_status_to_string(s.stream.status);

                    if(!value_in_csv(filter_node_type, node_type))
                        continue;
                    if(!value_in_csv(filter_ingest_status, ingest_status_str))
                        continue;
                    if(!value_in_csv(filter_stream_status, stream_status_str))
                        continue;

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
                            // topology-specific labels (used for filtering)
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

                    // apply same filters as actors
                    const char *node_type;
                    if(rrdhost_is_virtual(host))
                        node_type = "vnode";
                    else if(s.ingest.status == RRDHOST_INGEST_STATUS_ARCHIVED)
                        node_type = "stale";
                    else {
                        uint32_t *cc = streaming_topology_parent_child_count_get(parent_child_count, host);
                        node_type = (cc && *cc > 0) ? "parent" : "child";
                    }
                    if(!value_in_csv(filter_node_type, node_type))
                        continue;
                    if(!value_in_csv(filter_ingest_status, rrdhost_ingest_status_to_string(s.ingest.status)))
                        continue;
                    if(!value_in_csv(filter_stream_status, rrdhost_streaming_status_to_string(s.stream.status)))
                        continue;

                    char host_actor_id[256];
                    streaming_topology_actor_id_for_host(host, host_actor_id, sizeof(host_actor_id));

                    bool is_vnode = rrdhost_is_virtual(host);

                    // determine link target and type from streaming_path
                    const char *link_type = NULL;
                    char target_actor_id[256] = "";

                    ND_UUID link_ids[2];
                    uint16_t link_n = streaming_topology_get_path_ids(host, 0, link_ids, 2);

                    if(is_vnode && link_n >= 2) {
                        // vnodes: path[0] is the originating agent
                        streaming_topology_actor_id_for_uuid(link_ids[0], target_actor_id, sizeof(target_actor_id));
                        link_type = "virtual";
                    }
                    else if(!is_vnode && link_n >= 2) {
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
        }
    }

    buffer_json_member_add_time_t(wb, "expires", now_realtime_sec() + STREAMING_FUNCTION_UPDATE_EVERY);
    buffer_json_finalize(wb);
    freez(function_copy);
    return HTTP_RESP_OK;
}


int function_streaming(BUFFER *wb, const char *function __maybe_unused, BUFFER *payload __maybe_unused, const char *source __maybe_unused) {

    time_t now = now_realtime_sec();

    buffer_flush(wb);
    wb->content_type = CT_APPLICATION_JSON;
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);

    buffer_json_member_add_string(wb, "hostname", rrdhost_hostname(localhost));
    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_string(wb, "type", "table");
    buffer_json_member_add_time_t(wb, "update_every", STREAMING_FUNCTION_UPDATE_EVERY);
    buffer_json_member_add_boolean(wb, "has_history", false);
    buffer_json_member_add_string(wb, "help", RRDFUNCTIONS_STREAMING_HELP);
    buffer_json_member_add_array(wb, "data");

    size_t max_sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_MAX] = { 0 };
    size_t max_db_metrics = 0, max_db_instances = 0, max_db_contexts = 0;
    size_t max_collection_replication_instances = 0, max_streaming_replication_instances = 0;
    size_t max_ml_anomalous = 0, max_ml_normal = 0, max_ml_trained = 0, max_ml_pending = 0, max_ml_silenced = 0;

    time_t
        max_db_duration = 0,
        max_db_from = 0,
        max_db_to = 0,
        max_in_age = 0,
        max_out_age = 0,
        max_out_attempt_age = 0;

    uint64_t
        max_in_since = 0,
        max_out_since = 0,
        max_out_attempt_since = 0;

    int16_t
        max_in_hops = -1,
        max_out_hops = -1;

    int
        max_in_local_port = 0,
        max_in_remote_port = 0,
        max_out_local_port = 0,
        max_out_remote_port = 0;

    uint32_t
        max_in_connections = 0,
        max_out_connections = 0;

    {
        RRDHOST *host;
        dfe_start_read(rrdhost_root_index, host) {
            RRDHOST_STATUS s;
            rrdhost_status(host, now, &s, RRDHOST_STATUS_ALL);
            buffer_json_add_array_item_array(wb);

            if(s.db.metrics > max_db_metrics)
                max_db_metrics = s.db.metrics;

            if(s.db.instances > max_db_instances)
                max_db_instances = s.db.instances;

            if(s.db.contexts > max_db_contexts)
                max_db_contexts = s.db.contexts;

            if(s.ingest.replication.instances > max_collection_replication_instances)
                max_collection_replication_instances = s.ingest.replication.instances;

            if(s.stream.replication.instances > max_streaming_replication_instances)
                max_streaming_replication_instances = s.stream.replication.instances;

            for(int i = 0; i < STREAM_TRAFFIC_TYPE_MAX ;i++) {
                if (s.stream.sent_bytes_on_this_connection_per_type[i] >
                    max_sent_bytes_on_this_connection_per_type[i])
                    max_sent_bytes_on_this_connection_per_type[i] =
                        s.stream.sent_bytes_on_this_connection_per_type[i];
            }

            // Node
            buffer_json_add_array_item_string(wb, rrdhost_hostname(s.host));

            // rowOptions
            buffer_json_add_array_item_object(wb);
            {
                const char *severity = NULL; // normal, debug, notice, warning, critical
                if(!rrdhost_option_check(host, RRDHOST_OPTION_EPHEMERAL_HOST)) {
                    switch(s.ingest.status) {
                        case RRDHOST_INGEST_STATUS_OFFLINE:
                        case RRDHOST_INGEST_STATUS_ARCHIVED:
                            severity = "critical";
                            break;

                        default:
                        case RRDHOST_INGEST_STATUS_INITIALIZING:
                        case RRDHOST_INGEST_STATUS_ONLINE:
                        case RRDHOST_INGEST_STATUS_REPLICATING:
                            break;
                    }

                    switch(s.stream.status) {
                        case RRDHOST_STREAM_STATUS_OFFLINE:
                            if(!severity && s.stream.reason != STREAM_HANDSHAKE_SP_NO_DESTINATION)
                                severity = "warning";
                            break;

                        default:
                        case RRDHOST_STREAM_STATUS_REPLICATING:
                        case RRDHOST_STREAM_STATUS_ONLINE:
                            break;
                    }
                }
                buffer_json_member_add_string(wb, "severity", severity ? severity : "normal");
            }
            buffer_json_object_close(wb); // rowOptions

            // Ephemerality
            buffer_json_add_array_item_string(wb, rrdhost_option_check(s.host, RRDHOST_OPTION_EPHEMERAL_HOST) ? "ephemeral" : "permanent");

            // AgentName and AgentVersion
            buffer_json_add_array_item_string(wb, rrdhost_program_name(host));
            buffer_json_add_array_item_string(wb, rrdhost_program_version(host));

            // System Info
            rrdhost_system_info_to_streaming_function_array(wb, s.host->system_info);

            // retention
            buffer_json_add_array_item_uint64(wb, s.db.first_time_s * MSEC_PER_SEC); // dbFrom
            if(s.db.first_time_s > max_db_from) max_db_from = s.db.first_time_s;

            buffer_json_add_array_item_uint64(wb, s.db.last_time_s * MSEC_PER_SEC); // dbTo
            if(s.db.last_time_s > max_db_to) max_db_to = s.db.last_time_s;

            if(s.db.first_time_s && s.db.last_time_s && s.db.last_time_s > s.db.first_time_s) {
                time_t db_duration = s.db.last_time_s - s.db.first_time_s;
                buffer_json_add_array_item_uint64(wb, db_duration); // dbDuration
                if(db_duration > max_db_duration) max_db_duration = db_duration;
            }
            else
                buffer_json_add_array_item_string(wb, NULL); // dbDuration

            buffer_json_add_array_item_uint64(wb, s.db.metrics); // dbMetrics
            buffer_json_add_array_item_uint64(wb, s.db.instances); // dbInstances
            buffer_json_add_array_item_uint64(wb, s.db.contexts); // dbContexts

            // statuses
            buffer_json_add_array_item_string(wb, rrdhost_ingest_status_to_string(s.ingest.status)); // InStatus
            buffer_json_add_array_item_string(wb, rrdhost_streaming_status_to_string(s.stream.status)); // OutStatus
            buffer_json_add_array_item_string(wb, rrdhost_ml_status_to_string(s.ml.status)); // MLStatus

            // collection

            // InConnections
            buffer_json_add_array_item_uint64(wb, s.host->stream.rcv.status.connections);
            if(s.host->stream.rcv.status.connections > max_in_connections)
                max_in_connections = s.host->stream.rcv.status.connections;

            if(s.ingest.since) {
                uint64_t in_since = s.ingest.since * MSEC_PER_SEC;
                buffer_json_add_array_item_uint64(wb, in_since); // InSince
                if(in_since > max_in_since) max_in_since = in_since;

                time_t in_age = s.now - s.ingest.since;
                buffer_json_add_array_item_time_t(wb, in_age); // InAge
                if(in_age > max_in_age) max_in_age = in_age;
            }
            else {
                buffer_json_add_array_item_string(wb, NULL); // InSince
                buffer_json_add_array_item_string(wb, NULL); // InAge
            }

            // InReason
            if(s.ingest.type == RRDHOST_INGEST_TYPE_LOCALHOST)
                buffer_json_add_array_item_string(wb, "LOCALHOST");
            else if(s.ingest.type == RRDHOST_INGEST_TYPE_VIRTUAL)
                buffer_json_add_array_item_string(wb, "VIRTUAL NODE");
            else
                buffer_json_add_array_item_string(wb, stream_handshake_error_to_string(s.ingest.reason));

            buffer_json_add_array_item_int64(wb, s.ingest.hops); // InHops
            if(s.ingest.hops > max_in_hops) max_in_hops = s.ingest.hops;

            buffer_json_add_array_item_double(wb, s.ingest.replication.completion); // InReplCompletion
            buffer_json_add_array_item_uint64(wb, s.ingest.replication.instances); // InReplInstances
            buffer_json_add_array_item_string(wb, s.ingest.type == RRDHOST_INGEST_TYPE_LOCALHOST || s.ingest.type == RRDHOST_INGEST_TYPE_VIRTUAL ? "localhost" : s.ingest.peers.local.ip); // InLocalIP

            buffer_json_add_array_item_uint64(wb, s.ingest.peers.local.port); // InLocalPort
            if(s.ingest.peers.local.port > max_in_local_port) max_in_local_port = s.ingest.peers.local.port;

            buffer_json_add_array_item_string(wb, s.ingest.peers.peer.ip); // InRemoteIP
            buffer_json_add_array_item_uint64(wb, s.ingest.peers.peer.port); // InRemotePort
            if(s.ingest.peers.peer.port > max_in_remote_port) max_in_remote_port = s.ingest.peers.peer.port;

            buffer_json_add_array_item_string(wb, s.ingest.ssl ? "SSL" : "PLAIN"); // InSSL
            stream_capabilities_to_json_array(wb, s.ingest.capabilities, NULL); // InCapabilities

            buffer_json_add_array_item_uint64(wb, s.ingest.collected.metrics); // CollectedMetrics
            buffer_json_add_array_item_uint64(wb, s.ingest.collected.instances); // CollectedInstances
            buffer_json_add_array_item_uint64(wb, s.ingest.collected.contexts); // CollectedContexts

            // streaming

            // OutConnections
            buffer_json_add_array_item_uint64(wb, s.host->stream.snd.status.connections);
            if(s.host->stream.snd.status.connections > max_out_connections)
                max_out_connections = s.host->stream.snd.status.connections;

            if(s.stream.since) {
                uint64_t out_since = s.stream.since * MSEC_PER_SEC;
                buffer_json_add_array_item_uint64(wb, out_since); // OutSince
                if(out_since > max_out_since) max_out_since = out_since;

                time_t out_age = s.now - s.stream.since;
                buffer_json_add_array_item_time_t(wb, out_age); // OutAge
                if(out_age > max_out_age) max_out_age = out_age;
            }
            else {
                buffer_json_add_array_item_string(wb, NULL); // OutSince
                buffer_json_add_array_item_string(wb, NULL); // OutAge
            }
            buffer_json_add_array_item_string(wb, stream_handshake_error_to_string(s.stream.reason)); // OutReason

            buffer_json_add_array_item_int64(wb, s.stream.hops); // OutHops
            if(s.stream.hops > max_out_hops) max_out_hops = s.stream.hops;

            buffer_json_add_array_item_double(wb, s.stream.replication.completion); // OutReplCompletion
            buffer_json_add_array_item_uint64(wb, s.stream.replication.instances); // OutReplInstances
            buffer_json_add_array_item_string(wb, s.stream.peers.local.ip); // OutLocalIP
            buffer_json_add_array_item_uint64(wb, s.stream.peers.local.port); // OutLocalPort
            if(s.stream.peers.local.port > max_out_local_port) max_out_local_port = s.stream.peers.local.port;

            buffer_json_add_array_item_string(wb, s.stream.peers.peer.ip); // OutRemoteIP
            buffer_json_add_array_item_uint64(wb, s.stream.peers.peer.port); // OutRemotePort
            if(s.stream.peers.peer.port > max_out_remote_port) max_out_remote_port = s.stream.peers.peer.port;

            buffer_json_add_array_item_string(wb, s.stream.ssl ? "SSL" : "PLAIN"); // OutSSL
            buffer_json_add_array_item_string(wb, s.stream.compression ? "COMPRESSED" : "UNCOMPRESSED"); // OutCompression
            stream_capabilities_to_json_array(wb, s.stream.capabilities, NULL); // OutCapabilities
            buffer_json_add_array_item_uint64(wb, s.stream.sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_DATA]);
            buffer_json_add_array_item_uint64(wb, s.stream.sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_METADATA]);
            buffer_json_add_array_item_uint64(wb, s.stream.sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_REPLICATION]);
            buffer_json_add_array_item_uint64(wb, s.stream.sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_FUNCTIONS]);

            buffer_json_add_array_item_array(wb); // OutAttemptHandshake
            usec_t last_attempt = stream_parent_handshake_error_to_json(wb, host);
            buffer_json_array_close(wb); // // OutAttemptHandshake

            if(!last_attempt) {
                buffer_json_add_array_item_string(wb, NULL); // OutAttemptSince
                buffer_json_add_array_item_string(wb, NULL); // OutAttemptAge
            }
            else {
                uint64_t out_attempt_since = last_attempt / USEC_PER_MS;
                buffer_json_add_array_item_uint64(wb, out_attempt_since); // OutAttemptSince
                if(out_attempt_since > max_out_attempt_since) max_out_attempt_since = out_attempt_since;

                time_t out_attempt_age = s.now - (time_t)(last_attempt / USEC_PER_SEC);
                buffer_json_add_array_item_time_t(wb, out_attempt_age); // OutAttemptAge
                if(out_attempt_age > max_out_attempt_age) max_out_attempt_age = out_attempt_age;
            }

            // ML
            if(s.ml.status == RRDHOST_ML_STATUS_RUNNING) {
                buffer_json_add_array_item_uint64(wb, s.ml.metrics.anomalous); // MlAnomalous
                buffer_json_add_array_item_uint64(wb, s.ml.metrics.normal); // MlNormal
                buffer_json_add_array_item_uint64(wb, s.ml.metrics.trained); // MlTrained
                buffer_json_add_array_item_uint64(wb, s.ml.metrics.pending); // MlPending
                buffer_json_add_array_item_uint64(wb, s.ml.metrics.silenced); // MlSilenced

                if(s.ml.metrics.anomalous > max_ml_anomalous)
                    max_ml_anomalous = s.ml.metrics.anomalous;

                if(s.ml.metrics.normal > max_ml_normal)
                    max_ml_normal = s.ml.metrics.normal;

                if(s.ml.metrics.trained > max_ml_trained)
                    max_ml_trained = s.ml.metrics.trained;

                if(s.ml.metrics.pending > max_ml_pending)
                    max_ml_pending = s.ml.metrics.pending;

                if(s.ml.metrics.silenced > max_ml_silenced)
                    max_ml_silenced = s.ml.metrics.silenced;

            }
            else {
                buffer_json_add_array_item_string(wb, NULL); // MlAnomalous
                buffer_json_add_array_item_string(wb, NULL); // MlNormal
                buffer_json_add_array_item_string(wb, NULL); // MlTrained
                buffer_json_add_array_item_string(wb, NULL); // MlPending
                buffer_json_add_array_item_string(wb, NULL); // MlSilenced
            }

            // close
            buffer_json_array_close(wb);
        }
        dfe_done(host);
    }
    buffer_json_array_close(wb); // data
    buffer_json_member_add_object(wb, "columns");
    {
        size_t field_id = 0;

        // Node
        buffer_rrdf_table_add_field(wb, field_id++, "Node", "Node's Hostname",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_UNIQUE_KEY | RRDF_FIELD_OPTS_STICKY,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "rowOptions", "rowOptions",
                                    RRDF_FIELD_TYPE_NONE, RRDR_FIELD_VISUAL_ROW_OPTIONS, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_FIXED, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE, RRDF_FIELD_OPTS_DUMMY,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Ephemerality", "The type of ephemerality for the node",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "AgentName", "The name of the Netdata agent",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "AgentVersion", "The version of the Netdata agent",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OSName", "The name of the host's operating system",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OSId", "The identifier of the host's operating system",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OSIdLike", "The ID-like string for the host's OS",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OSVersion", "The version of the host's operating system",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OSVersionId", "The version identifier of the host's OS",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OSDetection", "Details about host OS detection",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "CPUCores", "The number of CPU cores in the host",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "DiskSpace", "The total disk space available on the host",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "CPUFreq", "The CPU frequency of the host",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "RAMTotal", "The total RAM available on the host",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "ContainerOSName", "The name of the container's operating system",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "ContainerOSId", "The identifier of the container's operating system",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "ContainerOSIdLike", "The ID-like string for the container's OS",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "ContainerOSVersion", "The version of the container's OS",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "ContainerOSVersionId", "The version identifier of the container's OS",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "ContainerOSDetection", "Details about container OS detection",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "IsK8sNode", "Whether this node is part of a Kubernetes cluster",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "KernelName", "The kernel name",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "KernelVersion", "The kernel version",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Architecture", "The system architecture",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Virtualization", "The virtualization technology in use",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "VirtDetection", "Details about virtualization detection",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Container", "Container type information",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "ContainerDetection", "Details about container detection",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "CloudProviderType", "The type of cloud provider",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "CloudInstanceType", "The type of cloud instance",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "CloudInstanceRegion", "The region of the cloud instance",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "dbFrom", "DB Data Retention From",
                                    RRDF_FIELD_TYPE_TIMESTAMP, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_DATETIME_MS,
                                    0, NULL, (double)max_db_from * MSEC_PER_SEC, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MIN, RRDF_FIELD_FILTER_NONE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "dbTo", "DB Data Retention To",
                                    RRDF_FIELD_TYPE_TIMESTAMP, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_DATETIME_MS,
                                    0, NULL, (double)max_db_to * MSEC_PER_SEC, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MAX, RRDF_FIELD_FILTER_NONE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "dbDuration", "DB Data Retention Duration",
                                    RRDF_FIELD_TYPE_DURATION, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_DURATION_S,
                                    0, NULL, (double)max_db_duration, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MAX, RRDF_FIELD_FILTER_NONE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "dbMetrics", "Time-series Metrics in the DB",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, NULL, (double)max_db_metrics, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "dbInstances", "Instances in the DB",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, NULL, (double)max_db_instances, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "dbContexts", "Contexts in the DB",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, NULL, (double)max_db_contexts, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        // --- statuses ---

        buffer_rrdf_table_add_field(wb, field_id++, "InStatus", "Data Collection Online Status",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);


        buffer_rrdf_table_add_field(wb, field_id++, "OutStatus", "Streaming Online Status",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "MlStatus", "ML Status",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        // --- collection ---

        buffer_rrdf_table_add_field(wb, field_id++, "InConnections", "Number of times this child connected",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, (double)max_in_connections, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "InSince", "Last Data Collection Status Change",
                                    RRDF_FIELD_TYPE_TIMESTAMP, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_DATETIME_MS,
                                    0, NULL, (double)max_in_since, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MIN, RRDF_FIELD_FILTER_NONE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "InAge", "Last Data Collection Online Status Change Age",
                                    RRDF_FIELD_TYPE_DURATION, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_DURATION_S,
                                    0, NULL, (double)max_in_age, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MAX, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "InReason", "Data Collection Online Status Reason",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "InHops", "Data Collection Distance Hops from Origin Node",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, (double)max_in_hops, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MIN, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "InReplCompletion", "Inbound Replication Completion",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                                    1, "%", 100.0, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MIN, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "InReplInstances", "Inbound Replicating Instances",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "instances", (double)max_collection_replication_instances, RRDF_FIELD_SORT_DESCENDING,
                                    NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "InLocalIP", "Inbound Local IP",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "InLocalPort", "Inbound Local Port",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, NULL, (double)max_in_local_port, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "InRemoteIP", "Inbound Remote IP",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "InRemotePort", "Inbound Remote Port",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, NULL, (double)max_in_remote_port, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "InSSL", "Inbound SSL Connection",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "InCapabilities", "Inbound Connection Capabilities",
                                    RRDF_FIELD_TYPE_ARRAY, RRDF_FIELD_VISUAL_PILL, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "CollectedMetrics", "Time-series Metrics Currently Collected",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, NULL, (double)max_db_metrics, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "CollectedInstances", "Instances Currently Collected",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, NULL, (double)max_db_instances, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "CollectedContexts", "Contexts Currently Collected",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, NULL, (double)max_db_contexts, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        // --- streaming ---

        buffer_rrdf_table_add_field(wb, field_id++, "OutConnections", "Number of times connected to a parent",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, (double)max_out_connections, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutSince", "Last Streaming Status Change",
                                    RRDF_FIELD_TYPE_TIMESTAMP, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_DATETIME_MS,
                                    0, NULL, (double)max_out_since, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MAX, RRDF_FIELD_FILTER_NONE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutAge", "Last Streaming Status Change Age",
                                    RRDF_FIELD_TYPE_DURATION, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_DURATION_S,
                                    0, NULL, (double)max_out_age, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MIN, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutReason", "Streaming Status Reason",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutHops", "Streaming Distance Hops from Origin Node",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, (double)max_out_hops, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MIN, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutReplCompletion", "Outbound Replication Completion",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                                    1, "%", 100.0, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MIN, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutReplInstances", "Outbound Replicating Instances",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "instances", (double)max_streaming_replication_instances, RRDF_FIELD_SORT_DESCENDING,
                                    NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutLocalIP", "Outbound Local IP",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutLocalPort", "Outbound Local Port",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutRemoteIP", "Outbound Remote IP",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutRemotePort", "Outbound Remote Port",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, NULL, (double)max_out_remote_port, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutSSL", "Outbound SSL Connection",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutCompression", "Outbound Compressed Connection",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutCapabilities", "Outbound Connection Capabilities",
                                    RRDF_FIELD_TYPE_ARRAY, RRDF_FIELD_VISUAL_PILL, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutTrafficData", "Outbound Metric Data Traffic",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "bytes", (double)max_sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_DATA],
                                    RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutTrafficMetadata", "Outbound Metric Metadata Traffic",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "bytes",
                                    (double)max_sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_METADATA],
                                    RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutTrafficReplication", "Outbound Metric Replication Traffic",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "bytes",
                                    (double)max_sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_REPLICATION],
                                    RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutTrafficFunctions", "Outbound Metric Functions Traffic",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "bytes",
                                    (double)max_sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_FUNCTIONS],
                                    RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutAttemptHandshake",
                                    "Outbound Connection Attempt Handshake Status",
                                    RRDF_FIELD_TYPE_ARRAY, RRDF_FIELD_VISUAL_PILL, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutAttemptSince",
                                    "Last Outbound Connection Attempt Status Change Time",
                                    RRDF_FIELD_TYPE_TIMESTAMP, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_DATETIME_MS,
                                    0, NULL, (double)max_out_attempt_since, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MAX, RRDF_FIELD_FILTER_NONE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutAttemptAge",
                                    "Last Outbound Connection Attempt Status Change Age",
                                    RRDF_FIELD_TYPE_DURATION, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_DURATION_S,
                                    0, NULL, (double)max_out_attempt_age, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MIN, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        // --- ML ---

        buffer_rrdf_table_add_field(wb, field_id++, "MlAnomalous", "Number of Anomalous Metrics",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "metrics",
                                    (double)max_ml_anomalous,
                                    RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "MlNormal", "Number of Not Anomalous Metrics",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "metrics",
                                    (double)max_ml_normal,
                                    RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "MlTrained", "Number of Trained Metrics",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "metrics",
                                    (double)max_ml_trained,
                                    RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "MlPending", "Number of Pending Metrics",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "metrics",
                                    (double)max_ml_pending,
                                    RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "MlSilenced", "Number of Silenced Metrics",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "metrics",
                                    (double)max_ml_silenced,
                                    RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
    }
    buffer_json_object_close(wb); // columns
    buffer_json_member_add_string(wb, "default_sort_column", "Node");
    buffer_json_member_add_object(wb, "charts");
    {
        // Data Collection Age chart
        buffer_json_member_add_object(wb, "InAge");
        {
            buffer_json_member_add_string(wb, "name", "Data Collection Age");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "InAge");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        // Streaming Age chart
        buffer_json_member_add_object(wb, "OutAge");
        {
            buffer_json_member_add_string(wb, "name", "Streaming Age");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "OutAge");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        // DB Duration
        buffer_json_member_add_object(wb, "dbDuration");
        {
            buffer_json_member_add_string(wb, "name", "Retention Duration");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "dbDuration");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb); // charts

    buffer_json_member_add_array(wb, "default_charts");
    {
        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_string(wb, "InAge");
        buffer_json_add_array_item_string(wb, "Node");
        buffer_json_array_close(wb);

        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_string(wb, "OutAge");
        buffer_json_add_array_item_string(wb, "Node");
        buffer_json_array_close(wb);
    }
    buffer_json_array_close(wb);

    buffer_json_member_add_object(wb, "group_by");
    {
        GROUP_BY_COLUMN("OSName", "O/S Name");
        GROUP_BY_COLUMN("OSId", "O/S ID");
        GROUP_BY_COLUMN("OSIdLike", "O/S ID Like");
        GROUP_BY_COLUMN("OSVersion", "O/S Version");
        GROUP_BY_COLUMN("OSVersionId", "O/S Version ID");
        GROUP_BY_COLUMN("OSDetection", "O/S Detection");
        GROUP_BY_COLUMN("CPUCores", "CPU Cores");
        GROUP_BY_COLUMN("ContainerOSName", "Container O/S Name");
        GROUP_BY_COLUMN("ContainerOSId", "Container O/S ID");
        GROUP_BY_COLUMN("ContainerOSIdLike", "Container O/S ID Like");
        GROUP_BY_COLUMN("ContainerOSVersion", "Container O/S Version");
        GROUP_BY_COLUMN("ContainerOSVersionId", "Container O/S Version ID");
        GROUP_BY_COLUMN("ContainerOSDetection", "Container O/S Detection");
        GROUP_BY_COLUMN("IsK8sNode", "Kubernetes Nodes");
        GROUP_BY_COLUMN("KernelName", "Kernel Name");
        GROUP_BY_COLUMN("KernelVersion", "Kernel Version");
        GROUP_BY_COLUMN("Architecture", "Architecture");
        GROUP_BY_COLUMN("Virtualization", "Virtualization Technology");
        GROUP_BY_COLUMN("VirtDetection", "Virtualization Detection");
        GROUP_BY_COLUMN("Container", "Container");
        GROUP_BY_COLUMN("ContainerDetection", "Container Detection");
        GROUP_BY_COLUMN("CloudProviderType", "Cloud Provider Type");
        GROUP_BY_COLUMN("CloudInstanceType", "Cloud Instance Type");
        GROUP_BY_COLUMN("CloudInstanceRegion", "Cloud Instance Region");

        GROUP_BY_COLUMN("InStatus", "Collection Status");
        GROUP_BY_COLUMN("OutStatus", "Streaming Status");
        GROUP_BY_COLUMN("MlStatus", "ML Status");
        GROUP_BY_COLUMN("InRemoteIP", "Inbound IP");
        GROUP_BY_COLUMN("OutRemoteIP", "Outbound IP");
    }
    buffer_json_object_close(wb); // group_by

    buffer_json_member_add_time_t(wb, "expires", now_realtime_sec() + STREAMING_FUNCTION_UPDATE_EVERY);
    buffer_json_finalize(wb);

    return HTTP_RESP_OK;
}
