// SPDX-License-Identifier: GPL-3.0-or-later

#include "proto/context/v1/context.pb.h"

#include "libnetdata/libnetdata.h"

#include "schema_wrapper_utils.h"

#include "context.h"

using namespace context::v1;

// ContextsSnapshot
contexts_snapshot_t contexts_snapshot_new(const char *claim_id, const char *node_id, uint64_t version)
{
    ContextsSnapshot *ctxs_snap = new ContextsSnapshot;

    if (ctxs_snap == NULL)
        return NULL;

    ctxs_snap->set_claim_id(claim_id);
    ctxs_snap->set_node_id(node_id);
    ctxs_snap->set_version(version);

    return ctxs_snap;
}

static void fill_ctx_updated(ContextUpdated *ctx, struct context_updated *c_ctx)
{
    ctx->set_id(c_ctx->id);
    ctx->set_version(c_ctx->version);
    ctx->set_first_entry(c_ctx->first_entry);
    ctx->set_last_entry(c_ctx->last_entry);
    ctx->set_deleted(c_ctx->deleted);
    ctx->set_title(c_ctx->title);
    ctx->set_priority(c_ctx->priority);
    ctx->set_chart_type(c_ctx->chart_type);
    ctx->set_units(c_ctx->units);
}

void contexts_snapshot_add_ctx_update(contexts_snapshot_t ctxs_snapshot, struct context_updated *ctx_update)
{
    ContextsSnapshot *ctxs_snap = (ContextsSnapshot *)ctxs_snapshot;
    ContextUpdated *ctx = ctxs_snap->add_contexts();

    fill_ctx_updated(ctx, ctx_update);
}

char *contexts_snapshot_2bin(contexts_snapshot_t ctxs_snapshot, size_t *len)
{
    ContextsSnapshot *ctxs_snap = (ContextsSnapshot *)ctxs_snapshot;
    *len = PROTO_COMPAT_MSG_SIZE_PTR(ctxs_snap);
    char *bin = (char*)mallocz(*len);
    if (!ctxs_snap->SerializeToArray(bin, *len)) {
        delete ctxs_snap;
        return NULL;
    }

    delete ctxs_snap;
    return bin;
}

// ContextsUpdated
contexts_updated_t contexts_updated_new(const char *claim_id, const char *node_id, uint64_t version_hash, uint64_t created_at)
{
    ContextsUpdated *ctxs_updated = new ContextsUpdated;

    if (ctxs_updated == NULL)
        return NULL;

    ctxs_updated->set_claim_id(claim_id);
    ctxs_updated->set_node_id(node_id);
    ctxs_updated->set_version_hash(version_hash);
    ctxs_updated->set_created_at(created_at);

    return ctxs_updated;
}

void contexts_updated_add_ctx_update(contexts_updated_t ctxs_updated, struct context_updated *ctx_update)
{
    ContextsUpdated *ctxs_update = (ContextsUpdated *)ctxs_updated;
    ContextUpdated *ctx = ctxs_update->add_contextupdates();

    fill_ctx_updated(ctx, ctx_update);
}

char *contexts_updated_2bin(contexts_updated_t ctxs_updated, size_t *len)
{
    ContextsUpdated *ctxs_update = (ContextsUpdated *)ctxs_updated;
    *len = PROTO_COMPAT_MSG_SIZE_PTR(ctxs_update);
    char *bin = (char*)mallocz(*len);
    if (!ctxs_update->SerializeToArray(bin, *len)) {
        delete ctxs_update;
        return NULL;
    }

    delete ctxs_update;
    return bin;
}
