// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ACLK_SCHEMA_WRAPPER_CONTEXT_H
#define ACLK_SCHEMA_WRAPPER_CONTEXT_H

#include <stdint.h>
#include <sys/types.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef void* contexts_updated_t;
typedef void* contexts_snapshot_t;

struct context_updated {
    // context id
    const char *id;

    uint64_t version;

    uint64_t first_entry;
    uint64_t last_entry;

    int deleted;

    const char *title;
    uint64_t priority;
    const char *chart_type;
    const char *units;
    const char *family;
};

// ContextS Snapshot related
contexts_snapshot_t contexts_snapshot_new(const char *claim_id, const char *node_id, uint64_t version);
void contexts_snapshot_delete(contexts_snapshot_t ctxs_snapshot);
void contexts_snapshot_set_version(contexts_snapshot_t ctxs_snapshot, uint64_t version);
void contexts_snapshot_add_ctx_update(contexts_snapshot_t ctxs_snapshot, struct context_updated *ctx_update);
char *contexts_snapshot_2bin(contexts_snapshot_t ctxs_snapshot, size_t *len);

// ContextS Updated related
contexts_updated_t contexts_updated_new(const char *claim_id, const char *node_id, uint64_t version_hash, uint64_t created_at);
void contexts_updated_delete(contexts_updated_t ctxs_updated);
void contexts_updated_update_version_hash(contexts_updated_t ctxs_updated, uint64_t version_hash);
void contexts_updated_add_ctx_update(contexts_updated_t ctxs_updated, struct context_updated *ctx_update);
char *contexts_updated_2bin(contexts_updated_t ctxs_updated, size_t *len);


#ifdef __cplusplus
}
#endif

#endif /* ACLK_SCHEMA_WRAPPER_CONTEXT_H */
