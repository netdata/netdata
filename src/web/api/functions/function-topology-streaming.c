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

static void streaming_topology_v1_emit_response_metadata(BUFFER *wb) {
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
}

typedef struct streaming_topology_v1_actor {
    char actor_id[256];
    char type[32];
    char machine_guid[UUID_STR_LEN];
    char node_id[UUID_STR_LEN];
    char hostname[256];
    char display_name[256];
    char severity[32];
    char ephemerality[32];
    char ingest_status[64];
    char stream_status[64];
    char ml_status[64];
    char agent_name[128];
    char agent_version[128];
    char health_status[64];
    char os_name[128];
    char architecture[64];
    char cpu_count[32];
    uint64_t child_count;
    uint64_t health_critical;
    uint64_t health_warning;
    uint64_t health_clear;
    RRDHOST *host;
    bool synthetic;
} STREAMING_TOPOLOGY_V1_ACTOR;

typedef struct streaming_topology_v1_actor_label {
    uint64_t actor;
    char key[RRDLABELS_MAX_NAME_LENGTH + 1];
    char value[RRDLABELS_MAX_VALUE_LENGTH + 1];
    char source[32];
    char kind[32];
    bool has_value_index;
    uint64_t value_index;
} STREAMING_TOPOLOGY_V1_ACTOR_LABEL;

typedef struct streaming_topology_v1_link {
    uint64_t src_actor;
    uint64_t dst_actor;
    char type[32];
    char state[64];
    char port_name[256];
    uint64_t discovered_at_ut;
    uint64_t last_seen_ut;
    int64_t hops;
    uint64_t connections;
    uint64_t replication_instances;
    NETDATA_DOUBLE replication_completion;
    uint64_t collected_metrics;
    uint64_t collected_instances;
    uint64_t collected_contexts;
} STREAMING_TOPOLOGY_V1_LINK;

typedef struct streaming_topology_v1_stream_path_row {
    uint64_t actor;
    uint64_t path_actor;
    uint64_t path_index;
    char hostname[256];
    char host_id[UUID_STR_LEN];
    char node_id[UUID_STR_LEN];
    char claim_id[UUID_STR_LEN];
    int64_t hops;
    uint64_t since_ut;
    uint64_t first_time_ut;
    uint64_t start_time_ms;
    uint64_t shutdown_time_ms;
    uint64_t capabilities;
    uint64_t flags;
} STREAMING_TOPOLOGY_V1_STREAM_PATH_ROW;

typedef struct streaming_topology_v1_retention_row {
    uint64_t actor;
    uint64_t observer_actor;
    char db_status[64];
    uint64_t db_from_ut;
    uint64_t db_to_ut;
    uint64_t db_duration;
    uint64_t db_metrics;
    uint64_t db_instances;
    uint64_t db_contexts;
} STREAMING_TOPOLOGY_V1_RETENTION_ROW;

typedef struct streaming_topology_v1_inbound_row {
    uint64_t parent_actor;
    uint64_t child_actor;
    bool has_source_actor;
    uint64_t source_actor;
    char received_type[32];
    char ingest_status[64];
    int64_t hops;
    uint64_t collected_metrics;
    uint64_t collected_instances;
    uint64_t collected_contexts;
    NETDATA_DOUBLE replication_completion;
    uint64_t ingest_age;
    char ssl[16];
    uint64_t alerts_critical;
    uint64_t alerts_warning;
} STREAMING_TOPOLOGY_V1_INBOUND_ROW;

typedef struct streaming_topology_v1_outbound_row {
    uint64_t actor;
    bool has_destination_actor;
    uint64_t destination_actor;
    char stream_status[64];
    int64_t hops;
    char ssl[16];
    char compression[24];
} STREAMING_TOPOLOGY_V1_OUTBOUND_ROW;

typedef struct streaming_topology_v1_payload {
    STREAMING_TOPOLOGY_V1_ACTOR *actors;
    size_t actors_used;
    size_t actors_size;

    STREAMING_TOPOLOGY_V1_LINK *links;
    size_t links_used;
    size_t links_size;

    STREAMING_TOPOLOGY_V1_ACTOR_LABEL *labels;
    size_t labels_used;
    size_t labels_size;

    STREAMING_TOPOLOGY_V1_STREAM_PATH_ROW *stream_path_rows;
    size_t stream_path_used;
    size_t stream_path_size;

    STREAMING_TOPOLOGY_V1_RETENTION_ROW *retention_rows;
    size_t retention_used;
    size_t retention_size;

    STREAMING_TOPOLOGY_V1_INBOUND_ROW *inbound_rows;
    size_t inbound_used;
    size_t inbound_size;

    STREAMING_TOPOLOGY_V1_OUTBOUND_ROW *outbound_rows;
    size_t outbound_used;
    size_t outbound_size;

    DICTIONARY *actor_index;
    DICTIONARY *emitted_links;
} STREAMING_TOPOLOGY_V1_PAYLOAD;

static void streaming_topology_v1_strncpy(char *dst, size_t dst_size, const char *src) {
    if(!dst || !dst_size)
        return;

    strncpyz(dst, src ? src : "", dst_size - 1);
}

static void streaming_topology_v1_uuid_str(ND_UUID uuid, char *dst, size_t dst_size) {
    if(!dst || !dst_size)
        return;

    if(!streaming_topology_uuid_guid(uuid, dst, dst_size))
        dst[0] = '\0';
}

static void streaming_topology_v1_actor_index_set(STREAMING_TOPOLOGY_V1_PAYLOAD *payload, const char *actor_id, uint64_t index) {
    dictionary_set(payload->actor_index, actor_id, &index, sizeof(index));
}

static bool streaming_topology_v1_actor_index_get(STREAMING_TOPOLOGY_V1_PAYLOAD *payload, const char *actor_id, uint64_t *index) {
    uint64_t *stored = dictionary_get(payload->actor_index, actor_id);
    if(!stored)
        return false;

    if(index)
        *index = *stored;

    return true;
}

static STREAMING_TOPOLOGY_V1_ACTOR *streaming_topology_v1_add_actor(STREAMING_TOPOLOGY_V1_PAYLOAD *payload, const char *actor_id) {
    if(payload->actors_used == payload->actors_size) {
        size_t new_size = payload->actors_size ? payload->actors_size * 2 : 16;
        payload->actors = reallocz(payload->actors, new_size * sizeof(*payload->actors));
        payload->actors_size = new_size;
    }

    STREAMING_TOPOLOGY_V1_ACTOR *actor = &payload->actors[payload->actors_used];
    *actor = (STREAMING_TOPOLOGY_V1_ACTOR){ 0 };
    streaming_topology_v1_strncpy(actor->actor_id, sizeof(actor->actor_id), actor_id);
    streaming_topology_v1_actor_index_set(payload, actor_id, payload->actors_used);
    payload->actors_used++;
    return actor;
}

static STREAMING_TOPOLOGY_V1_LINK *streaming_topology_v1_add_link(STREAMING_TOPOLOGY_V1_PAYLOAD *payload) {
    if(payload->links_used == payload->links_size) {
        size_t new_size = payload->links_size ? payload->links_size * 2 : 16;
        payload->links = reallocz(payload->links, new_size * sizeof(*payload->links));
        payload->links_size = new_size;
    }

    STREAMING_TOPOLOGY_V1_LINK *link = &payload->links[payload->links_used++];
    *link = (STREAMING_TOPOLOGY_V1_LINK){ 0 };
    return link;
}

static const char *streaming_topology_v1_label_source(RRDLABEL_SRC source) {
    if(source & RRDLABEL_SRC_K8S)
        return "k8s";

    if(source & RRDLABEL_SRC_ACLK)
        return "aclk";

    if(source & RRDLABEL_SRC_CONFIG)
        return "config";

    if(source & RRDLABEL_SRC_AUTO)
        return "auto";

    return "unknown";
}

static void streaming_topology_v1_add_actor_label_ex(
    STREAMING_TOPOLOGY_V1_PAYLOAD *payload,
    uint64_t actor,
    const char *key,
    const char *value,
    const char *source,
    const char *kind,
    bool has_value_index,
    uint64_t value_index) {
    if(!payload || !key || !*key || !value || !*value)
        return;

    if(payload->labels_used == payload->labels_size) {
        size_t new_size = payload->labels_size ? payload->labels_size * 2 : 64;
        payload->labels = reallocz(payload->labels, new_size * sizeof(*payload->labels));
        payload->labels_size = new_size;
    }

    STREAMING_TOPOLOGY_V1_ACTOR_LABEL *row = &payload->labels[payload->labels_used++];
    *row = (STREAMING_TOPOLOGY_V1_ACTOR_LABEL){ 0 };
    row->actor = actor;
    streaming_topology_v1_strncpy(row->key, sizeof(row->key), key);
    streaming_topology_v1_strncpy(row->value, sizeof(row->value), value);
    streaming_topology_v1_strncpy(row->source, sizeof(row->source), source ? source : "producer");
    streaming_topology_v1_strncpy(row->kind, sizeof(row->kind), kind ? kind : "metadata");
    row->has_value_index = has_value_index;
    row->value_index = value_index;
}

static void streaming_topology_v1_add_actor_label(
    STREAMING_TOPOLOGY_V1_PAYLOAD *payload,
    uint64_t actor,
    const char *key,
    const char *value,
    const char *kind) {
    streaming_topology_v1_add_actor_label_ex(payload, actor, key, value, "producer", kind, false, 0);
}

static void streaming_topology_v1_add_actor_label_uint(
    STREAMING_TOPOLOGY_V1_PAYLOAD *payload,
    uint64_t actor,
    const char *key,
    uint64_t value,
    const char *kind) {
    char text[32];
    snprintfz(text, sizeof(text), "%"PRIu64, value);
    streaming_topology_v1_add_actor_label(payload, actor, key, text, kind);
}

struct streaming_topology_v1_host_label_ctx {
    STREAMING_TOPOLOGY_V1_PAYLOAD *payload;
    uint64_t actor;
};

static int streaming_topology_v1_collect_host_label(
    const char *name,
    const char *value,
    RRDLABEL_SRC source,
    void *data) {
    struct streaming_topology_v1_host_label_ctx *ctx = data;
    streaming_topology_v1_add_actor_label_ex(
        ctx->payload, ctx->actor, name, value,
        streaming_topology_v1_label_source(source), "host_label", false, 0);
    return 0;
}

static void streaming_topology_v1_collect_actor_labels(
    STREAMING_TOPOLOGY_V1_PAYLOAD *payload,
    uint64_t actor_index,
    STREAMING_TOPOLOGY_V1_ACTOR *actor) {
    if(!payload || !actor)
        return;

    streaming_topology_v1_add_actor_label(payload, actor_index, "display_name", actor->display_name, "identity");
    streaming_topology_v1_add_actor_label(payload, actor_index, "hostname", actor->hostname, "identity");
    streaming_topology_v1_add_actor_label(payload, actor_index, "machine_guid", actor->machine_guid, "identity");
    streaming_topology_v1_add_actor_label(payload, actor_index, "node_id", actor->node_id, "identity");
    streaming_topology_v1_add_actor_label(payload, actor_index, "type", actor->type, "metadata");
    streaming_topology_v1_add_actor_label(payload, actor_index, "severity", actor->severity, "status");
    streaming_topology_v1_add_actor_label(payload, actor_index, "ephemerality", actor->ephemerality, "metadata");
    streaming_topology_v1_add_actor_label(payload, actor_index, "ingest_status", actor->ingest_status, "status");
    streaming_topology_v1_add_actor_label(payload, actor_index, "stream_status", actor->stream_status, "status");
    streaming_topology_v1_add_actor_label(payload, actor_index, "ml_status", actor->ml_status, "status");
    streaming_topology_v1_add_actor_label(payload, actor_index, "agent_name", actor->agent_name, "metadata");
    streaming_topology_v1_add_actor_label(payload, actor_index, "agent_version", actor->agent_version, "metadata");
    streaming_topology_v1_add_actor_label(payload, actor_index, "health_status", actor->health_status, "status");
    streaming_topology_v1_add_actor_label(payload, actor_index, "os_name", actor->os_name, "system");
    streaming_topology_v1_add_actor_label(payload, actor_index, "architecture", actor->architecture, "system");
    streaming_topology_v1_add_actor_label(payload, actor_index, "cpu_count", actor->cpu_count, "system");
    streaming_topology_v1_add_actor_label_uint(payload, actor_index, "child_count", actor->child_count, "metric");
    streaming_topology_v1_add_actor_label_uint(payload, actor_index, "health_critical", actor->health_critical, "metric");
    streaming_topology_v1_add_actor_label_uint(payload, actor_index, "health_warning", actor->health_warning, "metric");
    streaming_topology_v1_add_actor_label_uint(payload, actor_index, "health_clear", actor->health_clear, "metric");

    if(actor->host && actor->host->rrdlabels) {
        struct streaming_topology_v1_host_label_ctx ctx = {
            .payload = payload,
            .actor = actor_index,
        };
        rrdlabels_walkthrough_read(actor->host->rrdlabels, streaming_topology_v1_collect_host_label, &ctx);
    }
}

static STREAMING_TOPOLOGY_V1_STREAM_PATH_ROW *streaming_topology_v1_add_stream_path_row(
    STREAMING_TOPOLOGY_V1_PAYLOAD *payload) {
    if(payload->stream_path_used == payload->stream_path_size) {
        size_t new_size = payload->stream_path_size ? payload->stream_path_size * 2 : 32;
        payload->stream_path_rows = reallocz(payload->stream_path_rows, new_size * sizeof(*payload->stream_path_rows));
        payload->stream_path_size = new_size;
    }

    STREAMING_TOPOLOGY_V1_STREAM_PATH_ROW *row = &payload->stream_path_rows[payload->stream_path_used++];
    *row = (STREAMING_TOPOLOGY_V1_STREAM_PATH_ROW){ 0 };
    return row;
}

static STREAMING_TOPOLOGY_V1_RETENTION_ROW *streaming_topology_v1_add_retention_row(
    STREAMING_TOPOLOGY_V1_PAYLOAD *payload) {
    if(payload->retention_used == payload->retention_size) {
        size_t new_size = payload->retention_size ? payload->retention_size * 2 : 16;
        payload->retention_rows = reallocz(payload->retention_rows, new_size * sizeof(*payload->retention_rows));
        payload->retention_size = new_size;
    }

    STREAMING_TOPOLOGY_V1_RETENTION_ROW *row = &payload->retention_rows[payload->retention_used++];
    *row = (STREAMING_TOPOLOGY_V1_RETENTION_ROW){ 0 };
    return row;
}

static STREAMING_TOPOLOGY_V1_INBOUND_ROW *streaming_topology_v1_add_inbound_row(
    STREAMING_TOPOLOGY_V1_PAYLOAD *payload) {
    if(payload->inbound_used == payload->inbound_size) {
        size_t new_size = payload->inbound_size ? payload->inbound_size * 2 : 32;
        payload->inbound_rows = reallocz(payload->inbound_rows, new_size * sizeof(*payload->inbound_rows));
        payload->inbound_size = new_size;
    }

    STREAMING_TOPOLOGY_V1_INBOUND_ROW *row = &payload->inbound_rows[payload->inbound_used++];
    *row = (STREAMING_TOPOLOGY_V1_INBOUND_ROW){ 0 };
    return row;
}

static STREAMING_TOPOLOGY_V1_OUTBOUND_ROW *streaming_topology_v1_add_outbound_row(
    STREAMING_TOPOLOGY_V1_PAYLOAD *payload) {
    if(payload->outbound_used == payload->outbound_size) {
        size_t new_size = payload->outbound_size ? payload->outbound_size * 2 : 32;
        payload->outbound_rows = reallocz(payload->outbound_rows, new_size * sizeof(*payload->outbound_rows));
        payload->outbound_size = new_size;
    }

    STREAMING_TOPOLOGY_V1_OUTBOUND_ROW *row = &payload->outbound_rows[payload->outbound_used++];
    *row = (STREAMING_TOPOLOGY_V1_OUTBOUND_ROW){ 0 };
    return row;
}

static bool streaming_topology_v1_link_seen(STREAMING_TOPOLOGY_V1_PAYLOAD *payload, uint64_t src, uint64_t dst, const char *type) {
    char link_key[128];
    snprintfz(link_key, sizeof(link_key), "%"PRIu64"|%"PRIu64"|%s", src, dst, type ? type : "");
    if(dictionary_get(payload->emitted_links, link_key))
        return true;

    uint8_t one = 1;
    dictionary_set(payload->emitted_links, link_key, &one, sizeof(one));
    return false;
}

static void streaming_topology_v1_free(STREAMING_TOPOLOGY_V1_PAYLOAD *payload) {
    if(!payload)
        return;

    freez(payload->actors);
    freez(payload->links);
    freez(payload->labels);
    freez(payload->stream_path_rows);
    freez(payload->retention_rows);
    freez(payload->inbound_rows);
    freez(payload->outbound_rows);

    if(payload->actor_index)
        dictionary_destroy(payload->actor_index);
    if(payload->emitted_links)
        dictionary_destroy(payload->emitted_links);

    *payload = (STREAMING_TOPOLOGY_V1_PAYLOAD){ 0 };
}

static void streaming_topology_v1_emit_column(
    BUFFER *wb,
    const char *id,
    const char *type,
    const char *role,
    bool nullable,
    const char *aggregation) {
    buffer_json_add_array_item_object(wb);
    buffer_json_member_add_string(wb, "id", id);
    buffer_json_member_add_string(wb, "type", type);
    if(nullable)
        buffer_json_member_add_boolean(wb, "nullable", true);
    if(role)
        buffer_json_member_add_string(wb, "role", role);
    if(aggregation)
        buffer_json_member_add_string(wb, "aggregation", aggregation);
    buffer_json_object_close(wb);
}

static void streaming_topology_v1_emit_values_start(BUFFER *wb) {
    buffer_json_add_array_item_object(wb);
    buffer_json_member_add_string(wb, "codec", "values");
    buffer_json_member_add_array(wb, "values");
}

static void streaming_topology_v1_emit_values_end(BUFFER *wb) {
    buffer_json_array_close(wb);
    buffer_json_object_close(wb);
}

static void streaming_topology_v1_add_nullable_uint(BUFFER *wb, bool has_value, uint64_t value) {
    if(has_value)
        buffer_json_add_array_item_uint64(wb, value);
    else
        buffer_json_add_array_item_string(wb, NULL);
}

static void streaming_topology_v1_add_timestamp(BUFFER *wb, uint64_t timestamp_ut) {
    if(timestamp_ut)
        buffer_json_add_array_item_datetime_rfc3339(wb, timestamp_ut, true);
    else
        buffer_json_add_array_item_string(wb, NULL);
}

static const char *streaming_topology_v1_node_type(
    RRDHOST *host,
    RRDHOST_STATUS *status,
    DICTIONARY *parent_child_count) {
    if(rrdhost_is_virtual(host))
        return "vnode";

    if(host != localhost && status->ingest.status == RRDHOST_INGEST_STATUS_ARCHIVED)
        return "stale";

    uint32_t *cc = streaming_topology_parent_child_count_get(parent_child_count, host);
    return (cc && *cc > 0) ? "parent" : "child";
}

static const char *streaming_topology_v1_severity(RRDHOST *host, RRDHOST_STATUS *status) {
    if(rrdhost_option_check(host, RRDHOST_OPTION_EPHEMERAL_HOST))
        return "normal";

    switch(status->ingest.status) {
        case RRDHOST_INGEST_STATUS_OFFLINE:
        case RRDHOST_INGEST_STATUS_ARCHIVED:
            return "critical";
        default:
            break;
    }

    if(status->stream.status == RRDHOST_STREAM_STATUS_OFFLINE &&
       status->stream.reason != STREAM_HANDSHAKE_SP_NO_DESTINATION)
        return "warning";

    return "normal";
}

struct streaming_topology_v1_synth_actor_ctx {
    STREAMING_TOPOLOGY_V1_PAYLOAD *payload;
};

static bool streaming_topology_v1_collect_synth_actor(
    void *userdata,
    uint16_t index __maybe_unused,
    STRING *hostname,
    ND_UUID host_id,
    ND_UUID node_id,
    ND_UUID claim_id __maybe_unused,
    int16_t hops __maybe_unused,
    time_t since __maybe_unused,
    time_t first_time_t __maybe_unused,
    uint32_t start_time_ms __maybe_unused,
    uint32_t shutdown_time_ms __maybe_unused,
    STREAM_CAPABILITIES capabilities __maybe_unused,
    uint32_t flags __maybe_unused) {
    struct streaming_topology_v1_synth_actor_ctx *ctx = userdata;

    if(UUIDeq(host_id, localhost->host_id))
        return true;

    char guid[UUID_STR_LEN];
    if(!streaming_topology_uuid_guid(host_id, guid, sizeof(guid)))
        return true;

    char actor_id[256];
    streaming_topology_actor_id_from_guid(guid, actor_id, sizeof(actor_id));
    if(streaming_topology_v1_actor_index_get(ctx->payload, actor_id, NULL))
        return true;

    STREAMING_TOPOLOGY_V1_ACTOR *actor = streaming_topology_v1_add_actor(ctx->payload, actor_id);
    actor->synthetic = true;
    streaming_topology_v1_strncpy(actor->type, sizeof(actor->type), "parent");
    streaming_topology_v1_strncpy(actor->machine_guid, sizeof(actor->machine_guid), guid);
    streaming_topology_v1_uuid_str(node_id, actor->node_id, sizeof(actor->node_id));
    streaming_topology_v1_strncpy(actor->hostname, sizeof(actor->hostname), string2str(hostname));
    streaming_topology_v1_strncpy(actor->display_name, sizeof(actor->display_name), string2str(hostname));
    streaming_topology_v1_strncpy(actor->severity, sizeof(actor->severity), "normal");
    streaming_topology_v1_strncpy(actor->ephemerality, sizeof(actor->ephemerality), "permanent");
    streaming_topology_v1_strncpy(actor->ingest_status, sizeof(actor->ingest_status), "unknown");
    streaming_topology_v1_strncpy(actor->stream_status, sizeof(actor->stream_status), "unknown");
    streaming_topology_v1_strncpy(actor->ml_status, sizeof(actor->ml_status), "unknown");
    streaming_topology_v1_strncpy(actor->health_status, sizeof(actor->health_status), "unknown");
    streaming_topology_v1_collect_actor_labels(ctx->payload, ctx->payload->actors_used - 1, actor);
    return true;
}

static void streaming_topology_v1_collect_actors(
    STREAMING_TOPOLOGY_V1_PAYLOAD *payload,
    DICTIONARY *parent_child_count,
    time_t now) {
    RRDHOST *host;
    dfe_start_read(rrdhost_root_index, host) {
        RRDHOST_STATUS status;
        rrdhost_status(host, now, &status, RRDHOST_STATUS_ALL);

        char actor_id[256];
        streaming_topology_actor_id_for_host(host, actor_id, sizeof(actor_id));

        STREAMING_TOPOLOGY_V1_ACTOR *actor = streaming_topology_v1_add_actor(payload, actor_id);
        actor->host = host;
        streaming_topology_v1_strncpy(actor->type, sizeof(actor->type),
            streaming_topology_v1_node_type(host, &status, parent_child_count));
        streaming_topology_host_guid(host, actor->machine_guid, sizeof(actor->machine_guid));
        streaming_topology_v1_uuid_str(host->node_id, actor->node_id, sizeof(actor->node_id));
        streaming_topology_v1_strncpy(actor->hostname, sizeof(actor->hostname), rrdhost_hostname(host));
        streaming_topology_v1_strncpy(actor->display_name, sizeof(actor->display_name), rrdhost_hostname(host));
        streaming_topology_v1_strncpy(actor->severity, sizeof(actor->severity),
            streaming_topology_v1_severity(host, &status));
        streaming_topology_v1_strncpy(actor->ephemerality, sizeof(actor->ephemerality),
            rrdhost_option_check(host, RRDHOST_OPTION_EPHEMERAL_HOST) ? "ephemeral" : "permanent");
        streaming_topology_v1_strncpy(actor->ingest_status, sizeof(actor->ingest_status),
            rrdhost_ingest_status_to_string(status.ingest.status));
        streaming_topology_v1_strncpy(actor->stream_status, sizeof(actor->stream_status),
            rrdhost_streaming_status_to_string(status.stream.status));
        streaming_topology_v1_strncpy(actor->ml_status, sizeof(actor->ml_status),
            rrdhost_ml_status_to_string(status.ml.status));
        streaming_topology_v1_strncpy(actor->agent_name, sizeof(actor->agent_name), rrdhost_program_name(host));
        streaming_topology_v1_strncpy(actor->agent_version, sizeof(actor->agent_version), rrdhost_program_version(host));
        streaming_topology_v1_strncpy(actor->health_status, sizeof(actor->health_status),
            rrdhost_health_status_to_string(status.health.status));
        rrdlabels_get_value_strcpyz(host->rrdlabels, actor->os_name, sizeof(actor->os_name), "_os_name");
        rrdlabels_get_value_strcpyz(host->rrdlabels, actor->architecture, sizeof(actor->architecture), "_architecture");
        rrdlabels_get_value_strcpyz(host->rrdlabels, actor->cpu_count, sizeof(actor->cpu_count), "_system_cores");

        uint32_t *cc = streaming_topology_parent_child_count_get(parent_child_count, host);
        actor->child_count = cc ? *cc : 0;
        if(status.health.status == RRDHOST_HEALTH_STATUS_RUNNING) {
            actor->health_critical = status.health.alerts.critical;
            actor->health_warning = status.health.alerts.warning;
            actor->health_clear = status.health.alerts.clear;
        }

        streaming_topology_v1_collect_actor_labels(payload, payload->actors_used - 1, actor);
    }
    dfe_done(host);

    struct streaming_topology_v1_synth_actor_ctx ctx = { .payload = payload };
    dfe_start_read(rrdhost_root_index, host) {
        rrdhost_stream_path_visit(host, 1, streaming_topology_v1_collect_synth_actor, &ctx);
    }
    dfe_done(host);
}

static bool streaming_topology_v1_actor_index_for_host(
    STREAMING_TOPOLOGY_V1_PAYLOAD *payload,
    RRDHOST *host,
    uint64_t *index) {
    char actor_id[256];
    streaming_topology_actor_id_for_host(host, actor_id, sizeof(actor_id));
    return streaming_topology_v1_actor_index_get(payload, actor_id, index);
}

static void streaming_topology_v1_add_link_if_new(
    STREAMING_TOPOLOGY_V1_PAYLOAD *payload,
    uint64_t src_actor,
    uint64_t dst_actor,
    const char *type,
    const char *state,
    const char *port_name,
    uint64_t discovered_at_ut,
    uint64_t last_seen_ut,
    int64_t hops,
    uint64_t connections,
    uint64_t replication_instances,
    NETDATA_DOUBLE replication_completion,
    uint64_t collected_metrics,
    uint64_t collected_instances,
    uint64_t collected_contexts) {
    if(streaming_topology_v1_link_seen(payload, src_actor, dst_actor, type))
        return;

    STREAMING_TOPOLOGY_V1_LINK *link = streaming_topology_v1_add_link(payload);
    link->src_actor = src_actor;
    link->dst_actor = dst_actor;
    streaming_topology_v1_strncpy(link->type, sizeof(link->type), type);
    streaming_topology_v1_strncpy(link->state, sizeof(link->state), state);
    streaming_topology_v1_strncpy(link->port_name, sizeof(link->port_name), port_name);
    link->discovered_at_ut = discovered_at_ut;
    link->last_seen_ut = last_seen_ut;
    link->hops = hops;
    link->connections = connections;
    link->replication_instances = replication_instances;
    link->replication_completion = replication_completion;
    link->collected_metrics = collected_metrics;
    link->collected_instances = collected_instances;
    link->collected_contexts = collected_contexts;
}

struct streaming_topology_v1_synth_link_ctx {
    STREAMING_TOPOLOGY_V1_PAYLOAD *payload;
    bool has_prev;
    uint64_t prev_actor;
    char prev_actor_id[256];
    char prev_hostname[256];
    time_t prev_since;
    time_t prev_first_time_t;
};

static bool streaming_topology_v1_collect_synth_link(
    void *userdata,
    uint16_t index __maybe_unused,
    STRING *hostname,
    ND_UUID host_id,
    ND_UUID node_id __maybe_unused,
    ND_UUID claim_id __maybe_unused,
    int16_t hops __maybe_unused,
    time_t since,
    time_t first_time_t,
    uint32_t start_time_ms __maybe_unused,
    uint32_t shutdown_time_ms __maybe_unused,
    STREAM_CAPABILITIES capabilities __maybe_unused,
    uint32_t flags __maybe_unused) {
    struct streaming_topology_v1_synth_link_ctx *ctx = userdata;

    char guid[UUID_STR_LEN];
    if(!streaming_topology_uuid_guid(host_id, guid, sizeof(guid))) {
        ctx->has_prev = false;
        return true;
    }

    char cur_actor_id[256];
    streaming_topology_actor_id_from_guid(guid, cur_actor_id, sizeof(cur_actor_id));

    uint64_t cur_actor;
    if(!streaming_topology_v1_actor_index_get(ctx->payload, cur_actor_id, &cur_actor)) {
        ctx->has_prev = false;
        return true;
    }

    if(ctx->has_prev) {
        streaming_topology_v1_add_link_if_new(
            ctx->payload,
            ctx->prev_actor,
            cur_actor,
            "streaming",
            "online",
            ctx->prev_hostname,
            ((uint64_t)(ctx->prev_first_time_t ? ctx->prev_first_time_t : ctx->prev_since)) * USEC_PER_SEC,
            ((uint64_t)(since ? since : ctx->prev_since)) * USEC_PER_SEC,
            0, 0, 0, 0, 0, 0, 0);
    }

    ctx->has_prev = true;
    ctx->prev_actor = cur_actor;
    streaming_topology_v1_strncpy(ctx->prev_actor_id, sizeof(ctx->prev_actor_id), cur_actor_id);
    streaming_topology_v1_strncpy(ctx->prev_hostname, sizeof(ctx->prev_hostname), string2str(hostname));
    ctx->prev_since = since;
    ctx->prev_first_time_t = first_time_t;
    return true;
}

static void streaming_topology_v1_collect_links(
    STREAMING_TOPOLOGY_V1_PAYLOAD *payload,
    time_t now,
    usec_t now_ut) {
    char localhost_actor_id[256];
    streaming_topology_actor_id_for_host(localhost, localhost_actor_id, sizeof(localhost_actor_id));
    uint64_t localhost_actor = 0;
    streaming_topology_v1_actor_index_get(payload, localhost_actor_id, &localhost_actor);

    RRDHOST *host;
    dfe_start_read(rrdhost_root_index, host) {
        if(host == localhost)
            continue;

        RRDHOST_STATUS status;
        rrdhost_status(host, now, &status, RRDHOST_STATUS_ALL);

        uint64_t src_actor;
        if(!streaming_topology_v1_actor_index_for_host(payload, host, &src_actor))
            continue;

        char target_actor_id[256] = "";
        const char *link_type = NULL;
        bool is_vnode = rrdhost_is_virtual(host);
        ND_UUID link_ids[2];
        uint16_t link_n = streaming_topology_get_path_ids(host, 0, link_ids, 2);

        if(is_vnode) {
            snprintfz(target_actor_id, sizeof(target_actor_id), "%s", localhost_actor_id);
            link_type = "virtual";
        }
        else if(link_n >= 2) {
            streaming_topology_actor_id_for_uuid(link_ids[1], target_actor_id, sizeof(target_actor_id));
            link_type = "streaming";
        }
        else {
            snprintfz(target_actor_id, sizeof(target_actor_id), "%s", localhost_actor_id);
            link_type = "stale";
        }

        uint64_t dst_actor;
        if(!streaming_topology_v1_actor_index_get(payload, target_actor_id, &dst_actor))
            continue;

        streaming_topology_v1_add_link_if_new(
            payload,
            src_actor,
            dst_actor,
            link_type,
            rrdhost_ingest_status_to_string(status.ingest.status),
            rrdhost_hostname(host),
            ((uint64_t)(status.ingest.since ? status.ingest.since : now)) * USEC_PER_SEC,
            now_ut,
            status.ingest.hops,
            strcmp(link_type, "virtual") != 0 ? status.host->stream.rcv.status.connections : 0,
            strcmp(link_type, "virtual") != 0 ? status.ingest.replication.instances : 0,
            strcmp(link_type, "virtual") != 0 ? status.ingest.replication.completion : 0,
            strcmp(link_type, "virtual") != 0 ? status.ingest.collected.metrics : 0,
            strcmp(link_type, "virtual") != 0 ? status.ingest.collected.instances : 0,
            strcmp(link_type, "virtual") != 0 ? status.ingest.collected.contexts : 0);
    }
    dfe_done(host);

    dfe_start_read(rrdhost_root_index, host) {
        struct streaming_topology_v1_synth_link_ctx ctx = {
            .payload = payload,
            .has_prev = false,
        };
        rrdhost_stream_path_visit(host, 0, streaming_topology_v1_collect_synth_link, &ctx);
    }
    dfe_done(host);
}

struct streaming_topology_v1_stream_path_ctx {
    STREAMING_TOPOLOGY_V1_PAYLOAD *payload;
    uint64_t actor;
    bool seen_localhost;
    bool emitted;
    uint16_t next_index;
};

static bool streaming_topology_v1_collect_stream_path_row(
    void *userdata,
    uint16_t index,
    STRING *hostname,
    ND_UUID host_id,
    ND_UUID node_id,
    ND_UUID claim_id,
    int16_t hops,
    time_t since,
    time_t first_time_t,
    uint32_t start_time_ms,
    uint32_t shutdown_time_ms,
    STREAM_CAPABILITIES capabilities,
    uint32_t flags) {
    struct streaming_topology_v1_stream_path_ctx *ctx = userdata;

    STREAMING_TOPOLOGY_V1_STREAM_PATH_ROW *row = streaming_topology_v1_add_stream_path_row(ctx->payload);
    row->actor = ctx->actor;
    row->path_actor = ctx->actor;
    row->path_index = index;
    char path_actor_guid[UUID_STR_LEN];
    if(streaming_topology_uuid_guid(host_id, path_actor_guid, sizeof(path_actor_guid))) {
        char path_actor_id[256];
        streaming_topology_actor_id_from_guid(path_actor_guid, path_actor_id, sizeof(path_actor_id));
        streaming_topology_v1_actor_index_get(ctx->payload, path_actor_id, &row->path_actor);
    }
    streaming_topology_v1_strncpy(row->hostname, sizeof(row->hostname), string2str(hostname));
    streaming_topology_v1_uuid_str(host_id, row->host_id, sizeof(row->host_id));
    streaming_topology_v1_uuid_str(node_id, row->node_id, sizeof(row->node_id));
    streaming_topology_v1_uuid_str(claim_id, row->claim_id, sizeof(row->claim_id));
    row->hops = hops;
    row->since_ut = since > 0 ? (uint64_t)since * USEC_PER_SEC : 0;
    row->first_time_ut = first_time_t > 0 ? (uint64_t)first_time_t * USEC_PER_SEC : 0;
    row->start_time_ms = start_time_ms;
    row->shutdown_time_ms = shutdown_time_ms;
    row->capabilities = capabilities;
    row->flags = flags;
    if(UUIDeq(host_id, localhost->host_id))
        ctx->seen_localhost = true;
    ctx->emitted = true;
    ctx->next_index = index + 1;
    return true;
}

static void streaming_topology_v1_collect_actor_detail_rows(
    STREAMING_TOPOLOGY_V1_PAYLOAD *payload,
    DICTIONARY *parent_descendants,
    time_t now) {
    uint64_t localhost_actor = 0;
    streaming_topology_v1_actor_index_for_host(payload, localhost, &localhost_actor);

    for(size_t i = 0; i < payload->actors_used; i++) {
        STREAMING_TOPOLOGY_V1_ACTOR *actor = &payload->actors[i];
        if(!actor->host)
            continue;

        RRDHOST_STATUS status;
        rrdhost_status(actor->host, now, &status, RRDHOST_STATUS_ALL);

        struct streaming_topology_v1_stream_path_ctx sp_ctx = {
            .payload = payload,
            .actor = i,
            .emitted = false,
        };
        rrdhost_stream_path_visit(actor->host, 0, streaming_topology_v1_collect_stream_path_row, &sp_ctx);
        if(sp_ctx.emitted && !sp_ctx.seen_localhost) {
            // Match the legacy highlight path helper: stored paths do not
            // always carry the local agent, but rendered paths need it.
            STREAMING_TOPOLOGY_V1_STREAM_PATH_ROW *row = streaming_topology_v1_add_stream_path_row(payload);
            row->actor = i;
            row->path_actor = localhost_actor;
            row->path_index = sp_ctx.next_index;
            streaming_topology_v1_strncpy(row->hostname, sizeof(row->hostname), rrdhost_hostname(localhost));
            streaming_topology_host_guid(localhost, row->host_id, sizeof(row->host_id));
            streaming_topology_v1_uuid_str(localhost->node_id, row->node_id, sizeof(row->node_id));
        }
        else if(!sp_ctx.emitted) {
            STREAMING_TOPOLOGY_V1_STREAM_PATH_ROW *row = streaming_topology_v1_add_stream_path_row(payload);
            row->actor = i;
            row->path_actor = i;
            row->path_index = 0;
            streaming_topology_v1_strncpy(row->hostname, sizeof(row->hostname), actor->hostname);
            streaming_topology_v1_strncpy(row->host_id, sizeof(row->host_id), actor->machine_guid);
            streaming_topology_v1_strncpy(row->node_id, sizeof(row->node_id), actor->node_id);
            row->hops = status.ingest.hops;
            row->since_ut = status.ingest.since ? (uint64_t)status.ingest.since * USEC_PER_SEC : 0;
            row->first_time_ut = status.db.first_time_s ? (uint64_t)status.db.first_time_s * USEC_PER_SEC : 0;
        }

        STREAMING_TOPOLOGY_V1_RETENTION_ROW *retention = streaming_topology_v1_add_retention_row(payload);
        retention->actor = i;
        retention->observer_actor = localhost_actor;
        streaming_topology_v1_strncpy(retention->db_status, sizeof(retention->db_status),
            rrdhost_db_status_to_string(status.db.status));
        retention->db_from_ut = status.db.first_time_s ? (uint64_t)status.db.first_time_s * USEC_PER_SEC : 0;
        retention->db_to_ut = status.db.last_time_s ? (uint64_t)status.db.last_time_s * USEC_PER_SEC : 0;
        retention->db_duration =
            status.db.first_time_s && status.db.last_time_s && status.db.last_time_s > status.db.first_time_s ?
                (uint64_t)(status.db.last_time_s - status.db.first_time_s) : 0;
        retention->db_metrics = status.db.metrics;
        retention->db_instances = status.db.instances;
        retention->db_contexts = status.db.contexts;
    }

    for(size_t i = 0; i < payload->actors_used; i++) {
        STREAMING_TOPOLOGY_V1_ACTOR *parent = &payload->actors[i];
        if(!parent->host || parent->child_count == 0)
            continue;

        struct streaming_topology_descendant_list *nodes =
            streaming_topology_descendants_get(parent_descendants, parent->host);
        if(!nodes)
            continue;

        for(size_t j = 0; j < nodes->used; j++) {
            struct streaming_topology_descendant *descendant = &nodes->items[j];
            if(!descendant->host || descendant->host == parent->host)
                continue;

            uint64_t child_actor;
            if(!streaming_topology_v1_actor_index_for_host(payload, descendant->host, &child_actor))
                continue;

            RRDHOST_STATUS status;
            rrdhost_status(descendant->host, now, &status, RRDHOST_STATUS_ALL);

            STREAMING_TOPOLOGY_V1_INBOUND_ROW *row = streaming_topology_v1_add_inbound_row(payload);
            row->parent_actor = i;
            row->child_actor = child_actor;
            streaming_topology_v1_strncpy(row->received_type, sizeof(row->received_type),
                streaming_topology_received_type_to_string((enum streaming_topology_received_type)descendant->type));
            streaming_topology_v1_strncpy(row->ingest_status, sizeof(row->ingest_status),
                rrdhost_ingest_status_to_string(status.ingest.status));
            row->hops = status.ingest.hops;
            row->collected_metrics = status.ingest.collected.metrics;
            row->collected_instances = status.ingest.collected.instances;
            row->collected_contexts = status.ingest.collected.contexts;
            row->replication_completion = status.ingest.replication.completion;
            row->ingest_age = status.ingest.since ? (uint64_t)(status.now - status.ingest.since) : 0;
            streaming_topology_v1_strncpy(row->ssl, sizeof(row->ssl), status.ingest.ssl ? "SSL" : "PLAIN");
            row->alerts_critical =
                status.health.status == RRDHOST_HEALTH_STATUS_RUNNING ? status.health.alerts.critical : 0;
            row->alerts_warning =
                status.health.status == RRDHOST_HEALTH_STATUS_RUNNING ? status.health.alerts.warning : 0;

            if(!descendant->source_local && !UUIDiszero(descendant->source_uuid)) {
                char source_actor_id[256];
                streaming_topology_actor_id_for_uuid(descendant->source_uuid, source_actor_id, sizeof(source_actor_id));
                row->has_source_actor =
                    streaming_topology_v1_actor_index_get(payload, source_actor_id, &row->source_actor);
            }
        }
    }

    for(size_t i = 0; i < payload->actors_used; i++) {
        STREAMING_TOPOLOGY_V1_ACTOR *actor = &payload->actors[i];
        if(!actor->host)
            continue;

        RRDHOST_STATUS status;
        rrdhost_status(actor->host, now, &status, RRDHOST_STATUS_ALL);

        STREAMING_TOPOLOGY_V1_OUTBOUND_ROW *row = streaming_topology_v1_add_outbound_row(payload);
        row->actor = i;
        streaming_topology_v1_strncpy(row->stream_status, sizeof(row->stream_status),
            rrdhost_streaming_status_to_string(status.stream.status));
        row->hops = status.stream.hops;
        streaming_topology_v1_strncpy(row->ssl, sizeof(row->ssl), status.stream.ssl ? "SSL" : "PLAIN");
        streaming_topology_v1_strncpy(row->compression, sizeof(row->compression),
            status.stream.compression ? "COMPRESSED" : "UNCOMPRESSED");

        ND_UUID path[128];
        uint16_t path_n = streaming_topology_get_path_ids(actor->host, 0, path, 128);
        for(uint16_t pi = 0; pi < path_n; pi++) {
            if(UUIDeq(path[pi], actor->host->host_id) && pi + 1 < path_n) {
                char destination_actor_id[256];
                streaming_topology_actor_id_for_uuid(path[pi + 1], destination_actor_id, sizeof(destination_actor_id));
                row->has_destination_actor =
                    streaming_topology_v1_actor_index_get(payload, destination_actor_id, &row->destination_actor);
                break;
            }
        }
    }
}

static void streaming_topology_v1_emit_actor_columns(BUFFER *wb) {
    streaming_topology_v1_emit_column(wb, "id", "string", "identity", false, NULL);
    streaming_topology_v1_emit_column(wb, "type", "string", "group_key", false, NULL);
    streaming_topology_v1_emit_column(wb, "layer", "string", "group_key", false, NULL);
    streaming_topology_v1_emit_column(wb, "machine_guid", "string", "merge_identity", true, NULL);
    streaming_topology_v1_emit_column(wb, "node_id", "string", "merge_identity", true, NULL);
    streaming_topology_v1_emit_column(wb, "hostname", "string", "attribute", true, NULL);
    streaming_topology_v1_emit_column(wb, "display_name", "string", "attribute", true, NULL);
    streaming_topology_v1_emit_column(wb, "severity", "string", "attribute", true, NULL);
    streaming_topology_v1_emit_column(wb, "ephemerality", "string", "attribute", true, NULL);
    streaming_topology_v1_emit_column(wb, "ingest_status", "string", "attribute", true, NULL);
    streaming_topology_v1_emit_column(wb, "stream_status", "string", "attribute", true, NULL);
    streaming_topology_v1_emit_column(wb, "ml_status", "string", "attribute", true, NULL);
    streaming_topology_v1_emit_column(wb, "agent_name", "string", "attribute", true, NULL);
    streaming_topology_v1_emit_column(wb, "agent_version", "string", "attribute", true, NULL);
    streaming_topology_v1_emit_column(wb, "health_status", "string", "attribute", true, NULL);
    streaming_topology_v1_emit_column(wb, "os_name", "string", "attribute", true, NULL);
    streaming_topology_v1_emit_column(wb, "architecture", "string", "attribute", true, NULL);
    streaming_topology_v1_emit_column(wb, "cpu_count", "string", "attribute", true, NULL);
    streaming_topology_v1_emit_column(wb, "child_count", "uint", "metric", false, "sum");
    streaming_topology_v1_emit_column(wb, "health_critical", "uint", "metric", false, "sum");
    streaming_topology_v1_emit_column(wb, "health_warning", "uint", "metric", false, "sum");
    streaming_topology_v1_emit_column(wb, "health_clear", "uint", "metric", false, "sum");
}

static void streaming_topology_v1_emit_link_columns(BUFFER *wb) {
    streaming_topology_v1_emit_column(wb, "src_actor", "actor_ref", "reference", false, NULL);
    streaming_topology_v1_emit_column(wb, "dst_actor", "actor_ref", "reference", false, NULL);
    streaming_topology_v1_emit_column(wb, "type", "string", "group_key", false, NULL);
    streaming_topology_v1_emit_column(wb, "state", "string", "attribute", true, NULL);
    streaming_topology_v1_emit_column(wb, "port_name", "string", "attribute", true, NULL);
    streaming_topology_v1_emit_column(wb, "discovered_at", "timestamp", "timestamp", true, NULL);
    streaming_topology_v1_emit_column(wb, "last_seen", "timestamp", "timestamp", true, NULL);
    streaming_topology_v1_emit_column(wb, "hops", "int", "metric", false, "max");
    streaming_topology_v1_emit_column(wb, "evidence_count", "uint", "metric", false, "sum");
    streaming_topology_v1_emit_column(wb, "connections", "uint", "metric", false, "sum");
    streaming_topology_v1_emit_column(wb, "replication_instances", "uint", "metric", false, "sum");
    streaming_topology_v1_emit_column(wb, "replication_completion", "float", "metric", false, "avg");
    streaming_topology_v1_emit_column(wb, "collected_metrics", "uint", "metric", false, "sum");
    streaming_topology_v1_emit_column(wb, "collected_instances", "uint", "metric", false, "sum");
    streaming_topology_v1_emit_column(wb, "collected_contexts", "uint", "metric", false, "sum");
}

static void streaming_topology_v1_emit_actor_label_columns(BUFFER *wb) {
    streaming_topology_v1_emit_column(wb, "actor", "actor_ref", "reference", false, NULL);
    streaming_topology_v1_emit_column(wb, "key", "string", "attribute", false, NULL);
    streaming_topology_v1_emit_column(wb, "value", "string", "attribute", false, NULL);
    streaming_topology_v1_emit_column(wb, "source", "string", "attribute", true, NULL);
    streaming_topology_v1_emit_column(wb, "kind", "string", "attribute", true, NULL);
    streaming_topology_v1_emit_column(wb, "value_index", "uint", "attribute", true, NULL);
}

static void streaming_topology_v1_emit_evidence_columns(BUFFER *wb) {
    streaming_topology_v1_emit_column(wb, "link", "link_ref", "reference", false, NULL);
    streaming_topology_v1_emit_column(wb, "src_actor", "actor_ref", "reference", false, NULL);
    streaming_topology_v1_emit_column(wb, "dst_actor", "actor_ref", "reference", false, NULL);
    streaming_topology_v1_emit_column(wb, "type", "string", "group_key", false, NULL);
    streaming_topology_v1_emit_column(wb, "state", "string", "attribute", true, NULL);
    streaming_topology_v1_emit_column(wb, "port_name", "string", "attribute", true, NULL);
    streaming_topology_v1_emit_column(wb, "discovered_at", "timestamp", "timestamp", true, NULL);
    streaming_topology_v1_emit_column(wb, "last_seen", "timestamp", "timestamp", true, NULL);
    streaming_topology_v1_emit_column(wb, "hops", "int", "metric", false, "max");
    streaming_topology_v1_emit_column(wb, "connections", "uint", "metric", false, "sum");
    streaming_topology_v1_emit_column(wb, "replication_instances", "uint", "metric", false, "sum");
    streaming_topology_v1_emit_column(wb, "replication_completion", "float", "metric", false, "avg");
    streaming_topology_v1_emit_column(wb, "collected_metrics", "uint", "metric", false, "sum");
    streaming_topology_v1_emit_column(wb, "collected_instances", "uint", "metric", false, "sum");
    streaming_topology_v1_emit_column(wb, "collected_contexts", "uint", "metric", false, "sum");
}

static void streaming_topology_v1_emit_stream_path_columns(BUFFER *wb) {
    streaming_topology_v1_emit_column(wb, "actor", "actor_ref", "reference", false, NULL);
    streaming_topology_v1_emit_column(wb, "path_actor", "actor_ref", "reference", false, NULL);
    streaming_topology_v1_emit_column(wb, "path_index", "uint", NULL, false, NULL);
    streaming_topology_v1_emit_column(wb, "hostname", "string", "attribute", true, NULL);
    streaming_topology_v1_emit_column(wb, "host_id", "string", "merge_identity", true, NULL);
    streaming_topology_v1_emit_column(wb, "node_id", "string", "merge_identity", true, NULL);
    streaming_topology_v1_emit_column(wb, "claim_id", "string", "attribute", true, NULL);
    streaming_topology_v1_emit_column(wb, "hops", "int", "metric", false, "max");
    streaming_topology_v1_emit_column(wb, "since", "timestamp", "timestamp", true, NULL);
    streaming_topology_v1_emit_column(wb, "first_time", "timestamp", "timestamp", true, NULL);
    streaming_topology_v1_emit_column(wb, "start_time_ms", "uint", "metric", false, "max");
    streaming_topology_v1_emit_column(wb, "shutdown_time_ms", "uint", "metric", false, "max");
    streaming_topology_v1_emit_column(wb, "capabilities", "uint", "attribute", false, NULL);
    streaming_topology_v1_emit_column(wb, "flags", "uint", "attribute", false, NULL);
}

static void streaming_topology_v1_emit_retention_columns(BUFFER *wb) {
    streaming_topology_v1_emit_column(wb, "actor", "actor_ref", "reference", false, NULL);
    streaming_topology_v1_emit_column(wb, "observer_actor", "actor_ref", "reference", false, NULL);
    streaming_topology_v1_emit_column(wb, "db_status", "string", "attribute", true, NULL);
    streaming_topology_v1_emit_column(wb, "db_from", "timestamp", "timestamp", true, NULL);
    streaming_topology_v1_emit_column(wb, "db_to", "timestamp", "timestamp", true, NULL);
    streaming_topology_v1_emit_column(wb, "db_duration", "duration", "metric", false, "max");
    streaming_topology_v1_emit_column(wb, "db_metrics", "uint", "metric", false, "sum");
    streaming_topology_v1_emit_column(wb, "db_instances", "uint", "metric", false, "sum");
    streaming_topology_v1_emit_column(wb, "db_contexts", "uint", "metric", false, "sum");
}

static void streaming_topology_v1_emit_inbound_columns(BUFFER *wb) {
    streaming_topology_v1_emit_column(wb, "parent_actor", "actor_ref", "reference", false, NULL);
    streaming_topology_v1_emit_column(wb, "child_actor", "actor_ref", "reference", false, NULL);
    streaming_topology_v1_emit_column(wb, "source_actor", "actor_ref", "reference", true, NULL);
    streaming_topology_v1_emit_column(wb, "received_type", "string", "attribute", true, NULL);
    streaming_topology_v1_emit_column(wb, "ingest_status", "string", "attribute", true, NULL);
    streaming_topology_v1_emit_column(wb, "hops", "int", "metric", false, "max");
    streaming_topology_v1_emit_column(wb, "collected_metrics", "uint", "metric", false, "sum");
    streaming_topology_v1_emit_column(wb, "collected_instances", "uint", "metric", false, "sum");
    streaming_topology_v1_emit_column(wb, "collected_contexts", "uint", "metric", false, "sum");
    streaming_topology_v1_emit_column(wb, "replication_completion", "float", "metric", false, "avg");
    streaming_topology_v1_emit_column(wb, "ingest_age", "duration", "metric", false, "max");
    streaming_topology_v1_emit_column(wb, "ssl", "string", "attribute", true, NULL);
    streaming_topology_v1_emit_column(wb, "alerts_critical", "uint", "metric", false, "sum");
    streaming_topology_v1_emit_column(wb, "alerts_warning", "uint", "metric", false, "sum");
}

static void streaming_topology_v1_emit_outbound_columns(BUFFER *wb) {
    streaming_topology_v1_emit_column(wb, "actor", "actor_ref", "reference", false, NULL);
    streaming_topology_v1_emit_column(wb, "destination_actor", "actor_ref", "reference", true, NULL);
    streaming_topology_v1_emit_column(wb, "stream_status", "string", "attribute", true, NULL);
    streaming_topology_v1_emit_column(wb, "hops", "int", "metric", false, "max");
    streaming_topology_v1_emit_column(wb, "ssl", "string", "attribute", true, NULL);
    streaming_topology_v1_emit_column(wb, "compression", "string", "attribute", true, NULL);
}

static void streaming_topology_v1_emit_modal_direct_column(
    BUFFER *wb,
    const char *id,
    const char *label,
    const char *column,
    const char *cell) {
    buffer_json_add_array_item_object(wb);
    {
        buffer_json_member_add_string(wb, "id", id);
        buffer_json_member_add_string(wb, "label", label);
        buffer_json_member_add_object(wb, "projection");
        {
            buffer_json_member_add_string(wb, "kind", "direct");
            buffer_json_member_add_string(wb, "column", column);
        }
        buffer_json_object_close(wb);
        buffer_json_member_add_string(wb, "cell", cell);
    }
    buffer_json_object_close(wb);
}

static void streaming_topology_v1_emit_modal_actor_ref_column(
    BUFFER *wb,
    const char *id,
    const char *label,
    const char *actor_column) {
    buffer_json_add_array_item_object(wb);
    {
        buffer_json_member_add_string(wb, "id", id);
        buffer_json_member_add_string(wb, "label", label);
        buffer_json_member_add_object(wb, "projection");
        {
            buffer_json_member_add_string(wb, "kind", "actor_ref_label");
            buffer_json_member_add_string(wb, "actor_column", actor_column);
        }
        buffer_json_object_close(wb);
        buffer_json_member_add_string(wb, "cell", "actor_link");
    }
    buffer_json_object_close(wb);
}

static void streaming_topology_v1_emit_modal_label_lookup_column(
    BUFFER *wb,
    const char *id,
    const char *label,
    const char *actor_column,
    const char *label_key,
    const char *cell) {
    buffer_json_add_array_item_object(wb);
    {
        buffer_json_member_add_string(wb, "id", id);
        buffer_json_member_add_string(wb, "label", label);
        buffer_json_member_add_object(wb, "projection");
        {
            buffer_json_member_add_string(wb, "kind", "label_lookup");
            if(actor_column)
                buffer_json_member_add_string(wb, "actor_column", actor_column);
            buffer_json_member_add_string(wb, "label_key", label_key);
        }
        buffer_json_object_close(wb);
        buffer_json_member_add_string(wb, "cell", cell);
    }
    buffer_json_object_close(wb);
}

static void streaming_topology_v1_emit_modal_actor_table_source(BUFFER *wb, const char *table) {
    buffer_json_member_add_object(wb, "source");
    {
        buffer_json_member_add_string(wb, "kind", "actor_table");
        buffer_json_member_add_string(wb, "table", table);
    }
    buffer_json_object_close(wb);
}

static void streaming_topology_v1_emit_modal_actor_owner_filter(BUFFER *wb, const char *actor_column) {
    buffer_json_member_add_object(wb, "owner_filter");
    {
        buffer_json_member_add_string(wb, "mode", "actor_column");
        buffer_json_member_add_string(wb, "actor_column", actor_column);
    }
    buffer_json_object_close(wb);
}

static void streaming_topology_v1_emit_modal_sort(BUFFER *wb, const char *column, const char *direction) {
    buffer_json_member_add_object(wb, "sort");
    {
        buffer_json_member_add_string(wb, "column", column);
        buffer_json_member_add_string(wb, "direction", direction);
    }
    buffer_json_object_close(wb);
}

static void streaming_topology_v1_emit_modal_identification_field(BUFFER *wb, const char *key, const char *label) {
    buffer_json_add_array_item_object(wb);
    {
        buffer_json_member_add_string(wb, "key", key);
        buffer_json_member_add_string(wb, "label", label);
        buffer_json_member_add_uint64(wb, "max_values", 1);
    }
    buffer_json_object_close(wb);
}

static void streaming_topology_v1_emit_modal_label_identification(BUFFER *wb) {
    buffer_json_member_add_object(wb, "identification");
    {
        buffer_json_member_add_boolean(wb, "enabled", true);
        buffer_json_member_add_array(wb, "fields");
        {
            streaming_topology_v1_emit_modal_identification_field(wb, "hostname", "Hostname");
            streaming_topology_v1_emit_modal_identification_field(wb, "type", "Node Type");
            streaming_topology_v1_emit_modal_identification_field(wb, "stream_status", "Stream");
            streaming_topology_v1_emit_modal_identification_field(wb, "ingest_status", "Ingest");
            streaming_topology_v1_emit_modal_identification_field(wb, "machine_guid", "Machine GUID");
            streaming_topology_v1_emit_modal_identification_field(wb, "agent_version", "Agent");
        }
        buffer_json_array_close(wb);
    }
    buffer_json_object_close(wb);
}

static void streaming_topology_v1_emit_actor_modal(BUFFER *wb) {
    buffer_json_member_add_object(wb, "modal");
    {
        buffer_json_member_add_boolean(wb, "enabled", true);
        buffer_json_member_add_object(wb, "labels");
        {
            buffer_json_member_add_boolean(wb, "enabled", true);
            buffer_json_member_add_string(wb, "table", "actor_labels");
            streaming_topology_v1_emit_modal_label_identification(wb);
        }
        buffer_json_object_close(wb);
        buffer_json_member_add_object(wb, "mini_topology");
        {
            buffer_json_member_add_boolean(wb, "enabled", true);
            buffer_json_member_add_uint64(wb, "depth", 1);
        }
        buffer_json_object_close(wb);
        buffer_json_member_add_array(wb, "sections");
        {
            buffer_json_add_array_item_object(wb);
            {
                buffer_json_member_add_string(wb, "id", "stream_path");
                buffer_json_member_add_string(wb, "label", "Stream path");
                buffer_json_member_add_uint64(wb, "order", 1);
                streaming_topology_v1_emit_modal_actor_table_source(wb, "stream_path");
                streaming_topology_v1_emit_modal_actor_owner_filter(wb, "actor");
                buffer_json_member_add_array(wb, "columns");
                {
                    streaming_topology_v1_emit_modal_direct_column(wb, "path_index", "Hop", "path_index", "number");
                    streaming_topology_v1_emit_modal_actor_ref_column(wb, "path_actor", "Node", "path_actor");
                    streaming_topology_v1_emit_modal_direct_column(wb, "hostname", "Hostname", "hostname", "text");
                    streaming_topology_v1_emit_modal_direct_column(wb, "hops", "Hops", "hops", "number");
                    streaming_topology_v1_emit_modal_direct_column(wb, "since", "Since", "since", "timestamp");
                    streaming_topology_v1_emit_modal_direct_column(wb, "first_time", "First seen", "first_time", "timestamp");
                }
                buffer_json_array_close(wb);
                streaming_topology_v1_emit_modal_sort(wb, "path_index", "asc");
                buffer_json_member_add_string(wb, "empty_label", "No stream path rows");
            }
            buffer_json_object_close(wb);

            buffer_json_add_array_item_object(wb);
            {
                buffer_json_member_add_string(wb, "id", "retention");
                buffer_json_member_add_string(wb, "label", "Retention");
                buffer_json_member_add_uint64(wb, "order", 2);
                streaming_topology_v1_emit_modal_actor_table_source(wb, "retention");
                streaming_topology_v1_emit_modal_actor_owner_filter(wb, "actor");
                buffer_json_member_add_array(wb, "columns");
                {
                    streaming_topology_v1_emit_modal_direct_column(wb, "db_status", "Status", "db_status", "badge");
                    streaming_topology_v1_emit_modal_direct_column(wb, "db_from", "From", "db_from", "timestamp");
                    streaming_topology_v1_emit_modal_direct_column(wb, "db_to", "To", "db_to", "timestamp");
                    streaming_topology_v1_emit_modal_direct_column(wb, "db_duration", "Duration", "db_duration", "duration");
                    streaming_topology_v1_emit_modal_direct_column(wb, "db_metrics", "Metrics", "db_metrics", "number");
                    streaming_topology_v1_emit_modal_direct_column(wb, "db_contexts", "Contexts", "db_contexts", "number");
                }
                buffer_json_array_close(wb);
                buffer_json_member_add_string(wb, "empty_label", "No retention rows");
            }
            buffer_json_object_close(wb);

            buffer_json_add_array_item_object(wb);
            {
                buffer_json_member_add_string(wb, "id", "inbound");
                buffer_json_member_add_string(wb, "label", "Inbound children");
                buffer_json_member_add_uint64(wb, "order", 3);
                streaming_topology_v1_emit_modal_actor_table_source(wb, "inbound");
                streaming_topology_v1_emit_modal_actor_owner_filter(wb, "parent_actor");
                buffer_json_member_add_array(wb, "columns");
                {
                    streaming_topology_v1_emit_modal_actor_ref_column(wb, "child", "Child", "child_actor");
                    streaming_topology_v1_emit_modal_actor_ref_column(wb, "source", "Received from", "source_actor");
                    streaming_topology_v1_emit_modal_label_lookup_column(wb, "child_type", "Node type", "child_actor", "type", "badge");
                    streaming_topology_v1_emit_modal_direct_column(wb, "received_type", "Type", "received_type", "badge");
                    streaming_topology_v1_emit_modal_direct_column(wb, "ingest_status", "Ingest", "ingest_status", "badge");
                    streaming_topology_v1_emit_modal_direct_column(wb, "hops", "Hops", "hops", "number");
                    streaming_topology_v1_emit_modal_direct_column(wb, "collected_metrics", "Metrics", "collected_metrics", "number");
                    streaming_topology_v1_emit_modal_direct_column(wb, "collected_instances", "Instances", "collected_instances", "number");
                    streaming_topology_v1_emit_modal_direct_column(wb, "collected_contexts", "Contexts", "collected_contexts", "number");
                    streaming_topology_v1_emit_modal_direct_column(wb, "replication_completion", "Replication", "replication_completion", "number");
                    streaming_topology_v1_emit_modal_direct_column(wb, "ingest_age", "Age", "ingest_age", "duration");
                    streaming_topology_v1_emit_modal_direct_column(wb, "ssl", "TLS", "ssl", "badge");
                    streaming_topology_v1_emit_modal_direct_column(wb, "alerts_critical", "Critical", "alerts_critical", "number");
                    streaming_topology_v1_emit_modal_direct_column(wb, "alerts_warning", "Warning", "alerts_warning", "number");
                }
                buffer_json_array_close(wb);
                buffer_json_member_add_string(wb, "empty_label", "No inbound children");
            }
            buffer_json_object_close(wb);

            buffer_json_add_array_item_object(wb);
            {
                buffer_json_member_add_string(wb, "id", "outbound");
                buffer_json_member_add_string(wb, "label", "Outbound stream");
                buffer_json_member_add_uint64(wb, "order", 4);
                streaming_topology_v1_emit_modal_actor_table_source(wb, "outbound");
                streaming_topology_v1_emit_modal_actor_owner_filter(wb, "actor");
                buffer_json_member_add_array(wb, "columns");
                {
                    streaming_topology_v1_emit_modal_actor_ref_column(wb, "actor", "Node", "actor");
                    streaming_topology_v1_emit_modal_label_lookup_column(wb, "actor_type", "Node type", "actor", "type", "badge");
                    streaming_topology_v1_emit_modal_actor_ref_column(wb, "destination", "Destination", "destination_actor");
                    streaming_topology_v1_emit_modal_direct_column(wb, "stream_status", "Status", "stream_status", "badge");
                    streaming_topology_v1_emit_modal_direct_column(wb, "hops", "Hops", "hops", "number");
                    streaming_topology_v1_emit_modal_direct_column(wb, "ssl", "TLS", "ssl", "badge");
                    streaming_topology_v1_emit_modal_direct_column(wb, "compression", "Compression", "compression", "badge");
                }
                buffer_json_array_close(wb);
                buffer_json_member_add_string(wb, "empty_label", "No outbound stream row");
            }
            buffer_json_object_close(wb);
        }
        buffer_json_array_close(wb);
    }
    buffer_json_object_close(wb);
}

static void streaming_topology_v1_emit_actor_type(
    BUFFER *wb,
    const char *id,
    const char *label,
    const char *color_slot,
    const char *icon,
    bool border,
    bool size_by_links,
    bool show_port_bullets) {
    buffer_json_member_add_object(wb, id);
    {
        buffer_json_member_add_string(wb, "layer", "streaming");
        buffer_json_member_add_array(wb, "identity");
        buffer_json_add_array_item_string(wb, "id");
        buffer_json_array_close(wb);
        buffer_json_member_add_array(wb, "merge_identity");
        buffer_json_add_array_item_string(wb, "machine_guid");
        buffer_json_add_array_item_string(wb, "node_id");
        buffer_json_array_close(wb);
        buffer_json_member_add_array(wb, "aggregation_scopes");
        buffer_json_add_array_item_string(wb, "node");
        buffer_json_array_close(wb);
        buffer_json_member_add_object(wb, "presentation");
        {
            buffer_json_member_add_string(wb, "label", label);
            buffer_json_member_add_string(wb, "role", "actor");
            buffer_json_member_add_string(wb, "icon", icon);
            buffer_json_member_add_string(wb, "color_slot", color_slot);
            buffer_json_member_add_string(wb, "opacity", strcmp(id, "stale") == 0 ? "faded" : "normal");
            buffer_json_member_add_object(wb, "border");
            {
                buffer_json_member_add_boolean(wb, "enabled", border);
            }
            buffer_json_object_close(wb);
            buffer_json_member_add_object(wb, "size");
            {
                buffer_json_member_add_string(wb, "mode", size_by_links ? "link_count" : "fixed");
            }
            buffer_json_object_close(wb);
            buffer_json_member_add_object(wb, "label_policy");
            {
                buffer_json_member_add_array(wb, "columns");
                buffer_json_add_array_item_string(wb, "display_name");
                buffer_json_add_array_item_string(wb, "hostname");
                buffer_json_array_close(wb);
                buffer_json_member_add_string(wb, "fallback", "type_label");
                buffer_json_member_add_uint64(wb, "max_length", 80);
                buffer_json_member_add_string(wb, "array", "reject");
            }
            buffer_json_object_close(wb);
            buffer_json_member_add_object(wb, "ports");
            {
                buffer_json_member_add_boolean(wb, "show_bullets", show_port_bullets);
                if(show_port_bullets) {
                    buffer_json_member_add_array(wb, "sources");
                    {
                        buffer_json_add_array_item_object(wb);
                        buffer_json_member_add_string(wb, "source", "links");
                        buffer_json_member_add_string(wb, "actor_column", "src_actor");
                        buffer_json_member_add_string(wb, "name_column", "port_name");
                        buffer_json_member_add_string(wb, "type_column", "type");
                        buffer_json_member_add_string(wb, "default_type", "streaming");
                        buffer_json_object_close(wb);
                    }
                    buffer_json_array_close(wb);
                }
            }
            buffer_json_object_close(wb);
            streaming_topology_v1_emit_actor_modal(wb);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb);
}

static void streaming_topology_v1_emit_link_type(
    BUFFER *wb,
    const char *id,
    const char *direction_role,
    const char *evidence_type,
    const char *label,
    const char *color_slot,
    const char *line_style,
    const char *width,
    const char *opacity) {
    buffer_json_member_add_object(wb, id);
    {
        buffer_json_member_add_string(wb, "orientation", "directed");
        buffer_json_member_add_string(wb, "direction_role", direction_role);
        buffer_json_member_add_object(wb, "aggregation");
        {
            buffer_json_member_add_string(wb, "direction", "preserve");
            buffer_json_member_add_string(wb, "evidence", "append");
            buffer_json_member_add_object(wb, "metrics");
            {
                buffer_json_member_add_string(wb, "hops", "max");
                buffer_json_member_add_string(wb, "evidence_count", "sum");
                buffer_json_member_add_string(wb, "connections", "sum");
                buffer_json_member_add_string(wb, "replication_instances", "sum");
                buffer_json_member_add_string(wb, "replication_completion", "avg");
                buffer_json_member_add_string(wb, "collected_metrics", "sum");
                buffer_json_member_add_string(wb, "collected_instances", "sum");
                buffer_json_member_add_string(wb, "collected_contexts", "sum");
            }
            buffer_json_object_close(wb);
        }
        buffer_json_object_close(wb);
        buffer_json_member_add_array(wb, "evidence_types");
        buffer_json_add_array_item_string(wb, evidence_type);
        buffer_json_array_close(wb);
        buffer_json_member_add_object(wb, "presentation");
        {
            buffer_json_member_add_string(wb, "label", label);
            buffer_json_member_add_string(wb, "color_slot", color_slot);
            buffer_json_member_add_string(wb, "line_style", line_style);
            buffer_json_member_add_string(wb, "width", width);
            buffer_json_member_add_string(wb, "opacity", opacity);
            buffer_json_member_add_string(wb, "curve", "auto");
            buffer_json_member_add_string(wb, "arrow", "forward");
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb);
}

static void streaming_topology_v1_emit_evidence_type(BUFFER *wb, const char *id, const char *link_type) {
    buffer_json_member_add_object(wb, id);
    {
        buffer_json_member_add_string(wb, "link_type", link_type);
        buffer_json_member_add_string(wb, "role", "relationship_evidence");
        buffer_json_member_add_array(wb, "columns");
        streaming_topology_v1_emit_evidence_columns(wb);
        buffer_json_array_close(wb);
        buffer_json_member_add_array(wb, "match_columns");
        buffer_json_add_array_item_string(wb, "src_actor");
        buffer_json_add_array_item_string(wb, "dst_actor");
        buffer_json_add_array_item_string(wb, "type");
        buffer_json_array_close(wb);
    }
    buffer_json_object_close(wb);
}

static void streaming_topology_v1_emit_table_type(
    BUFFER *wb,
    const char *id,
    const char *role,
    const char *owner,
    const char *aggregation,
    void (*emit_columns)(BUFFER *)) {
    buffer_json_member_add_object(wb, id);
    {
        buffer_json_member_add_string(wb, "role", role);
        buffer_json_member_add_string(wb, "owner", owner);
        buffer_json_member_add_string(wb, "aggregation", aggregation);
        buffer_json_member_add_array(wb, "columns");
        emit_columns(wb);
        buffer_json_array_close(wb);
    }
    buffer_json_object_close(wb);
}

static void streaming_topology_v1_emit_type_registry(BUFFER *wb) {
    buffer_json_member_add_object(wb, "types");
    {
        buffer_json_member_add_object(wb, "actor_types");
        {
            streaming_topology_v1_emit_actor_type(wb, "parent", "Netdata Parent", "primary", "parent", true, true, true);
            streaming_topology_v1_emit_actor_type(wb, "child", "Netdata Child", "primary", "netdata-agent", false, false, false);
            streaming_topology_v1_emit_actor_type(wb, "vnode", "Virtual Node", "warning", "netdata-agent", false, false, false);
            streaming_topology_v1_emit_actor_type(wb, "stale", "Stale Node", "dim", "netdata-agent", false, false, false);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "link_types");
        {
            streaming_topology_v1_emit_link_type(
                wb, "streaming", "dependency", "streaming_link",
                "Streaming", "primary", "solid", "thick", "normal");
            streaming_topology_v1_emit_link_type(
                wb, "virtual", "dependency", "virtual_link",
                "Virtual origin", "warning", "dashed", "thin", "muted");
            streaming_topology_v1_emit_link_type(
                wb, "stale", "observation", "stale_link",
                "Stale data", "dim", "dashed", "thin", "faded");
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "port_types");
        {
            buffer_json_member_add_object(wb, "streaming");
            {
                buffer_json_member_add_object(wb, "presentation");
                {
                    buffer_json_member_add_string(wb, "label", "Streaming child");
                    buffer_json_member_add_string(wb, "color_slot", "primary");
                    buffer_json_member_add_string(wb, "opacity", "normal");
                }
                buffer_json_object_close(wb);
            }
            buffer_json_object_close(wb);

            buffer_json_member_add_object(wb, "virtual");
            {
                buffer_json_member_add_object(wb, "presentation");
                {
                    buffer_json_member_add_string(wb, "label", "Virtual node");
                    buffer_json_member_add_string(wb, "color_slot", "warning");
                    buffer_json_member_add_string(wb, "opacity", "normal");
                }
                buffer_json_object_close(wb);
            }
            buffer_json_object_close(wb);

            buffer_json_member_add_object(wb, "stale");
            {
                buffer_json_member_add_object(wb, "presentation");
                {
                    buffer_json_member_add_string(wb, "label", "Stale node");
                    buffer_json_member_add_string(wb, "color_slot", "dim");
                    buffer_json_member_add_string(wb, "opacity", "faded");
                }
                buffer_json_object_close(wb);
            }
            buffer_json_object_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "evidence_types");
        {
            streaming_topology_v1_emit_evidence_type(wb, "streaming_link", "streaming");
            streaming_topology_v1_emit_evidence_type(wb, "virtual_link", "virtual");
            streaming_topology_v1_emit_evidence_type(wb, "stale_link", "stale");
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "table_types");
        {
            streaming_topology_v1_emit_table_type(wb, "actor_labels", "actor_inventory", "actor", "set",
                streaming_topology_v1_emit_actor_label_columns);
            streaming_topology_v1_emit_table_type(wb, "stream_path", "actor_detail", "actor", "append",
                streaming_topology_v1_emit_stream_path_columns);
            streaming_topology_v1_emit_table_type(wb, "retention", "actor_detail", "actor", "append",
                streaming_topology_v1_emit_retention_columns);
            streaming_topology_v1_emit_table_type(wb, "inbound", "relationship_summary", "actor", "append",
                streaming_topology_v1_emit_inbound_columns);
            streaming_topology_v1_emit_table_type(wb, "outbound", "relationship_summary", "actor", "append",
                streaming_topology_v1_emit_outbound_columns);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "aggregation_scopes");
        {
            buffer_json_member_add_object(wb, "node");
            {
                buffer_json_member_add_array(wb, "columns");
                buffer_json_add_array_item_string(wb, "machine_guid");
                buffer_json_add_array_item_string(wb, "node_id");
                buffer_json_array_close(wb);
                buffer_json_member_add_string(wb, "evidence_policy", "preserve");
            }
            buffer_json_object_close(wb);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb);
}

static void streaming_topology_v1_emit_presentation(BUFFER *wb) {
    buffer_json_member_add_object(wb, "presentation");
    {
        buffer_json_member_add_string(wb, "profile_version", "streaming.v1");
        buffer_json_member_add_object(wb, "selection");
        {
            buffer_json_member_add_object(wb, "actor_click");
            {
                buffer_json_member_add_string(wb, "mode", "highlight_path");
                buffer_json_member_add_string(wb, "path_table", "stream_path");
                buffer_json_member_add_string(wb, "path_owner_column", "actor");
                buffer_json_member_add_string(wb, "path_actor_column", "path_actor");
                buffer_json_member_add_string(wb, "path_order_column", "path_index");
            }
            buffer_json_object_close(wb);
        }
        buffer_json_object_close(wb);

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
            buffer_json_array_close(wb);

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
            buffer_json_array_close(wb);

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
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_array(wb, "port_fields");
        {
            buffer_json_add_array_item_object(wb);
            buffer_json_member_add_string(wb, "key", "type");
            buffer_json_member_add_string(wb, "label", "Type");
            buffer_json_object_close(wb);
        }
        buffer_json_array_close(wb);
    }
    buffer_json_object_close(wb);
}

static void streaming_topology_v1_emit_actor_table(BUFFER *wb, STREAMING_TOPOLOGY_V1_PAYLOAD *payload) {
    buffer_json_member_add_object(wb, "actors");
    {
        buffer_json_member_add_uint64(wb, "rows", payload->actors_used);
        buffer_json_member_add_array(wb, "columns");
        streaming_topology_v1_emit_actor_columns(wb);
        buffer_json_array_close(wb);

        buffer_json_member_add_array(wb, "values");
#define STREAMING_TOPOLOGY_ACTOR_STRING_VALUES(member) do { \
            streaming_topology_v1_emit_values_start(wb); \
            for(size_t i = 0; i < payload->actors_used; i++) \
                buffer_json_add_array_item_string(wb, payload->actors[i].member[0] ? payload->actors[i].member : NULL); \
            streaming_topology_v1_emit_values_end(wb); \
        } while(0)

        STREAMING_TOPOLOGY_ACTOR_STRING_VALUES(actor_id);
        STREAMING_TOPOLOGY_ACTOR_STRING_VALUES(type);
        streaming_topology_v1_emit_values_start(wb);
        for(size_t i = 0; i < payload->actors_used; i++)
            buffer_json_add_array_item_string(wb, "streaming");
        streaming_topology_v1_emit_values_end(wb);
        STREAMING_TOPOLOGY_ACTOR_STRING_VALUES(machine_guid);
        STREAMING_TOPOLOGY_ACTOR_STRING_VALUES(node_id);
        STREAMING_TOPOLOGY_ACTOR_STRING_VALUES(hostname);
        STREAMING_TOPOLOGY_ACTOR_STRING_VALUES(display_name);
        STREAMING_TOPOLOGY_ACTOR_STRING_VALUES(severity);
        STREAMING_TOPOLOGY_ACTOR_STRING_VALUES(ephemerality);
        STREAMING_TOPOLOGY_ACTOR_STRING_VALUES(ingest_status);
        STREAMING_TOPOLOGY_ACTOR_STRING_VALUES(stream_status);
        STREAMING_TOPOLOGY_ACTOR_STRING_VALUES(ml_status);
        STREAMING_TOPOLOGY_ACTOR_STRING_VALUES(agent_name);
        STREAMING_TOPOLOGY_ACTOR_STRING_VALUES(agent_version);
        STREAMING_TOPOLOGY_ACTOR_STRING_VALUES(health_status);
        STREAMING_TOPOLOGY_ACTOR_STRING_VALUES(os_name);
        STREAMING_TOPOLOGY_ACTOR_STRING_VALUES(architecture);
        STREAMING_TOPOLOGY_ACTOR_STRING_VALUES(cpu_count);

#undef STREAMING_TOPOLOGY_ACTOR_STRING_VALUES
#define STREAMING_TOPOLOGY_ACTOR_UINT_VALUES(member) do { \
            streaming_topology_v1_emit_values_start(wb); \
            for(size_t i = 0; i < payload->actors_used; i++) \
                buffer_json_add_array_item_uint64(wb, payload->actors[i].member); \
            streaming_topology_v1_emit_values_end(wb); \
        } while(0)

        STREAMING_TOPOLOGY_ACTOR_UINT_VALUES(child_count);
        STREAMING_TOPOLOGY_ACTOR_UINT_VALUES(health_critical);
        STREAMING_TOPOLOGY_ACTOR_UINT_VALUES(health_warning);
        STREAMING_TOPOLOGY_ACTOR_UINT_VALUES(health_clear);
#undef STREAMING_TOPOLOGY_ACTOR_UINT_VALUES

        buffer_json_array_close(wb);
    }
    buffer_json_object_close(wb);
}

static void streaming_topology_v1_emit_link_table(BUFFER *wb, STREAMING_TOPOLOGY_V1_PAYLOAD *payload) {
    buffer_json_member_add_object(wb, "links");
    {
        buffer_json_member_add_uint64(wb, "rows", payload->links_used);
        buffer_json_member_add_array(wb, "columns");
        streaming_topology_v1_emit_link_columns(wb);
        buffer_json_array_close(wb);
        buffer_json_member_add_array(wb, "values");

        streaming_topology_v1_emit_values_start(wb);
        for(size_t i = 0; i < payload->links_used; i++)
            buffer_json_add_array_item_uint64(wb, payload->links[i].src_actor);
        streaming_topology_v1_emit_values_end(wb);

        streaming_topology_v1_emit_values_start(wb);
        for(size_t i = 0; i < payload->links_used; i++)
            buffer_json_add_array_item_uint64(wb, payload->links[i].dst_actor);
        streaming_topology_v1_emit_values_end(wb);

        streaming_topology_v1_emit_values_start(wb);
        for(size_t i = 0; i < payload->links_used; i++)
            buffer_json_add_array_item_string(wb, payload->links[i].type);
        streaming_topology_v1_emit_values_end(wb);

        streaming_topology_v1_emit_values_start(wb);
        for(size_t i = 0; i < payload->links_used; i++)
            buffer_json_add_array_item_string(wb, payload->links[i].state[0] ? payload->links[i].state : NULL);
        streaming_topology_v1_emit_values_end(wb);

        streaming_topology_v1_emit_values_start(wb);
        for(size_t i = 0; i < payload->links_used; i++)
            buffer_json_add_array_item_string(wb, payload->links[i].port_name[0] ? payload->links[i].port_name : NULL);
        streaming_topology_v1_emit_values_end(wb);

        streaming_topology_v1_emit_values_start(wb);
        for(size_t i = 0; i < payload->links_used; i++)
            streaming_topology_v1_add_timestamp(wb, payload->links[i].discovered_at_ut);
        streaming_topology_v1_emit_values_end(wb);

        streaming_topology_v1_emit_values_start(wb);
        for(size_t i = 0; i < payload->links_used; i++)
            streaming_topology_v1_add_timestamp(wb, payload->links[i].last_seen_ut);
        streaming_topology_v1_emit_values_end(wb);

        streaming_topology_v1_emit_values_start(wb);
        for(size_t i = 0; i < payload->links_used; i++)
            buffer_json_add_array_item_int64(wb, payload->links[i].hops);
        streaming_topology_v1_emit_values_end(wb);

        streaming_topology_v1_emit_values_start(wb);
        for(size_t i = 0; i < payload->links_used; i++)
            buffer_json_add_array_item_uint64(wb, 1);
        streaming_topology_v1_emit_values_end(wb);

        streaming_topology_v1_emit_values_start(wb);
        for(size_t i = 0; i < payload->links_used; i++)
            buffer_json_add_array_item_uint64(wb, payload->links[i].connections);
        streaming_topology_v1_emit_values_end(wb);

        streaming_topology_v1_emit_values_start(wb);
        for(size_t i = 0; i < payload->links_used; i++)
            buffer_json_add_array_item_uint64(wb, payload->links[i].replication_instances);
        streaming_topology_v1_emit_values_end(wb);

        streaming_topology_v1_emit_values_start(wb);
        for(size_t i = 0; i < payload->links_used; i++)
            buffer_json_add_array_item_double(wb, payload->links[i].replication_completion);
        streaming_topology_v1_emit_values_end(wb);

        streaming_topology_v1_emit_values_start(wb);
        for(size_t i = 0; i < payload->links_used; i++)
            buffer_json_add_array_item_uint64(wb, payload->links[i].collected_metrics);
        streaming_topology_v1_emit_values_end(wb);

        streaming_topology_v1_emit_values_start(wb);
        for(size_t i = 0; i < payload->links_used; i++)
            buffer_json_add_array_item_uint64(wb, payload->links[i].collected_instances);
        streaming_topology_v1_emit_values_end(wb);

        streaming_topology_v1_emit_values_start(wb);
        for(size_t i = 0; i < payload->links_used; i++)
            buffer_json_add_array_item_uint64(wb, payload->links[i].collected_contexts);
        streaming_topology_v1_emit_values_end(wb);

        buffer_json_array_close(wb);
    }
    buffer_json_object_close(wb);
}

static bool streaming_topology_v1_link_is_type(STREAMING_TOPOLOGY_V1_LINK *link, const char *link_type) {
    return link && link_type && strcmp(link->type, link_type) == 0;
}

static size_t streaming_topology_v1_count_links_by_type(STREAMING_TOPOLOGY_V1_PAYLOAD *payload, const char *link_type) {
    size_t count = 0;
    for(size_t i = 0; i < payload->links_used; i++) {
        if(streaming_topology_v1_link_is_type(&payload->links[i], link_type))
            count++;
    }
    return count;
}

static void streaming_topology_v1_emit_evidence_section(
    BUFFER *wb,
    STREAMING_TOPOLOGY_V1_PAYLOAD *payload,
    const char *evidence_type,
    const char *link_type) {
    buffer_json_member_add_object(wb, evidence_type);
    {
        buffer_json_member_add_string(wb, "type", evidence_type);
        buffer_json_member_add_object(wb, "table");
        {
            buffer_json_member_add_uint64(wb, "rows", streaming_topology_v1_count_links_by_type(payload, link_type));
            buffer_json_member_add_array(wb, "columns");
            streaming_topology_v1_emit_evidence_columns(wb);
            buffer_json_array_close(wb);
            buffer_json_member_add_array(wb, "values");

            streaming_topology_v1_emit_values_start(wb);
            for(size_t i = 0; i < payload->links_used; i++) {
                if(streaming_topology_v1_link_is_type(&payload->links[i], link_type))
                    buffer_json_add_array_item_uint64(wb, i);
            }
            streaming_topology_v1_emit_values_end(wb);

            streaming_topology_v1_emit_values_start(wb);
            for(size_t i = 0; i < payload->links_used; i++) {
                if(streaming_topology_v1_link_is_type(&payload->links[i], link_type))
                    buffer_json_add_array_item_uint64(wb, payload->links[i].src_actor);
            }
            streaming_topology_v1_emit_values_end(wb);

            streaming_topology_v1_emit_values_start(wb);
            for(size_t i = 0; i < payload->links_used; i++) {
                if(streaming_topology_v1_link_is_type(&payload->links[i], link_type))
                    buffer_json_add_array_item_uint64(wb, payload->links[i].dst_actor);
            }
            streaming_topology_v1_emit_values_end(wb);

            streaming_topology_v1_emit_values_start(wb);
            for(size_t i = 0; i < payload->links_used; i++) {
                if(streaming_topology_v1_link_is_type(&payload->links[i], link_type))
                    buffer_json_add_array_item_string(wb, payload->links[i].type);
            }
            streaming_topology_v1_emit_values_end(wb);

            streaming_topology_v1_emit_values_start(wb);
            for(size_t i = 0; i < payload->links_used; i++) {
                if(streaming_topology_v1_link_is_type(&payload->links[i], link_type))
                    buffer_json_add_array_item_string(wb, payload->links[i].state[0] ? payload->links[i].state : NULL);
            }
            streaming_topology_v1_emit_values_end(wb);

            streaming_topology_v1_emit_values_start(wb);
            for(size_t i = 0; i < payload->links_used; i++) {
                if(streaming_topology_v1_link_is_type(&payload->links[i], link_type))
                    buffer_json_add_array_item_string(wb, payload->links[i].port_name[0] ? payload->links[i].port_name : NULL);
            }
            streaming_topology_v1_emit_values_end(wb);

            streaming_topology_v1_emit_values_start(wb);
            for(size_t i = 0; i < payload->links_used; i++) {
                if(streaming_topology_v1_link_is_type(&payload->links[i], link_type))
                    streaming_topology_v1_add_timestamp(wb, payload->links[i].discovered_at_ut);
            }
            streaming_topology_v1_emit_values_end(wb);

            streaming_topology_v1_emit_values_start(wb);
            for(size_t i = 0; i < payload->links_used; i++) {
                if(streaming_topology_v1_link_is_type(&payload->links[i], link_type))
                    streaming_topology_v1_add_timestamp(wb, payload->links[i].last_seen_ut);
            }
            streaming_topology_v1_emit_values_end(wb);

#define STREAMING_TOPOLOGY_LINK_INT_VALUES(member) do { \
                streaming_topology_v1_emit_values_start(wb); \
                for(size_t i = 0; i < payload->links_used; i++) { \
                    if(streaming_topology_v1_link_is_type(&payload->links[i], link_type)) \
                        buffer_json_add_array_item_int64(wb, payload->links[i].member); \
                } \
                streaming_topology_v1_emit_values_end(wb); \
            } while(0)
#define STREAMING_TOPOLOGY_LINK_UINT_VALUES(member) do { \
                streaming_topology_v1_emit_values_start(wb); \
                for(size_t i = 0; i < payload->links_used; i++) { \
                    if(streaming_topology_v1_link_is_type(&payload->links[i], link_type)) \
                        buffer_json_add_array_item_uint64(wb, payload->links[i].member); \
                } \
                streaming_topology_v1_emit_values_end(wb); \
            } while(0)

            STREAMING_TOPOLOGY_LINK_INT_VALUES(hops);
            STREAMING_TOPOLOGY_LINK_UINT_VALUES(connections);
            STREAMING_TOPOLOGY_LINK_UINT_VALUES(replication_instances);
#undef STREAMING_TOPOLOGY_LINK_INT_VALUES

            streaming_topology_v1_emit_values_start(wb);
            for(size_t i = 0; i < payload->links_used; i++) {
                if(streaming_topology_v1_link_is_type(&payload->links[i], link_type))
                    buffer_json_add_array_item_double(wb, payload->links[i].replication_completion);
            }
            streaming_topology_v1_emit_values_end(wb);

            STREAMING_TOPOLOGY_LINK_UINT_VALUES(collected_metrics);
            STREAMING_TOPOLOGY_LINK_UINT_VALUES(collected_instances);
            STREAMING_TOPOLOGY_LINK_UINT_VALUES(collected_contexts);
#undef STREAMING_TOPOLOGY_LINK_UINT_VALUES

            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb);
}

static void streaming_topology_v1_emit_evidence_table(BUFFER *wb, STREAMING_TOPOLOGY_V1_PAYLOAD *payload) {
    buffer_json_member_add_object(wb, "evidence");
    {
        streaming_topology_v1_emit_evidence_section(wb, payload, "streaming_link", "streaming");
        streaming_topology_v1_emit_evidence_section(wb, payload, "virtual_link", "virtual");
        streaming_topology_v1_emit_evidence_section(wb, payload, "stale_link", "stale");
    }
    buffer_json_object_close(wb);
}

static void streaming_topology_v1_emit_actor_labels_table(BUFFER *wb, STREAMING_TOPOLOGY_V1_PAYLOAD *payload) {
    buffer_json_member_add_object(wb, "actor_labels");
    {
        buffer_json_member_add_string(wb, "type", "actor_labels");
        buffer_json_member_add_object(wb, "table");
        {
            buffer_json_member_add_uint64(wb, "rows", payload->labels_used);
            buffer_json_member_add_array(wb, "columns");
            streaming_topology_v1_emit_actor_label_columns(wb);
            buffer_json_array_close(wb);
            buffer_json_member_add_array(wb, "values");

#define STREAMING_TOPOLOGY_LABEL_UINT_VALUES(member) do { \
                streaming_topology_v1_emit_values_start(wb); \
                for(size_t i = 0; i < payload->labels_used; i++) \
                    buffer_json_add_array_item_uint64(wb, payload->labels[i].member); \
                streaming_topology_v1_emit_values_end(wb); \
            } while(0)
#define STREAMING_TOPOLOGY_LABEL_STRING_VALUES(member) do { \
                streaming_topology_v1_emit_values_start(wb); \
                for(size_t i = 0; i < payload->labels_used; i++) \
                    buffer_json_add_array_item_string(wb, payload->labels[i].member[0] ? payload->labels[i].member : NULL); \
                streaming_topology_v1_emit_values_end(wb); \
            } while(0)

            STREAMING_TOPOLOGY_LABEL_UINT_VALUES(actor);
            STREAMING_TOPOLOGY_LABEL_STRING_VALUES(key);
            STREAMING_TOPOLOGY_LABEL_STRING_VALUES(value);
            STREAMING_TOPOLOGY_LABEL_STRING_VALUES(source);
            STREAMING_TOPOLOGY_LABEL_STRING_VALUES(kind);

            streaming_topology_v1_emit_values_start(wb);
            for(size_t i = 0; i < payload->labels_used; i++)
                streaming_topology_v1_add_nullable_uint(wb, payload->labels[i].has_value_index, payload->labels[i].value_index);
            streaming_topology_v1_emit_values_end(wb);

#undef STREAMING_TOPOLOGY_LABEL_UINT_VALUES
#undef STREAMING_TOPOLOGY_LABEL_STRING_VALUES

            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb);
}

static void streaming_topology_v1_emit_stream_path_table(BUFFER *wb, STREAMING_TOPOLOGY_V1_PAYLOAD *payload) {
    buffer_json_member_add_object(wb, "stream_path");
    {
        buffer_json_member_add_string(wb, "type", "stream_path");
        buffer_json_member_add_object(wb, "table");
        {
            buffer_json_member_add_uint64(wb, "rows", payload->stream_path_used);
            buffer_json_member_add_array(wb, "columns");
            streaming_topology_v1_emit_stream_path_columns(wb);
            buffer_json_array_close(wb);
            buffer_json_member_add_array(wb, "values");

#define STREAMING_TOPOLOGY_STREAM_PATH_UINT_VALUES(member) do { \
                streaming_topology_v1_emit_values_start(wb); \
                for(size_t i = 0; i < payload->stream_path_used; i++) \
                    buffer_json_add_array_item_uint64(wb, payload->stream_path_rows[i].member); \
                streaming_topology_v1_emit_values_end(wb); \
            } while(0)
#define STREAMING_TOPOLOGY_STREAM_PATH_STRING_VALUES(member) do { \
                streaming_topology_v1_emit_values_start(wb); \
                for(size_t i = 0; i < payload->stream_path_used; i++) \
                    buffer_json_add_array_item_string(wb, payload->stream_path_rows[i].member[0] ? payload->stream_path_rows[i].member : NULL); \
                streaming_topology_v1_emit_values_end(wb); \
            } while(0)

            STREAMING_TOPOLOGY_STREAM_PATH_UINT_VALUES(actor);
            STREAMING_TOPOLOGY_STREAM_PATH_UINT_VALUES(path_actor);
            STREAMING_TOPOLOGY_STREAM_PATH_UINT_VALUES(path_index);
            STREAMING_TOPOLOGY_STREAM_PATH_STRING_VALUES(hostname);
            STREAMING_TOPOLOGY_STREAM_PATH_STRING_VALUES(host_id);
            STREAMING_TOPOLOGY_STREAM_PATH_STRING_VALUES(node_id);
            STREAMING_TOPOLOGY_STREAM_PATH_STRING_VALUES(claim_id);

            streaming_topology_v1_emit_values_start(wb);
            for(size_t i = 0; i < payload->stream_path_used; i++)
                buffer_json_add_array_item_int64(wb, payload->stream_path_rows[i].hops);
            streaming_topology_v1_emit_values_end(wb);

            streaming_topology_v1_emit_values_start(wb);
            for(size_t i = 0; i < payload->stream_path_used; i++)
                streaming_topology_v1_add_timestamp(wb, payload->stream_path_rows[i].since_ut);
            streaming_topology_v1_emit_values_end(wb);

            streaming_topology_v1_emit_values_start(wb);
            for(size_t i = 0; i < payload->stream_path_used; i++)
                streaming_topology_v1_add_timestamp(wb, payload->stream_path_rows[i].first_time_ut);
            streaming_topology_v1_emit_values_end(wb);

            STREAMING_TOPOLOGY_STREAM_PATH_UINT_VALUES(start_time_ms);
            STREAMING_TOPOLOGY_STREAM_PATH_UINT_VALUES(shutdown_time_ms);
            STREAMING_TOPOLOGY_STREAM_PATH_UINT_VALUES(capabilities);
            STREAMING_TOPOLOGY_STREAM_PATH_UINT_VALUES(flags);

#undef STREAMING_TOPOLOGY_STREAM_PATH_UINT_VALUES
#undef STREAMING_TOPOLOGY_STREAM_PATH_STRING_VALUES
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb);
}

static void streaming_topology_v1_emit_retention_table(BUFFER *wb, STREAMING_TOPOLOGY_V1_PAYLOAD *payload) {
    buffer_json_member_add_object(wb, "retention");
    {
        buffer_json_member_add_string(wb, "type", "retention");
        buffer_json_member_add_object(wb, "table");
        {
            buffer_json_member_add_uint64(wb, "rows", payload->retention_used);
            buffer_json_member_add_array(wb, "columns");
            streaming_topology_v1_emit_retention_columns(wb);
            buffer_json_array_close(wb);
            buffer_json_member_add_array(wb, "values");

#define STREAMING_TOPOLOGY_RETENTION_UINT_VALUES(member) do { \
                streaming_topology_v1_emit_values_start(wb); \
                for(size_t i = 0; i < payload->retention_used; i++) \
                    buffer_json_add_array_item_uint64(wb, payload->retention_rows[i].member); \
                streaming_topology_v1_emit_values_end(wb); \
            } while(0)

            STREAMING_TOPOLOGY_RETENTION_UINT_VALUES(actor);
            STREAMING_TOPOLOGY_RETENTION_UINT_VALUES(observer_actor);

            streaming_topology_v1_emit_values_start(wb);
            for(size_t i = 0; i < payload->retention_used; i++)
                buffer_json_add_array_item_string(wb, payload->retention_rows[i].db_status[0] ? payload->retention_rows[i].db_status : NULL);
            streaming_topology_v1_emit_values_end(wb);

            streaming_topology_v1_emit_values_start(wb);
            for(size_t i = 0; i < payload->retention_used; i++)
                streaming_topology_v1_add_timestamp(wb, payload->retention_rows[i].db_from_ut);
            streaming_topology_v1_emit_values_end(wb);

            streaming_topology_v1_emit_values_start(wb);
            for(size_t i = 0; i < payload->retention_used; i++)
                streaming_topology_v1_add_timestamp(wb, payload->retention_rows[i].db_to_ut);
            streaming_topology_v1_emit_values_end(wb);

            STREAMING_TOPOLOGY_RETENTION_UINT_VALUES(db_duration);
            STREAMING_TOPOLOGY_RETENTION_UINT_VALUES(db_metrics);
            STREAMING_TOPOLOGY_RETENTION_UINT_VALUES(db_instances);
            STREAMING_TOPOLOGY_RETENTION_UINT_VALUES(db_contexts);
#undef STREAMING_TOPOLOGY_RETENTION_UINT_VALUES
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb);
}

static void streaming_topology_v1_emit_inbound_table(BUFFER *wb, STREAMING_TOPOLOGY_V1_PAYLOAD *payload) {
    buffer_json_member_add_object(wb, "inbound");
    {
        buffer_json_member_add_string(wb, "type", "inbound");
        buffer_json_member_add_object(wb, "table");
        {
            buffer_json_member_add_uint64(wb, "rows", payload->inbound_used);
            buffer_json_member_add_array(wb, "columns");
            streaming_topology_v1_emit_inbound_columns(wb);
            buffer_json_array_close(wb);
            buffer_json_member_add_array(wb, "values");

            streaming_topology_v1_emit_values_start(wb);
            for(size_t i = 0; i < payload->inbound_used; i++)
                buffer_json_add_array_item_uint64(wb, payload->inbound_rows[i].parent_actor);
            streaming_topology_v1_emit_values_end(wb);

            streaming_topology_v1_emit_values_start(wb);
            for(size_t i = 0; i < payload->inbound_used; i++)
                buffer_json_add_array_item_uint64(wb, payload->inbound_rows[i].child_actor);
            streaming_topology_v1_emit_values_end(wb);

            streaming_topology_v1_emit_values_start(wb);
            for(size_t i = 0; i < payload->inbound_used; i++)
                streaming_topology_v1_add_nullable_uint(wb,
                    payload->inbound_rows[i].has_source_actor, payload->inbound_rows[i].source_actor);
            streaming_topology_v1_emit_values_end(wb);

#define STREAMING_TOPOLOGY_INBOUND_STRING_VALUES(member) do { \
                streaming_topology_v1_emit_values_start(wb); \
                for(size_t i = 0; i < payload->inbound_used; i++) \
                    buffer_json_add_array_item_string(wb, payload->inbound_rows[i].member[0] ? payload->inbound_rows[i].member : NULL); \
                streaming_topology_v1_emit_values_end(wb); \
            } while(0)
#define STREAMING_TOPOLOGY_INBOUND_UINT_VALUES(member) do { \
                streaming_topology_v1_emit_values_start(wb); \
                for(size_t i = 0; i < payload->inbound_used; i++) \
                    buffer_json_add_array_item_uint64(wb, payload->inbound_rows[i].member); \
                streaming_topology_v1_emit_values_end(wb); \
            } while(0)

            STREAMING_TOPOLOGY_INBOUND_STRING_VALUES(received_type);
            STREAMING_TOPOLOGY_INBOUND_STRING_VALUES(ingest_status);

            streaming_topology_v1_emit_values_start(wb);
            for(size_t i = 0; i < payload->inbound_used; i++)
                buffer_json_add_array_item_int64(wb, payload->inbound_rows[i].hops);
            streaming_topology_v1_emit_values_end(wb);

            STREAMING_TOPOLOGY_INBOUND_UINT_VALUES(collected_metrics);
            STREAMING_TOPOLOGY_INBOUND_UINT_VALUES(collected_instances);
            STREAMING_TOPOLOGY_INBOUND_UINT_VALUES(collected_contexts);

            streaming_topology_v1_emit_values_start(wb);
            for(size_t i = 0; i < payload->inbound_used; i++)
                buffer_json_add_array_item_double(wb, payload->inbound_rows[i].replication_completion);
            streaming_topology_v1_emit_values_end(wb);

            STREAMING_TOPOLOGY_INBOUND_UINT_VALUES(ingest_age);
            STREAMING_TOPOLOGY_INBOUND_STRING_VALUES(ssl);
            STREAMING_TOPOLOGY_INBOUND_UINT_VALUES(alerts_critical);
            STREAMING_TOPOLOGY_INBOUND_UINT_VALUES(alerts_warning);
#undef STREAMING_TOPOLOGY_INBOUND_STRING_VALUES
#undef STREAMING_TOPOLOGY_INBOUND_UINT_VALUES
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb);
}

static void streaming_topology_v1_emit_outbound_table(BUFFER *wb, STREAMING_TOPOLOGY_V1_PAYLOAD *payload) {
    buffer_json_member_add_object(wb, "outbound");
    {
        buffer_json_member_add_string(wb, "type", "outbound");
        buffer_json_member_add_object(wb, "table");
        {
            buffer_json_member_add_uint64(wb, "rows", payload->outbound_used);
            buffer_json_member_add_array(wb, "columns");
            streaming_topology_v1_emit_outbound_columns(wb);
            buffer_json_array_close(wb);
            buffer_json_member_add_array(wb, "values");

            streaming_topology_v1_emit_values_start(wb);
            for(size_t i = 0; i < payload->outbound_used; i++)
                buffer_json_add_array_item_uint64(wb, payload->outbound_rows[i].actor);
            streaming_topology_v1_emit_values_end(wb);

            streaming_topology_v1_emit_values_start(wb);
            for(size_t i = 0; i < payload->outbound_used; i++)
                streaming_topology_v1_add_nullable_uint(wb,
                    payload->outbound_rows[i].has_destination_actor, payload->outbound_rows[i].destination_actor);
            streaming_topology_v1_emit_values_end(wb);

            streaming_topology_v1_emit_values_start(wb);
            for(size_t i = 0; i < payload->outbound_used; i++)
                buffer_json_add_array_item_string(wb, payload->outbound_rows[i].stream_status[0] ? payload->outbound_rows[i].stream_status : NULL);
            streaming_topology_v1_emit_values_end(wb);

            streaming_topology_v1_emit_values_start(wb);
            for(size_t i = 0; i < payload->outbound_used; i++)
                buffer_json_add_array_item_int64(wb, payload->outbound_rows[i].hops);
            streaming_topology_v1_emit_values_end(wb);

            streaming_topology_v1_emit_values_start(wb);
            for(size_t i = 0; i < payload->outbound_used; i++)
                buffer_json_add_array_item_string(wb, payload->outbound_rows[i].ssl[0] ? payload->outbound_rows[i].ssl : NULL);
            streaming_topology_v1_emit_values_end(wb);

            streaming_topology_v1_emit_values_start(wb);
            for(size_t i = 0; i < payload->outbound_used; i++)
                buffer_json_add_array_item_string(wb, payload->outbound_rows[i].compression[0] ? payload->outbound_rows[i].compression : NULL);
            streaming_topology_v1_emit_values_end(wb);

            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb);
}

static void streaming_topology_v1_emit_detail_tables(BUFFER *wb, STREAMING_TOPOLOGY_V1_PAYLOAD *payload) {
    buffer_json_member_add_object(wb, "tables");
    {
        buffer_json_member_add_object(wb, "actor");
        {
            streaming_topology_v1_emit_actor_labels_table(wb, payload);
            streaming_topology_v1_emit_stream_path_table(wb, payload);
            streaming_topology_v1_emit_retention_table(wb, payload);
            streaming_topology_v1_emit_inbound_table(wb, payload);
            streaming_topology_v1_emit_outbound_table(wb, payload);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb);
}

int function_streaming_topology(BUFFER *wb, const char *function, BUFFER *payload __maybe_unused, const char *source __maybe_unused) {
    time_t now = now_realtime_sec();
    usec_t now_ut = now_realtime_usec();

    struct streaming_topology_options options = { 0 };
    streaming_topology_parse_options(function, &options);
    char *function_copy = options.function_copy;

    if(options.info_only) {
        buffer_flush(wb);
        wb->content_type = CT_APPLICATION_JSON;
        buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);
        streaming_topology_v1_emit_response_metadata(wb);
        buffer_json_member_add_time_t(wb, "expires", now_realtime_sec() + STREAMING_FUNCTION_UPDATE_EVERY);
        buffer_json_finalize(wb);
        freez(function_copy);
        return HTTP_RESP_OK;
    }

    DICTIONARY *parent_child_count = dictionary_create_advanced(
        DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
        NULL, sizeof(uint32_t));
    DICTIONARY *parent_descendants = dictionary_create_advanced(
        DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
        NULL, sizeof(struct streaming_topology_descendant_list));

    STREAMING_TOPOLOGY_V1_PAYLOAD topology = {
        .actor_index = dictionary_create_advanced(
            DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
            NULL, sizeof(uint64_t)),
        .emitted_links = dictionary_create_advanced(
            DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
            NULL, sizeof(uint8_t)),
    };

    if(!parent_child_count || !parent_descendants || !topology.actor_index || !topology.emitted_links) {
        streaming_topology_v1_free(&topology);
        if(parent_descendants)
            dictionary_destroy(parent_descendants);
        if(parent_child_count)
            dictionary_destroy(parent_child_count);

        return streaming_topology_return_error(wb, function_copy,
            HTTP_RESP_INTERNAL_SERVER_ERROR,
            "failed to allocate streaming topology dictionaries");
    }

    {
        RRDHOST *host;
        dfe_start_read(rrdhost_root_index, host) {
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

            for(uint16_t i = 0; i < full_path_n; i++) {
                if(UUIDeq(full_path_ids[i], localhost->host_id))
                    continue;

                bool source_local = (i == 0);
                ND_UUID source_uuid = source_local ? empty_uuid : full_path_ids[i - 1];
                streaming_topology_descendants_append(parent_descendants,
                    full_path_ids[i], host, STREAMING_TOPOLOGY_RECEIVED_STREAMING, source_local, source_uuid);
            }
        }
        dfe_done(host);
    }

    {
        char localhost_guid[UUID_STR_LEN];
        if(streaming_topology_uuid_guid(localhost->host_id, localhost_guid, sizeof(localhost_guid))) {
            uint32_t live_count = 0;
            ND_UUID empty_uuid_for_live = {};
            RRDHOST *host;
            dfe_start_read(rrdhost_root_index, host) {
                if(host == localhost)
                    continue;

                if(rrdhost_is_virtual(host)) {
                    streaming_topology_descendants_append(parent_descendants,
                        localhost->host_id, host,
                        STREAMING_TOPOLOGY_RECEIVED_VIRTUAL, true, empty_uuid_for_live);
                    continue;
                }

                RRDHOST_STATUS status;
                rrdhost_status(host, now, &status, RRDHOST_STATUS_ALL);

                if(status.ingest.type == RRDHOST_INGEST_TYPE_CHILD &&
                   (status.ingest.status == RRDHOST_INGEST_STATUS_ONLINE ||
                    status.ingest.status == RRDHOST_INGEST_STATUS_REPLICATING)) {
                    live_count++;
                    streaming_topology_descendants_append(parent_descendants,
                        localhost->host_id, host,
                        STREAMING_TOPOLOGY_RECEIVED_STREAMING, false, empty_uuid_for_live);
                }
                else {
                    streaming_topology_descendants_append(parent_descendants,
                        localhost->host_id, host,
                        STREAMING_TOPOLOGY_RECEIVED_STALE, false, empty_uuid_for_live);
                }
            }
            dfe_done(host);

            uint32_t *existing = dictionary_get(parent_child_count, localhost_guid);
            if(existing)
                *existing = live_count;
            else if(live_count > 0)
                dictionary_set(parent_child_count, localhost_guid, &live_count, sizeof(live_count));
        }
    }

    streaming_topology_v1_collect_actors(&topology, parent_child_count, now);
    streaming_topology_v1_collect_links(&topology, now, now_ut);
    streaming_topology_v1_collect_actor_detail_rows(&topology, parent_descendants, now);

    buffer_flush(wb);
    wb->content_type = CT_APPLICATION_JSON;
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);

    streaming_topology_v1_emit_response_metadata(wb);

    buffer_json_member_add_object(wb, "data");
    {
        buffer_json_member_add_string(wb, "schema_version", "netdata.topology.v1");

        buffer_json_member_add_object(wb, "producer");
        {
            char localhost_agent_id[256];
            char localhost_guid[UUID_STR_LEN];
            streaming_topology_agent_id_for_host(localhost, localhost_agent_id, sizeof(localhost_agent_id));
            streaming_topology_host_guid(localhost, localhost_guid, sizeof(localhost_guid));

            buffer_json_member_add_string(wb, "source", "streaming");
            buffer_json_member_add_string(wb, "instance", localhost_agent_id);
            if(!UUIDiszero(localhost->node_id))
                buffer_json_member_add_uuid(wb, "node_id", localhost->node_id.uuid);
            if(localhost_guid[0])
                buffer_json_member_add_string(wb, "machine_guid", localhost_guid);
            buffer_json_member_add_string(wb, "agent_version", rrdhost_program_version(localhost));
            buffer_json_member_add_string(wb, "plugin", "netdata");
            buffer_json_member_add_array(wb, "capabilities");
            buffer_json_add_array_item_string(wb, "topology-v1");
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_datetime_rfc3339(wb, "collected_at", now_ut, true);
        buffer_json_member_add_object(wb, "view");
        {
            buffer_json_member_add_string(wb, "id", "streaming");
            buffer_json_member_add_string(wb, "scope", "node");
            buffer_json_member_add_string(wb, "mode", "detailed");
            buffer_json_member_add_array(wb, "group_by");
            buffer_json_add_array_item_string(wb, "node");
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "dictionaries");
        {
            buffer_json_member_add_array(wb, "strings");
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        streaming_topology_v1_emit_type_registry(wb);
        streaming_topology_v1_emit_presentation(wb);
        streaming_topology_v1_emit_actor_table(wb, &topology);
        streaming_topology_v1_emit_link_table(wb, &topology);
        streaming_topology_v1_emit_evidence_table(wb, &topology);
        streaming_topology_v1_emit_detail_tables(wb, &topology);

        buffer_json_member_add_object(wb, "stats");
        {
            buffer_json_member_add_uint64(wb, "actors", topology.actors_used);
            buffer_json_member_add_uint64(wb, "links", topology.links_used);
            buffer_json_member_add_uint64(wb, "evidence_rows", topology.links_used);
            buffer_json_member_add_uint64(wb, "stream_path_rows", topology.stream_path_used);
            buffer_json_member_add_uint64(wb, "retention_rows", topology.retention_used);
            buffer_json_member_add_uint64(wb, "inbound_rows", topology.inbound_used);
            buffer_json_member_add_uint64(wb, "outbound_rows", topology.outbound_used);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb);

    buffer_json_member_add_time_t(wb, "expires", now_realtime_sec() + STREAMING_FUNCTION_UPDATE_EVERY);
    buffer_json_finalize(wb);

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
    streaming_topology_v1_free(&topology);
    freez(function_copy);
    return HTTP_RESP_OK;
}
