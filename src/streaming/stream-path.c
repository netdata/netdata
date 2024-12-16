// SPDX-License-Identifier: GPL-3.0-or-later

#include "stream-path.h"
#include "stream.h"
#include "stream-receiver-internals.h"
#include "stream-sender-internals.h"
#include "plugins.d/pluginsd_internals.h"

typedef enum __attribute__((packed)) {
    STREAM_PATH_FLAG_NONE       = 0,
    STREAM_PATH_FLAG_ACLK       = (1 << 0),
    STREAM_PATH_FLAG_HEALTH     = (1 << 1),
    STREAM_PATH_FLAG_ML         = (1 << 2),
    STREAM_PATH_FLAG_EPHEMERAL  = (1 << 3),
    STREAM_PATH_FLAG_VIRTUAL    = (1 << 4),
} STREAM_PATH_FLAGS;

typedef struct stream_path {
    STRING *hostname;               // the hostname of the agent
    ND_UUID host_id;                // the machine guid of the agent
    ND_UUID node_id;                // the cloud node id of the agent
    ND_UUID claim_id;               // the cloud claim id of the agent
    time_t since;                   // the timestamp of the last update
    time_t first_time_t;            // the oldest timestamp in the db
    int16_t hops;                   // -1 = stale node, 0 = localhost, >0 the hops count
    STREAM_PATH_FLAGS flags;        // ACLK or NONE for the moment
    STREAM_CAPABILITIES capabilities; // streaming connection capabilities
    uint32_t start_time_ms;         // median time in ms the agent needs to start
    uint32_t shutdown_time_ms;      // median time in ms the agent needs to shutdown
} STREAM_PATH;

ENUM_STR_MAP_DEFINE(STREAM_PATH_FLAGS) = {
    { .id = STREAM_PATH_FLAG_ACLK,      .name = "aclk" },
    { .id = STREAM_PATH_FLAG_HEALTH,    .name = "health" },
    { .id = STREAM_PATH_FLAG_ML,        .name = "ml" },
    { .id = STREAM_PATH_FLAG_EPHEMERAL, .name = "ephemeral" },
    { .id = STREAM_PATH_FLAG_VIRTUAL,   .name = "virtual" },

    // terminator
    { . id = 0, .name = NULL }
};

BITMAP_STR_DEFINE_FUNCTIONS(STREAM_PATH_FLAGS, STREAM_PATH_FLAG_NONE, "");

static void stream_path_cleanup(STREAM_PATH *p) {
    string_freez(p->hostname);
    p->hostname = NULL;
    p->host_id = UUID_ZERO;
    p->node_id = UUID_ZERO;
    p->claim_id = UUID_ZERO;
    p->hops = 0;
    p->since = 0;
    p->first_time_t = 0;
    p->capabilities = 0;
    p->flags = STREAM_PATH_FLAG_NONE;
    p->start_time_ms = 0;
    p->shutdown_time_ms = 0;
}

static void rrdhost_stream_path_clear_unsafe(RRDHOST *host, bool destroy) {
    for(size_t i = 0; i < host->stream.path.used ; i++)
        stream_path_cleanup(&host->stream.path.array[i]);

    host->stream.path.used = 0;

    if(destroy) {
        freez(host->stream.path.array);
        host->stream.path.array = NULL;
        host->stream.path.size = 0;
    }
}

void rrdhost_stream_path_clear(RRDHOST *host, bool destroy) {
    rw_spinlock_write_lock(&host->stream.path.spinlock);
    rrdhost_stream_path_clear_unsafe(host, destroy);
    rw_spinlock_write_unlock(&host->stream.path.spinlock);
}

static void stream_path_to_json_object(BUFFER *wb, STREAM_PATH *p) {
    buffer_json_add_array_item_object(wb);
    {
        buffer_json_member_add_uint64(wb, "version", 1);
        buffer_json_member_add_string(wb, "hostname", string2str(p->hostname));
        buffer_json_member_add_uuid(wb, "host_id", p->host_id.uuid);
        buffer_json_member_add_uuid(wb, "node_id", p->node_id.uuid);
        buffer_json_member_add_uuid(wb, "claim_id", p->claim_id.uuid);
        buffer_json_member_add_int64(wb, "hops", p->hops);
        buffer_json_member_add_uint64(wb, "since", p->since);
        buffer_json_member_add_uint64(wb, "first_time_t", p->first_time_t);
        buffer_json_member_add_uint64(wb, "start_time", p->start_time_ms);
        buffer_json_member_add_uint64(wb, "shutdown_time", p->shutdown_time_ms);
        stream_capabilities_to_json_array(wb, p->capabilities, "capabilities");
        STREAM_PATH_FLAGS_2json(wb, "flags", p->flags);
    }
    buffer_json_object_close(wb);
}

static STREAM_PATH rrdhost_stream_path_self(RRDHOST *host) {
    STREAM_PATH p = { 0 };

    bool is_localhost = host == localhost || rrdhost_option_check(host, RRDHOST_OPTION_VIRTUAL_HOST);

    p.hostname = string_dup(localhost->hostname);
    p.host_id = localhost->host_id;
    p.node_id = localhost->node_id;
    p.claim_id = claim_id_get_uuid();
    p.start_time_ms = get_agent_event_time_median(EVENT_AGENT_START_TIME) / USEC_PER_MS;
    p.shutdown_time_ms = get_agent_event_time_median(EVENT_AGENT_SHUTDOWN_TIME) / USEC_PER_MS;

    p.flags = STREAM_PATH_FLAG_NONE;
    if(!UUIDiszero(p.claim_id))
        p.flags |= STREAM_PATH_FLAG_ACLK;

    if(rrdhost_option_check(host, RRDHOST_OPTION_EPHEMERAL_HOST))
        p.flags |= STREAM_PATH_FLAG_EPHEMERAL;

    if(rrdhost_option_check(host, RRDHOST_OPTION_VIRTUAL_HOST))
        p.flags |= STREAM_PATH_FLAG_VIRTUAL;

    if(host->health.enabled)
        p.flags |= STREAM_PATH_FLAG_HEALTH;

    if(ml_enabled(host))
        p.flags |= STREAM_PATH_FLAG_ML;

    rrdhost_receiver_lock(host);
    if(host->receiver) {
        p.hops = (int16_t)host->receiver->hops;
        p.since = host->receiver->connected_since_s;
    }
    else {
        p.hops = (is_localhost) ? 0 : -1; // -1 for stale nodes
        p.since = netdata_start_time;
    }
    rrdhost_receiver_unlock(host);

    // the following may get the receiver lock again!
    p.capabilities = stream_our_capabilities(host, true);

    rrdhost_retention(host, 0, false, &p.first_time_t, NULL);

    return p;
}

uint64_t rrdhost_stream_path_total_reboot_time_ms(RRDHOST *host) {
    uint64_t total_ms = 0;

    rw_spinlock_read_lock(&host->stream.path.spinlock);
    for (size_t i = 0; i < host->stream.path.used; i++) {
        STREAM_PATH *tmp_path = &host->stream.path.array[i];
        if(UUIDeq(host->host_id, tmp_path->host_id)) {
            total_ms = tmp_path->start_time_ms + tmp_path->shutdown_time_ms;
            break;
        }
    }
    rw_spinlock_read_unlock(&host->stream.path.spinlock);
    return total_ms;
}

bool rrdhost_is_host_in_stream_path_before_us(struct rrdhost *host, ND_UUID remote_agent_host_id, int16_t our_hops) {
    if(UUIDiszero(remote_agent_host_id)) return false;
    if(UUIDeq(localhost->host_id, remote_agent_host_id)) return true;

    bool rc = false;
    rw_spinlock_read_lock(&host->stream.path.spinlock);
    for (size_t i = 0; i < host->stream.path.used; i++) {
        STREAM_PATH *p = &host->stream.path.array[i];
        if(UUIDeq(remote_agent_host_id, p->host_id) && p->hops < our_hops) {
            rc = true;
            break;
        }
    }
    rw_spinlock_read_unlock(&host->stream.path.spinlock);
    return rc;
}

void rrdhost_stream_path_to_json(BUFFER *wb, struct rrdhost *host, const char *key, bool add_version) {
    if(add_version)
        buffer_json_member_add_uint64(wb, "version", 1);

    STREAM_PATH tmp = rrdhost_stream_path_self(host);

    rw_spinlock_read_lock(&host->stream.path.spinlock);
    buffer_json_member_add_array(wb, key);
    {
        {
            bool found_self = false;
            for (size_t i = 0; i < host->stream.path.used; i++) {
                STREAM_PATH *p = &host->stream.path.array[i];
                if(UUIDeq(localhost->host_id, p->host_id)) {
                    // this is us, use the current data
                    p = &tmp;
                    found_self = true;
                }
                stream_path_to_json_object(wb, p);
            }

            if(!found_self) {
                // we didn't find ourselves in the list.
                // append us.
                stream_path_to_json_object(wb, &tmp);
            }
        }
    }
    buffer_json_array_close(wb); // key
    rw_spinlock_read_unlock(&host->stream.path.spinlock);

    stream_path_cleanup(&tmp);
}

static BUFFER *stream_path_payload(RRDHOST *host) {
    BUFFER *wb = buffer_create(0, NULL);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);
    rrdhost_stream_path_to_json(wb, host, STREAM_PATH_JSON_MEMBER, true);
    buffer_json_finalize(wb);
    return wb;
}

void stream_path_send_to_parent(RRDHOST *host) {
    struct sender_state *s = host->sender;
    if(!s ||
        !stream_has_capability(s, STREAM_CAP_PATHS) ||
        !rrdhost_can_stream_metadata_to_parent(host))
        return;

    CLEAN_BUFFER *payload = stream_path_payload(host);
    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    buffer_sprintf(wb, PLUGINSD_KEYWORD_JSON " " PLUGINSD_KEYWORD_JSON_CMD_STREAM_PATH "\n%s\n" PLUGINSD_KEYWORD_JSON_END "\n", buffer_tostring(payload));
    sender_commit_clean_buffer(s, wb, STREAM_TRAFFIC_TYPE_METADATA);
}

void stream_path_send_to_child(RRDHOST *host) {
    if(host == localhost)
        return;

    CLEAN_BUFFER *payload = stream_path_payload(host);

    rrdhost_receiver_lock(host);
    if(stream_has_capability(host->receiver, STREAM_CAP_PATHS) &&
        rrdhost_flag_check(host, RRDHOST_FLAG_COLLECTOR_ONLINE)) {

        CLEAN_BUFFER *wb = buffer_create(0, NULL);
        buffer_sprintf(wb, PLUGINSD_KEYWORD_JSON " " PLUGINSD_KEYWORD_JSON_CMD_STREAM_PATH "\n%s\n" PLUGINSD_KEYWORD_JSON_END "\n", buffer_tostring(payload));
        send_to_plugin(buffer_tostring(wb), __atomic_load_n(&host->receiver->thread.parser, __ATOMIC_RELAXED), STREAM_TRAFFIC_TYPE_METADATA);
    }
    rrdhost_receiver_unlock(host);
}

void stream_path_child_disconnected(RRDHOST *host) {
    rrdhost_stream_path_clear(host, true);
}

void stream_path_parent_disconnected(RRDHOST *host) {
    rw_spinlock_write_lock(&host->stream.path.spinlock);

    size_t cleared = 0;
    size_t used = host->stream.path.used;
    for (size_t i = 0; i < used; i++) {
        STREAM_PATH *p = &host->stream.path.array[i];
        if(UUIDeq(localhost->host_id, p->host_id)) {
            host->stream.path.used = i + 1;

            for(size_t j = i + 1; j < used ;j++) {
                stream_path_cleanup(&host->stream.path.array[j]);
                cleared++;
            }

            break;
        }
    }

    rw_spinlock_write_unlock(&host->stream.path.spinlock);

    if(cleared)
        stream_path_send_to_child(host);
}

void stream_path_retention_updated(RRDHOST *host) {
    if(!host || !localhost) return;
    stream_path_send_to_parent(host);
    stream_path_send_to_child(host);
}

void stream_path_node_id_updated(RRDHOST *host) {
    if(!host || !localhost) return;
    stream_path_send_to_parent(host);
    stream_path_send_to_child(host);
}

// --------------------------------------------------------------------------------------------------------------------


static bool parse_single_path(json_object *jobj, const char *path, STREAM_PATH *p, BUFFER *error) {
    uint32_t version = 0;
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "version", version, error, false);

    JSONC_PARSE_TXT2STRING_OR_ERROR_AND_RETURN(jobj, path, "hostname", p->hostname, error, true);
    JSONC_PARSE_TXT2UUID_OR_ERROR_AND_RETURN(jobj, path, "host_id", p->host_id.uuid, error, true);
    JSONC_PARSE_TXT2UUID_OR_ERROR_AND_RETURN(jobj, path, "node_id", p->node_id.uuid, error, true);
    JSONC_PARSE_TXT2UUID_OR_ERROR_AND_RETURN(jobj, path, "claim_id", p->claim_id.uuid, error, true);
    JSONC_PARSE_INT64_OR_ERROR_AND_RETURN(jobj, path, "hops", p->hops, error, true);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "since", p->since, error, true);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "first_time_t", p->first_time_t, error, true);
    JSONC_PARSE_INT64_OR_ERROR_AND_RETURN(jobj, path, "start_time", p->start_time_ms, error, true);
    JSONC_PARSE_INT64_OR_ERROR_AND_RETURN(jobj, path, "shutdown_time", p->shutdown_time_ms, error, true);
    JSONC_PARSE_ARRAY_OF_TXT2BITMAP_OR_ERROR_AND_RETURN(jobj, path, "flags", STREAM_PATH_FLAGS_2id_one, p->flags, error, false);
    JSONC_PARSE_ARRAY_OF_TXT2BITMAP_OR_ERROR_AND_RETURN(jobj, path, "capabilities", stream_capabilities_parse_one, p->capabilities, error, false);

    if(!p->hostname) {
        buffer_strcat(error, "hostname cannot be empty");
        return false;
    }

    if(UUIDiszero(p->host_id)) {
        buffer_strcat(error, "host_id cannot be zero");
        return false;
    }

    if(p->hops < 0) {
        buffer_strcat(error, "hops cannot be negative (probably the child disconnected from the Netdata before us");
        return false;
    }

    if(p->capabilities == STREAM_CAP_NONE) {
        buffer_strcat(error, "capabilities cannot be empty");
        return false;
    }

    if(p->since <= 0) {
        buffer_strcat(error, "since cannot be <= 0");
        return false;
    }

    return true;
}

static XXH128_hash_t stream_path_hash_unsafe(RRDHOST *host) {
    if(!host->stream.path.used)
        return (XXH128_hash_t){ 0 };

    return XXH3_128bits(host->stream.path.array, sizeof(*host->stream.path.array) * host->stream.path.used);
}

static int compare_by_hops(const void *a, const void *b) {
    const STREAM_PATH *path1 = a;
    const STREAM_PATH *path2 = b;

    if (path1->hops < path2->hops)
        return -1;
    else if (path1->hops > path2->hops)
        return 1;

    return 0;
}

bool stream_path_set_from_json(RRDHOST *host, const char *json, bool from_parent) {
    if(!json || !*json)
        return false;

    CLEAN_JSON_OBJECT *jobj = json_tokener_parse(json);
    if(!jobj) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "STREAM PATH '%s': Cannot parse json: %s",
               rrdhost_hostname(host), json);
        return false;
    }

    rw_spinlock_write_lock(&host->stream.path.spinlock);
    XXH128_hash_t old_hash = stream_path_hash_unsafe(host);
    rrdhost_stream_path_clear_unsafe(host, true);

    CLEAN_BUFFER *error = buffer_create(0, NULL);

    json_object *_jarray;
    if (json_object_object_get_ex(jobj, STREAM_PATH_JSON_MEMBER, &_jarray) &&
        json_object_is_type(_jarray, json_type_array)) {
        size_t items = json_object_array_length(_jarray);
        host->stream.path.array = callocz(items, sizeof(*host->stream.path.array));
        host->stream.path.size = items;

        for (size_t i = 0; i < items; ++i) {
            json_object *joption = json_object_array_get_idx(_jarray, i);
            if (!json_object_is_type(joption, json_type_object)) {
                nd_log(NDLS_DAEMON, NDLP_ERR,
                       "STREAM PATH '%s': Array item No %zu is not an object: %s",
                       rrdhost_hostname(host), i, json);
                continue;
            }

            if(!parse_single_path(joption, "", &host->stream.path.array[host->stream.path.used], error)) {
                stream_path_cleanup(&host->stream.path.array[host->stream.path.used]);
                nd_log(NDLS_DAEMON, NDLP_ERR,
                       "STREAM PATH '%s': Array item No %zu cannot be parsed: %s: %s",
                       rrdhost_hostname(host), i, buffer_tostring(error), json);
            }
            else
                host->stream.path.used++;
        }
    }

    if(host->stream.path.used > 1) {
        // sorting is required in order to support stream_path_parent_disconnected()
        qsort(host->stream.path.array, host->stream.path.used,
              sizeof(*host->stream.path.array), compare_by_hops);
    }

    XXH128_hash_t new_hash = stream_path_hash_unsafe(host);
    rw_spinlock_write_unlock(&host->stream.path.spinlock);

    if(!XXH128_isEqual(old_hash, new_hash)) {
        if(!from_parent)
            stream_path_send_to_parent(host);

        // when it comes from the child, we still need to send it back to the child
        // including our own entry in it.
        stream_path_send_to_child(host);
    }

    return host->stream.path.used > 0;
}

void rrdhost_stream_path_init(RRDHOST *host) {
    rw_spinlock_init(&host->stream.path.spinlock);
}
