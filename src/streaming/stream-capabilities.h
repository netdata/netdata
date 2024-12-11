// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STREAM_CAPABILITIES_H
#define NETDATA_STREAM_CAPABILITIES_H

#include "libnetdata/libnetdata.h"

// ----------------------------------------------------------------------------
// obsolete versions - do not use anymore

#define STREAM_OLD_VERSION_CLAIM 3
#define STREAM_OLD_VERSION_CLABELS 4
#define STREAM_OLD_VERSION_LZ4 5

// ----------------------------------------------------------------------------
// capabilities negotiation

typedef enum {
    STREAM_CAP_NONE             = 0,

    // do not use the first 3 bits
    // they used to be versions 1, 2 and 3
    // before we introduce capabilities

    STREAM_CAP_V1               = (1 << 3), // v1 = the oldest protocol
    STREAM_CAP_V2               = (1 << 4), // v2 = the second version of the protocol (with host labels)
    STREAM_CAP_VN               = (1 << 5), // version negotiation supported (for versions 3, 4, 5 of the protocol)
                                            // v3 = claiming supported
                                            // v4 = chart labels supported
                                            // v5 = lz4 compression supported
    STREAM_CAP_VCAPS            = (1 << 6), // capabilities negotiation supported
    STREAM_CAP_HLABELS          = (1 << 7), // host labels supported
    STREAM_CAP_CLAIM            = (1 << 8), // claiming supported
    STREAM_CAP_CLABELS          = (1 << 9), // chart labels supported
    STREAM_CAP_LZ4              = (1 << 10), // lz4 compression supported
    STREAM_CAP_FUNCTIONS        = (1 << 11), // plugin functions supported
    STREAM_CAP_REPLICATION      = (1 << 12), // replication supported
    STREAM_CAP_BINARY           = (1 << 13), // streaming supports binary data
    STREAM_CAP_INTERPOLATED     = (1 << 14), // streaming supports interpolated streaming of values
    STREAM_CAP_IEEE754          = (1 << 15), // streaming supports binary/hex transfer of double values
    STREAM_CAP_DATA_WITH_ML     = (1 << 16), // leave this unused for as long as possible - NOT USED, BUT KEEP IT
    // STREAM_CAP_DYNCFG        = (1 << 17), // leave this unused for as long as possible
    STREAM_CAP_SLOTS            = (1 << 18), // the sender can appoint a unique slot for each chart
    STREAM_CAP_ZSTD             = (1 << 19), // ZSTD compression supported
    STREAM_CAP_GZIP             = (1 << 20), // GZIP compression supported
    STREAM_CAP_BROTLI           = (1 << 21), // BROTLI compression supported
    STREAM_CAP_PROGRESS         = (1 << 22), // Functions PROGRESS support
    STREAM_CAP_DYNCFG           = (1 << 23), // support for DYNCFG
    STREAM_CAP_NODE_ID          = (1 << 24), // support for sending NODE_ID back to the child
    STREAM_CAP_PATHS            = (1 << 25), // support for sending PATHS upstream and downstream
    STREAM_CAP_ML_MODELS        = (1 << 26), // support for sending MODELS upstream

    STREAM_CAP_INVALID          = (1 << 30), // used as an invalid value for capabilities when this is set
    // this must be signed int, so don't use the last bit
    // needed for negotiating errors between parent and child
} STREAM_CAPABILITIES;

#define STREAM_CAP_ALWAYS_DISABLED (STREAM_CAP_DATA_WITH_ML)

#ifdef ENABLE_LZ4
#define STREAM_CAP_LZ4_AVAILABLE STREAM_CAP_LZ4
#else
#define STREAM_CAP_LZ4_AVAILABLE 0
#endif  // ENABLE_LZ4

#ifdef ENABLE_ZSTD
#define STREAM_CAP_ZSTD_AVAILABLE STREAM_CAP_ZSTD
#else
#define STREAM_CAP_ZSTD_AVAILABLE 0
#endif  // ENABLE_ZSTD

#ifdef ENABLE_BROTLI
#define STREAM_CAP_BROTLI_AVAILABLE STREAM_CAP_BROTLI
#else
#define STREAM_CAP_BROTLI_AVAILABLE 0
#endif  // ENABLE_BROTLI

#define STREAM_CAP_COMPRESSIONS_AVAILABLE (STREAM_CAP_LZ4_AVAILABLE|STREAM_CAP_ZSTD_AVAILABLE|STREAM_CAP_BROTLI_AVAILABLE|STREAM_CAP_GZIP)

#define stream_has_capability(rpt, capability) ((rpt) && ((rpt)->capabilities & (capability)) == (capability))

static inline bool stream_has_more_than_one_capability_of(STREAM_CAPABILITIES caps, STREAM_CAPABILITIES mask) {
    STREAM_CAPABILITIES common = (STREAM_CAPABILITIES)(caps & mask);
    return (common & (common - 1)) != 0 && common != 0;
}

struct sender_state;
struct receiver_state;
struct rrdhost;

STREAM_CAPABILITIES stream_capabilities_parse_one(const char *str);

void stream_capabilities_to_string(BUFFER *wb, STREAM_CAPABILITIES caps);
void stream_capabilities_to_json_array(BUFFER *wb, STREAM_CAPABILITIES caps, const char *key);
void log_receiver_capabilities(struct receiver_state *rpt);
void log_sender_capabilities(struct sender_state *s);
STREAM_CAPABILITIES convert_stream_version_to_capabilities(int32_t version, struct rrdhost *host, bool sender);
int32_t stream_capabilities_to_vn(uint32_t caps);
STREAM_CAPABILITIES stream_our_capabilities(struct rrdhost *host, bool sender);

void check_local_streaming_capabilities(void);

#endif //NETDATA_STREAM_CAPABILITIES_H
