// SPDX-License-Identifier: GPL-3.0-or-later

#include "stream_path.h"
#include "rrdpush.h"
#include "collectors/plugins.d/pluginsd_internals.h"

ENUM_STR_MAP_DEFINE(STREAM_PATH_FLAGS) = {
    { .id = STREAM_PATH_FLAG_ACLK, .name = "aclk" },

    // terminator
    { . id = 0, .name = NULL }
};

BITMAP_STR_DEFINE_FUNCTIONS(STREAM_PATH_FLAGS, STREAM_PATH_FLAG_NONE, "");

static void stream_path_clear(STREAM_PATH *p) {
    string_freez(p->hostname);
    p->hostname = NULL;
    p->host_id = UUID_ZERO;
    p->node_id = UUID_ZERO;
    p->hops = 0;
    p->since = 0;
    p->first_time_t = 0;
    p->last_time_t = 0;
    p->capabilities = 0;
    p->flags = STREAM_PATH_FLAG_NONE;
}

void rrdhost_stream_path_clear(RRDHOST *host, bool destroy) {
    spinlock_lock(&host->rrdpush.path.spinlock);

    for(size_t i = 0; i < host->rrdpush.path.used ; i++)
        stream_path_clear(&host->rrdpush.path.array[i]);

    host->rrdpush.path.used = 0;

    if(destroy) {
        freez(host->rrdpush.path.array);
        host->rrdpush.path.array = NULL;
        host->rrdpush.path.size = 0;
    }

    spinlock_unlock(&host->rrdpush.path.spinlock);
}

static void stream_path_to_json_object(BUFFER *wb, STREAM_PATH *p) {
    buffer_json_member_add_string(wb, "hostname", string2str(p->hostname));
    buffer_json_member_add_uuid(wb, "host_id", p->host_id.uuid);
    buffer_json_member_add_uuid(wb, "node_id", p->node_id.uuid);
    buffer_json_member_add_uuid(wb, "claim_id", p->claim_id.uuid);
    buffer_json_member_add_int64(wb, "hops", p->hops);
    buffer_json_member_add_uint64(wb, "since", p->since);
    buffer_json_member_add_uint64(wb, "first_time_t", p->first_time_t);
    buffer_json_member_add_uint64(wb, "last_time_t", p->last_time_t);
    stream_capabilities_to_json_array(wb, p->capabilities, "capabilities");
    STREAM_PATH_FLAGS_2json(wb, "flags", p->flags);
}

static STREAM_PATH rrdhost_stream_path_self(RRDHOST *host) {
    STREAM_PATH p = { 0 };

    bool is_localhost = host == localhost || rrdhost_option_check(host, RRDHOST_OPTION_VIRTUAL_HOST);

    p.hostname = string_dup(localhost->hostname);
    p.host_id = localhost->host_id;
    p.node_id = localhost->node_id;
    p.claim_id = claim_id_get_uuid();

    p.flags = STREAM_PATH_FLAG_NONE;
    if(!UUIDiszero(p.claim_id))
        p.flags |= STREAM_PATH_FLAG_ACLK;

    bool has_receiver = false;
    spinlock_lock(&host->receiver_lock);
    if(host->receiver) {
        has_receiver = true;
        p.hops = (int16_t)host->receiver->hops;
        p.since = host->receiver->connected_since_s;
        p.capabilities = host->receiver->capabilities;
    }
    spinlock_unlock(&host->receiver_lock);

    if(!has_receiver) {
        p.hops = (is_localhost) ? 0 : -1; // -1 for stale nodes
        p.since = netdata_start_time;
        // the following may get the receiver lock again!
        p.capabilities = stream_our_capabilities(host, host == localhost);
    }

    rrdhost_retention(host, now_realtime_sec(),
                      is_localhost || has_receiver,
                      &p.first_time_t, &p.last_time_t);

    return p;
}

void rrdhost_stream_path_to_json(BUFFER *wb, struct rrdhost *host, const char *key, bool add_version) {
    spinlock_lock(&host->rrdpush.path.spinlock);
    buffer_json_member_add_array(wb, key);
    {
        if(add_version)
            buffer_json_member_add_uint64(wb, "version", 1);

        buffer_json_add_array_item_object(wb);
        {
            STREAM_PATH tmp = rrdhost_stream_path_self(host);

            bool found_self = false;
            for (size_t i = 0; i < host->rrdpush.path.used; i++) {
                STREAM_PATH *p = &host->rrdpush.path.array[i];
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

            stream_path_clear(&tmp);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_array_close(wb); // key
    spinlock_unlock(&host->rrdpush.path.spinlock);
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
    if(!s || !stream_has_capability(s, STREAM_CAP_PATHS))
        return;

    CLEAN_BUFFER *payload = stream_path_payload(s->host);

    BUFFER *wb = sender_start(s);
    buffer_sprintf(wb, PLUGINSD_KEYWORD_JSON " " PLUGINSD_KEYWORD_STREAM_PATH "\n%s\n" PLUGINSD_KEYWORD_JSON_END "\n", buffer_tostring(payload));
    sender_commit(s, wb, STREAM_TRAFFIC_TYPE_METADATA);
}

void stream_path_send_to_child(RRDHOST *host) {
    if(host == localhost)
        return;

    CLEAN_BUFFER *payload = stream_path_payload(host);

    spinlock_lock(&host->receiver_lock);
    if(host->receiver && stream_has_capability(host->receiver, STREAM_CAP_PATHS)) {

        CLEAN_BUFFER *wb = buffer_create(0, NULL);
        buffer_sprintf(wb, PLUGINSD_KEYWORD_JSON " " PLUGINSD_KEYWORD_STREAM_PATH "\n%s\n" PLUGINSD_KEYWORD_JSON_END "\n", buffer_tostring(payload));
        send_to_plugin(buffer_tostring(wb), __atomic_load_n(&host->receiver->parser, __ATOMIC_RELAXED));
    }
    spinlock_unlock(&host->receiver_lock);
}

void stream_path_parent_disconnected(RRDHOST *host) {
    spinlock_lock(&host->rrdpush.path.spinlock);

    size_t used = host->rrdpush.path.used;
    for (size_t i = 0; i < used; i++) {
        STREAM_PATH *p = &host->rrdpush.path.array[i];
        if(UUIDeq(localhost->host_id, p->host_id)) {
            host->rrdpush.path.used = i + 1;

            for(size_t j = i + 1; j < used ;j++)
                stream_path_clear(&host->rrdpush.path.array[j]);

            break;
        }
    }

    spinlock_unlock(&host->rrdpush.path.spinlock);
    stream_path_send_to_child(host);
}

// --------------------------------------------------------------------------------------------------------------------


static bool parse_single_path(json_object *jobj, const char *path, STREAM_PATH *p, BUFFER *error) {
    JSONC_PARSE_TXT2STRING_OR_ERROR_AND_RETURN(jobj, path, "hostname", p->hostname, error, true);
    JSONC_PARSE_TXT2UUID_OR_ERROR_AND_RETURN(jobj, path, "host_id", p->host_id.uuid, error, true);
    JSONC_PARSE_TXT2UUID_OR_ERROR_AND_RETURN(jobj, path, "node_id", p->node_id.uuid, error, true);
    JSONC_PARSE_TXT2UUID_OR_ERROR_AND_RETURN(jobj, path, "claim_id", p->claim_id.uuid, error, true);
    JSONC_PARSE_INT64_OR_ERROR_AND_RETURN(jobj, path, "hops", p->hops, error, true);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "since", p->since, error, true);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "first_time_t", p->first_time_t, error, true);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "last_time_t", p->last_time_t, error, true);
    JSONC_PARSE_ARRAY_OF_TXT2BITMAP_OR_ERROR_AND_RETURN(jobj, path, "flags", STREAM_PATH_FLAGS_2id_one, p->flags, error, true);
    JSONC_PARSE_ARRAY_OF_TXT2BITMAP_OR_ERROR_AND_RETURN(jobj, path, "capabilities", stream_capabilities_parse_one, p->capabilities, error, true);
    return true;
}

static XXH128_hash_t stream_path_hash_unsafe(RRDHOST *host) {
    if(!host->rrdpush.path.used)
        return (XXH128_hash_t){ 0 };

    return XXH3_128bits(host->rrdpush.path.array, sizeof(*host->rrdpush.path.array) * host->rrdpush.path.used);
}

bool stream_path_set_from_json(RRDHOST *host, const char *json) {
    if(!json || !*json)
        return false;

    CLEAN_JSON_OBJECT *jobj = json_tokener_parse(json);
    if(!jobj) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "STREAM PATH: Cannot parse json: %s", json);
        return false;
    }

    spinlock_lock(&host->rrdpush.path.spinlock);
    XXH128_hash_t old_hash = stream_path_hash_unsafe(host);
    rrdhost_stream_path_clear(host, true);

    CLEAN_BUFFER *error = buffer_create(0, NULL);

    json_object *_jarray;
    if (json_object_object_get_ex(jobj, STREAM_PATH_JSON_MEMBER, &_jarray) &&
        json_object_is_type(_jarray, json_type_array)) {
        size_t items = json_object_array_length(_jarray);
        host->rrdpush.path.array = callocz(items, sizeof(*host->rrdpush.path.array));
        host->rrdpush.path.size = items;

        for (size_t i = 0; i < items; ++i) {
            json_object *joption = json_object_array_get_idx(_jarray, i);
            if (!json_object_is_type(joption, json_type_object)) {
                nd_log(NDLS_DAEMON, NDLP_ERR,
                       "STREAM PATH: Array item No %zu is not an object: %s", i, json);
                continue;
            }

            if(!parse_single_path(joption, "", &host->rrdpush.path.array[host->rrdpush.path.used], error)) {
                stream_path_clear(&host->rrdpush.path.array[host->rrdpush.path.used]);
                nd_log(NDLS_DAEMON, NDLP_ERR,
                       "STREAM PATH: Array item No %zu cannot be parsed: %s: %s", i, buffer_tostring(error), json);
            }
            else
                host->rrdpush.path.used++;
        }
    }

    XXH128_hash_t new_hash = stream_path_hash_unsafe(host);
    spinlock_unlock(&host->rrdpush.path.spinlock);

    if(!XXH128_isEqual(old_hash, new_hash)) {
        stream_path_send_to_parent(host);
        stream_path_send_to_child(host);
    }

    return host->rrdpush.path.used > 0;
}