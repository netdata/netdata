// SPDX-License-Identifier: GPL-3.0-or-later

#include "stream.h"
#include "stream-receiver-internals.h"
#include "stream-sender-internals.h"

static STREAM_CAPABILITIES globally_disabled_capabilities = STREAM_CAP_ALWAYS_DISABLED;

static struct {
    STREAM_CAPABILITIES cap;
    const char *str;
} capability_names[] = {
    // DO NOT CHANGE NAMES
    // THEY ARE USED BY STREAM_PATH, SO CONNECTING OF DIFFERENT NODES WILL BREAK

    {STREAM_CAP_V1,           "V1" },
    {STREAM_CAP_V2,           "V2" },
    {STREAM_CAP_VN,           "VN" },
    {STREAM_CAP_VCAPS,        "VCAPS" },
    {STREAM_CAP_HLABELS,      "HLABELS" },
    {STREAM_CAP_CLAIM,        "CLAIM" },
    {STREAM_CAP_CLABELS,      "CLABELS" },
    {STREAM_CAP_LZ4,          "LZ4" },
    {STREAM_CAP_FUNCTIONS,    "FUNCTIONS" },
    {STREAM_CAP_REPLICATION,  "REPLICATION" },
    {STREAM_CAP_BINARY,       "BINARY" },
    {STREAM_CAP_INTERPOLATED, "INTERPOLATED" },
    {STREAM_CAP_IEEE754,      "IEEE754" },
    {STREAM_CAP_DATA_WITH_ML, "ML"},            // do not remove this - stream_path fails to parse old nodes
    {STREAM_CAP_ML_MODELS,    "MLMODELS" },
    {STREAM_CAP_DYNCFG,       "DYNCFG" },
    {STREAM_CAP_SLOTS,        "SLOTS" },
    {STREAM_CAP_ZSTD,         "ZSTD" },
    {STREAM_CAP_GZIP,         "GZIP" },
    {STREAM_CAP_BROTLI,       "BROTLI" },
    {STREAM_CAP_PROGRESS,     "PROGRESS" },
    {STREAM_CAP_NODE_ID,      "NODEID" },
    {STREAM_CAP_PATHS,        "PATHS" },

    // terminator
    {0 , NULL },
};

STREAM_CAPABILITIES stream_capabilities_parse_one(const char *str) {
    if (!str || !*str)
        return STREAM_CAP_NONE;

    for (size_t i = 0; capability_names[i].str; i++) {
        if (strcmp(capability_names[i].str, str) == 0)
            return capability_names[i].cap;
    }

    return STREAM_CAP_NONE;
}

void stream_capabilities_to_string(BUFFER *wb, STREAM_CAPABILITIES caps) {
    for(size_t i = 0; capability_names[i].str ; i++) {
        if(caps & capability_names[i].cap) {
            buffer_strcat(wb, capability_names[i].str);
            buffer_strcat(wb, " ");
        }
    }
}

void stream_capabilities_to_json_array(BUFFER *wb, STREAM_CAPABILITIES caps, const char *key) {
    if(key)
        buffer_json_member_add_array(wb, key);
    else
        buffer_json_add_array_item_array(wb);

    for(size_t i = 0; capability_names[i].str ; i++) {
        if(caps & capability_names[i].cap)
            buffer_json_add_array_item_string(wb, capability_names[i].str);
    }

    buffer_json_array_close(wb);
}

void log_receiver_capabilities(struct receiver_state *rpt) {
    BUFFER *wb = buffer_create(100, NULL);
    stream_capabilities_to_string(wb, rpt->capabilities);

    nd_log_daemon(NDLP_INFO, "STREAM RCV '%s' [from [%s]:%s]: established link with negotiated capabilities: %s",
                  rrdhost_hostname(rpt->host), rpt->remote_ip, rpt->remote_port, buffer_tostring(wb));

    buffer_free(wb);
}

void log_sender_capabilities(struct sender_state *s) {
    BUFFER *wb = buffer_create(100, NULL);
    stream_capabilities_to_string(wb, s->capabilities);

    nd_log_daemon(NDLP_INFO, "STREAM SND '%s' [to %s]: established link with negotiated capabilities: %s",
                  rrdhost_hostname(s->host), s->remote_ip, buffer_tostring(wb));

    buffer_free(wb);
}

STREAM_CAPABILITIES stream_our_capabilities(RRDHOST *host, bool sender) {
    STREAM_CAPABILITIES disabled_capabilities = globally_disabled_capabilities;

    if(host && sender) {
        // we have DATA_WITH_ML capability
        // we should remove the DATA_WITH_ML capability if our database does not have anomaly info
        // this can happen under these conditions: 1. we don't run ML, and 2. we don't receive ML
        rrdhost_receiver_lock(host);

        if (!ml_host_running(host) && !stream_has_capability(host->receiver, STREAM_CAP_ML_MODELS))
            disabled_capabilities |= STREAM_CAP_ML_MODELS;

        rrdhost_receiver_unlock(host);

        if(host->sender)
            disabled_capabilities |= host->sender->disabled_capabilities;
    }

//    if(sender) {
//        if(nd_profile.stream_sender_compression == ND_COMPRESSION_FASTEST)
//            // lz4 or nothing
//            disabled_capabilities |= (STREAM_CAP_COMPRESSIONS_AVAILABLE & ~(STREAM_CAP_LZ4_AVAILABLE));
//    }

    return (STREAM_CAP_V1 |
            STREAM_CAP_V2 |
            STREAM_CAP_VN |
            STREAM_CAP_VCAPS |
            STREAM_CAP_HLABELS |
            STREAM_CAP_CLAIM |
            STREAM_CAP_CLABELS |
            STREAM_CAP_FUNCTIONS |
            STREAM_CAP_REPLICATION |
            STREAM_CAP_BINARY |
            STREAM_CAP_INTERPOLATED |
            STREAM_CAP_SLOTS |
            STREAM_CAP_PROGRESS |
            STREAM_CAP_COMPRESSIONS_AVAILABLE |
            STREAM_CAP_DYNCFG |
            STREAM_CAP_NODE_ID |
            STREAM_CAP_PATHS |
            STREAM_CAP_IEEE754 |
            STREAM_CAP_ML_MODELS |
            0) & ~disabled_capabilities;
}

STREAM_CAPABILITIES convert_stream_version_to_capabilities(int32_t version, RRDHOST *host, bool sender) {
    STREAM_CAPABILITIES caps = 0;

    if(version <= 1) caps = STREAM_CAP_V1;
    else if(version < STREAM_OLD_VERSION_CLAIM) caps = STREAM_CAP_V2 | STREAM_CAP_HLABELS;
    else if(version <= STREAM_OLD_VERSION_CLAIM) caps = STREAM_CAP_VN | STREAM_CAP_HLABELS | STREAM_CAP_CLAIM;
    else if(version <= STREAM_OLD_VERSION_CLABELS) caps = STREAM_CAP_VN | STREAM_CAP_HLABELS | STREAM_CAP_CLAIM | STREAM_CAP_CLABELS;
    else if(version <= STREAM_OLD_VERSION_LZ4) caps = STREAM_CAP_VN | STREAM_CAP_HLABELS | STREAM_CAP_CLAIM | STREAM_CAP_CLABELS | STREAM_CAP_LZ4_AVAILABLE;
    else caps = version;

    if(caps & STREAM_CAP_VCAPS)
        caps &= ~(STREAM_CAP_V1|STREAM_CAP_V2|STREAM_CAP_VN);

    if(caps & STREAM_CAP_VN)
        caps &= ~(STREAM_CAP_V1|STREAM_CAP_V2);

    if(caps & STREAM_CAP_V2)
        caps &= ~(STREAM_CAP_V1);

    STREAM_CAPABILITIES common_caps = caps & stream_our_capabilities(host, sender);

    if(!(common_caps & STREAM_CAP_INTERPOLATED))
        // DATA WITH ML requires INTERPOLATED
        common_caps &= ~(STREAM_CAP_ML_MODELS);

    return common_caps;
}

int32_t stream_capabilities_to_vn(uint32_t caps) {
    if(caps & STREAM_CAP_LZ4) return STREAM_OLD_VERSION_LZ4;
    if(caps & STREAM_CAP_CLABELS) return STREAM_OLD_VERSION_CLABELS;
    return STREAM_OLD_VERSION_CLAIM; // if(caps & STREAM_CAP_CLAIM)
}

void check_local_streaming_capabilities(void) {
    ieee754_doubles = is_system_ieee754_double();
    if(!ieee754_doubles)
        globally_disabled_capabilities |= STREAM_CAP_IEEE754;
    else
        globally_disabled_capabilities &= ~STREAM_CAP_IEEE754;
}
