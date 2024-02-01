// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ACLK_SCHEMA_WRAPPER_CONTEXT_STREAM_H
#define ACLK_SCHEMA_WRAPPER_CONTEXT_STREAM_H

#ifdef __cplusplus
extern "C" {
#endif

struct stop_streaming_ctxs {
    char *claim_id;
    char *node_id;
    // we omit reason as there is only one defined at this point
    // as soon as there is more than one defined in StopStreaminContextsReason
    // we should add it
    // 0 - RATE_LIMIT_EXCEEDED
};

struct stop_streaming_ctxs *parse_stop_streaming_ctxs(const char *data, size_t len);

struct ctxs_checkpoint {
    char *claim_id;
    char *node_id;

    uint64_t version_hash;
};

struct ctxs_checkpoint *parse_ctxs_checkpoint(const char *data, size_t len);



#ifdef __cplusplus
}
#endif

#endif /* ACLK_SCHEMA_WRAPPER_CONTEXT_STREAM_H */
