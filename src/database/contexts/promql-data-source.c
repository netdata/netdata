// SPDX-License-Identifier: GPL-3.0-or-later
//
// PromQL data-source shim implementation.
//
// See `promql-data-source.h` for the public contract. This file is
// internal to Netdata: it links into the daemon, owns refcounted
// rrdcontext/rrddim handles for the lifetime of each resolved query, and
// translates the storage-engine iterator surface into the flat-typed
// shape the Rust PromQL evaluator consumes.
//
// Chunk 1 scope (SOW-0017):
//   - Host scope: NULL (localhost), "*" (all known hosts). Specific
//     host_machine_guid / hostname is a follow-up.
//   - Matchers: EQ/NE applied during enumeration; RE/NRE returned as
//     candidates for Rust to post-filter.
//   - Tier: tier 0 only; richer tier selection is a chunk 5 task.
//   - step_ms: native samples only; the evaluator handles step alignment.
//
// Each refinement is tracked in the SOW.

#include "promql-data-source.h"
#include "rrdcontext.h"
#include "libnetdata/libnetdata.h"

#define ND_PDS_LABEL_NAME_MAX 200
#define ND_PDS_LABEL_VALUE_MAX 1024

// ---------------------------------------------------------------------------
// Internal data structures
// ---------------------------------------------------------------------------

typedef struct nd_pds_series_record {
    // Owned label storage. `label_blob` is a single allocation; `labels[i].name`
    // and `labels[i].value` point into it. Freed by `series_record_destroy`.
    char *label_blob;
    size_t label_blob_size;
    nd_pds_label *labels;
    size_t labels_len;

    // Stable handle for sample iteration. Acquired during resolve; released
    // by `series_record_destroy`.
    RRDDIM_ACQUIRED *rda;

    // Cached metadata needed for sample collapse and tier addressing.
    RRD_ALGORITHM algorithm;
    int32_t multiplier;
    int32_t divisor;

    uint64_t signature;
} nd_pds_series_record;

struct nd_pds_query {
    nd_pds_series_record *series;
    size_t series_count;
    size_t series_cap;
};

struct nd_pds_samples {
    struct storage_engine_query_handle seqh;
    RRD_ALGORITHM algorithm;
    int32_t multiplier;
    int32_t divisor;
    bool active;
};

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

static void set_err(char *err, size_t err_len, const char *msg) {
    if (!err || err_len == 0) return;
    strncpyz(err, msg, err_len - 1);
}

// Find the first __name__ EQ matcher; returns its value or NULL if no such
// matcher exists. This is the fast path that lets us go directly to the
// matching context rather than enumerating all contexts on the host.
static const char *find_name_eq(const nd_pds_matcher *matchers, size_t n) {
    for (size_t i = 0; i < n; i++) {
        if (matchers[i].op == ND_PDS_EQ
            && matchers[i].name
            && strcmp(matchers[i].name, "__name__") == 0)
            return matchers[i].value ? matchers[i].value : "";
    }
    return NULL;
}

// Evaluate one EQ/NE matcher against a (name, value) label. RE/NRE always
// pass at the shim layer -- Rust applies them post-resolution.
static bool matcher_accepts(const nd_pds_matcher *m, const char *value) {
    if (!m) return true;
    if (m->op == ND_PDS_RE || m->op == ND_PDS_NRE) return true;
    const char *expected = m->value ? m->value : "";
    const char *actual = value ? value : "";
    bool eq = (strcmp(actual, expected) == 0);
    return (m->op == ND_PDS_EQ) ? eq : !eq;
}

// 64-bit FNV-1a over a NUL-terminated string. Stable, cheap, sufficient
// for in-process series-identity hashing.
static uint64_t fnv1a64(const char *s, uint64_t seed) {
    uint64_t h = seed;
    while (s && *s) {
        h ^= (uint8_t)*s++;
        h *= 0x100000001b3ULL;
    }
    return h;
}

static uint64_t compute_signature(const nd_pds_label *labels, size_t n) {
    // Hash labels in their stored order. Caller is expected to sort by name
    // before insertion for cross-series-comparable signatures.
    uint64_t h = 0xcbf29ce484222325ULL;
    for (size_t i = 0; i < n; i++) {
        h = fnv1a64(labels[i].name, h);
        h ^= 0x5cULL;  // name/value separator in hash space
        h = fnv1a64(labels[i].value, h);
        h ^= 0x1eULL;  // record separator
    }
    return h;
}

// ---------------------------------------------------------------------------
// Label assembly
// ---------------------------------------------------------------------------

// Builder for the per-series label set. Accumulates (name, value) pairs into
// owned storage; emits a sorted-by-name array at finalize time.
typedef struct label_builder {
    // Backing storage; grows.
    char *blob;
    size_t blob_size;
    size_t blob_cap;

    // Indexes into the blob; (name_off, value_off) pairs.
    struct {
        size_t name_off;
        size_t value_off;
    } *entries;
    size_t entries_len;
    size_t entries_cap;
} label_builder;

static void lb_init(label_builder *b) {
    memset(b, 0, sizeof(*b));
}

static bool lb_reserve_blob(label_builder *b, size_t additional) {
    if (b->blob_size + additional <= b->blob_cap) return true;
    size_t newcap = b->blob_cap ? b->blob_cap * 2 : 256;
    while (newcap < b->blob_size + additional) newcap *= 2;
    char *r = reallocz(b->blob, newcap);
    if (!r) return false;
    b->blob = r;
    b->blob_cap = newcap;
    return true;
}

static bool lb_reserve_entries(label_builder *b) {
    if (b->entries_len < b->entries_cap) return true;
    size_t newcap = b->entries_cap ? b->entries_cap * 2 : 16;
    void *r = reallocz(b->entries, newcap * sizeof(b->entries[0]));
    if (!r) return false;
    b->entries = r;
    b->entries_cap = newcap;
    return true;
}

// Sanitize a label name for Prometheus compatibility (a-zA-Z0-9_:, leading
// alpha/_). For now we delegate to the existing libnetdata sanitizer used
// by the prometheus exporter so two paths produce the same names.
static size_t sanitize_label_name(char *dst, const char *src, size_t dst_size) {
    return prometheus_rrdlabels_sanitize_name(dst, src, dst_size);
}

// Add (name, value) to the builder. `name` is sanitized; `value` is taken
// verbatim. Empty values are skipped silently -- Prometheus treats absent
// and empty-string labels identically.
static bool lb_push(label_builder *b, const char *name, const char *value) {
    if (!name || !*name) return true;
    if (!value || !*value) return true;

    char sanitized_name[ND_PDS_LABEL_NAME_MAX + 1];
    if (sanitize_label_name(sanitized_name, name, sizeof(sanitized_name)) == 0)
        return true;  // sanitized to empty string -> drop silently

    size_t name_len = strlen(sanitized_name) + 1;
    size_t value_len = strlen(value) + 1;
    if (!lb_reserve_blob(b, name_len + value_len)) return false;
    if (!lb_reserve_entries(b)) return false;

    size_t name_off = b->blob_size;
    memcpy(b->blob + name_off, sanitized_name, name_len);
    b->blob_size += name_len;

    size_t value_off = b->blob_size;
    memcpy(b->blob + value_off, value, value_len);
    b->blob_size += value_len;

    b->entries[b->entries_len].name_off = name_off;
    b->entries[b->entries_len].value_off = value_off;
    b->entries_len++;
    return true;
}

// Walk-callback shape used by `rrdlabels_walkthrough_read`.
static int lb_label_walker(const char *name, const char *value, RRDLABEL_SRC ls __maybe_unused, void *data) {
    label_builder *b = data;
    return lb_push(b, name, value) ? 0 : -1;
}

static int lb_entry_cmp(const void *a, const void *b, void *ud) {
    const label_builder *lb = ud;
    const typeof(lb->entries[0]) *ae = a;
    const typeof(lb->entries[0]) *be = b;
    return strcmp(lb->blob + ae->name_off, lb->blob + be->name_off);
}

// Materialize the builder into the series record. Sorts entries by label
// name and emits a contiguous `nd_pds_label` array indexed into the blob.
// On success, ownership of `b->blob` transfers to the series record;
// `b->entries` is freed.
static bool lb_finalize(label_builder *b, nd_pds_series_record *rec) {
    if (b->entries_len == 0) {
        rec->label_blob = NULL;
        rec->label_blob_size = 0;
        rec->labels = NULL;
        rec->labels_len = 0;
        freez(b->blob);
        freez(b->entries);
        memset(b, 0, sizeof(*b));
        return true;
    }

    qsort_r(b->entries, b->entries_len, sizeof(b->entries[0]), lb_entry_cmp, b);

    nd_pds_label *out = mallocz(b->entries_len * sizeof(nd_pds_label));
    for (size_t i = 0; i < b->entries_len; i++) {
        out[i].name = b->blob + b->entries[i].name_off;
        out[i].value = b->blob + b->entries[i].value_off;
    }

    rec->label_blob = b->blob;
    rec->label_blob_size = b->blob_size;
    rec->labels = out;
    rec->labels_len = b->entries_len;

    freez(b->entries);
    memset(b, 0, sizeof(*b));
    return true;
}

static void lb_discard(label_builder *b) {
    freez(b->blob);
    freez(b->entries);
    memset(b, 0, sizeof(*b));
}

// Lookup a label value by name in the builder; returns NULL if absent.
static const char *lb_get(const label_builder *b, const char *name) {
    for (size_t i = 0; i < b->entries_len; i++) {
        const char *n = b->blob + b->entries[i].name_off;
        if (strcmp(n, name) == 0)
            return b->blob + b->entries[i].value_off;
    }
    return NULL;
}

// ---------------------------------------------------------------------------
// Matcher evaluation against a label builder
// ---------------------------------------------------------------------------

// Apply all non-__name__ matchers against the labels already in the builder.
// Returns true if every matcher accepts; false if any rejects (and RE/NRE
// don't reject at the shim layer, see matcher_accepts).
static bool builder_matches(const label_builder *b,
                            const nd_pds_matcher *matchers, size_t n) {
    for (size_t i = 0; i < n; i++) {
        const nd_pds_matcher *m = &matchers[i];
        if (!m->name) continue;
        if (strcmp(m->name, "__name__") == 0) continue;  // resolved at context level
        if (m->op == ND_PDS_RE || m->op == ND_PDS_NRE) continue;  // post-filter in Rust

        const char *actual = lb_get(b, m->name);
        if (!matcher_accepts(m, actual ? actual : ""))
            return false;
    }
    return true;
}

// ---------------------------------------------------------------------------
// Resolve plumbing
// ---------------------------------------------------------------------------

typedef struct resolve_state {
    nd_pds_query *q;
    const nd_pds_matcher *matchers;
    size_t matchers_len;
    size_t max_series;
    bool overflowed;

    // Per-host scratch.
    RRDHOST *current_host;
    const char *current_host_name;
} resolve_state;

static void series_record_destroy(nd_pds_series_record *rec) {
    if (!rec) return;
    if (rec->rda) rrddim_acquired_release(rec->rda);
    freez(rec->labels);
    freez(rec->label_blob);
    memset(rec, 0, sizeof(*rec));
}

static bool q_reserve(nd_pds_query *q) {
    if (q->series_count < q->series_cap) return true;
    size_t newcap = q->series_cap ? q->series_cap * 2 : 32;
    void *r = reallocz(q->series, newcap * sizeof(q->series[0]));
    if (!r) return false;
    q->series = r;
    q->series_cap = newcap;
    return true;
}

// Push one resolved (host, chart, dimension) tuple as a series into the query.
// On success, ownership of `b->blob` transfers to the new record.
static bool push_series(resolve_state *state,
                        label_builder *b,
                        RRDDIM_ACQUIRED *rda,
                        RRD_ALGORITHM algorithm,
                        int32_t multiplier, int32_t divisor) {
    if (state->q->series_count >= state->max_series) {
        state->overflowed = true;
        lb_discard(b);
        if (rda) rrddim_acquired_release(rda);
        return false;
    }
    if (!q_reserve(state->q)) {
        lb_discard(b);
        if (rda) rrddim_acquired_release(rda);
        return false;
    }
    nd_pds_series_record *rec = &state->q->series[state->q->series_count];
    memset(rec, 0, sizeof(*rec));
    if (!lb_finalize(b, rec)) {
        if (rda) rrddim_acquired_release(rda);
        return false;
    }
    rec->rda = rda;
    rec->algorithm = algorithm;
    rec->multiplier = multiplier;
    rec->divisor = divisor;
    rec->signature = compute_signature(rec->labels, rec->labels_len);
    state->q->series_count++;
    return true;
}

// Callback shape for `rrdcontext_foreach_instance_with_rrdset_in_context`.
// `data` carries our `resolve_state` plus the current context name. Returns
// negative to abort iteration; non-negative is treated as a count of work
// done.
typedef struct instance_cb_ctx {
    resolve_state *state;
    const char *context_name;
} instance_cb_ctx;

static int resolve_instance_cb(RRDSET *st, void *data) {
    instance_cb_ctx *ctx = data;
    resolve_state *state = ctx->state;

    if (rrdset_flag_check(st, RRDSET_FLAG_OBSOLETE))
        return 0;

    // Pre-build the chart-level portion of the labels. We'll dup it per
    // dimension so each emitted series carries the same chart-side context.
    label_builder chart_labels;
    lb_init(&chart_labels);

    bool ok = true;
    ok = ok && lb_push(&chart_labels, "__name__", ctx->context_name);
    ok = ok && lb_push(&chart_labels, "instance", state->current_host_name);
    ok = ok && lb_push(&chart_labels, "chart", rrdset_id(st));
    if (ok && st->family)
        ok = lb_push(&chart_labels, "family", rrdset_family(st));
    if (ok && st->rrdlabels)
        ok = (rrdlabels_walkthrough_read(st->rrdlabels, lb_label_walker, &chart_labels) >= 0);
    if (ok && state->current_host && state->current_host->rrdlabels)
        ok = (rrdlabels_walkthrough_read(state->current_host->rrdlabels, lb_label_walker, &chart_labels) >= 0);

    if (!ok) {
        lb_discard(&chart_labels);
        return -1;
    }

    if (!builder_matches(&chart_labels, state->matchers, state->matchers_len)) {
        lb_discard(&chart_labels);
        return 0;
    }

    // The foreach macro expands to `do { for(...) { ... } } while(0)`, so
    // we cannot goto out mid-loop -- the cleanup attribute on the local
    // DICTFE handles unlocking when we exit the do/while normally. To bail
    // early we flip `aborted` and continue past the remaining iterations.
    int ret = 0;
    bool aborted = false;
    RRDDIM *rd;
    rrddim_foreach_read(rd, st) {
        if (aborted) continue;
        if (rrddim_flag_check(rd, RRDDIM_FLAG_OBSOLETE))
            continue;

        // Dup the chart-level labels into a per-dimension builder.
        label_builder dim_labels;
        lb_init(&dim_labels);
        bool dim_ok = true;
        for (size_t i = 0; i < chart_labels.entries_len && dim_ok; i++) {
            const char *n = chart_labels.blob + chart_labels.entries[i].name_off;
            const char *v = chart_labels.blob + chart_labels.entries[i].value_off;
            dim_ok = lb_push(&dim_labels, n, v);
        }
        if (dim_ok)
            dim_ok = lb_push(&dim_labels, "dimension", rrddim_name(rd));
        if (!dim_ok) {
            lb_discard(&dim_labels);
            aborted = true;
            ret = -1;
            continue;
        }

        // Re-check matchers now that the `dimension` label is present.
        if (!builder_matches(&dim_labels, state->matchers, state->matchers_len)) {
            lb_discard(&dim_labels);
            continue;
        }

        RRDDIM_ACQUIRED *rda = rrddim_find_and_acquire(st, rrddim_id(rd), false);
        if (!rda) {
            lb_discard(&dim_labels);
            continue;
        }

        if (!push_series(state, &dim_labels, rda, rd->algorithm, rd->multiplier, rd->divisor)) {
            // push_series cleaned up both the builder and the acquired handle.
            aborted = true;
            ret = state->overflowed ? -1 : -1;
            continue;
        }
        ret++;
    }
    rrddim_foreach_done(rd);

    lb_discard(&chart_labels);
    return ret;
}

// Iterate every context known to `host` and call the resolver on each.
// Used when no `__name__` EQ matcher fixes the context up front. We pull
// the context id from the dictionary's foreach-iterator name rather than
// from `RRDCONTEXT` directly -- the value type is internal to the
// rrdcontext module.
static void resolve_all_contexts_on_host(resolve_state *state, RRDHOST *host) {
    if (!host || !host->rrdctx.contexts) return;

    state->current_host = host;
    state->current_host_name = rrdhost_hostname(host);

    void *unused;
    dfe_start_read(host->rrdctx.contexts, unused) {
        (void)unused;
        if (state->overflowed) break;
        const char *ctx_name = unused_dfe.name;
        if (!ctx_name || !*ctx_name) continue;
        instance_cb_ctx cb_ctx = { .state = state, .context_name = ctx_name };
        rrdcontext_foreach_instance_with_rrdset_in_context(host, ctx_name, resolve_instance_cb, &cb_ctx);
    }
    dfe_done(unused);
}

// Resolve a single specified context on a host.
static void resolve_named_context_on_host(resolve_state *state, RRDHOST *host, const char *context_name) {
    if (!host || !context_name || !*context_name) return;
    state->current_host = host;
    state->current_host_name = rrdhost_hostname(host);

    instance_cb_ctx cb_ctx = { .state = state, .context_name = context_name };
    rrdcontext_foreach_instance_with_rrdset_in_context(host, context_name, resolve_instance_cb, &cb_ctx);
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

nd_pds_query *
nd_pds_resolve(const char *host_machine_guid,
               const nd_pds_matcher *matchers, size_t matchers_len,
               int64_t after_s __maybe_unused, int64_t before_s __maybe_unused,
               size_t max_series,
               char *err, size_t err_len) {
    if (max_series == 0) max_series = 10000;

    nd_pds_query *q = callocz(1, sizeof(nd_pds_query));
    if (!q) {
        set_err(err, err_len, "out of memory");
        return NULL;
    }

    resolve_state state = {
        .q = q,
        .matchers = matchers,
        .matchers_len = matchers_len,
        .max_series = max_series,
        .overflowed = false,
    };

    const char *name_eq = find_name_eq(matchers, matchers_len);

    // Determine host scope.
    bool scan_all_hosts = (host_machine_guid && strcmp(host_machine_guid, "*") == 0);

    if (scan_all_hosts) {
        rrd_rdlock();
        RRDHOST *host;
        dfe_start_read(rrdhost_root_index, host) {
            if (state.overflowed) break;
            if (name_eq)
                resolve_named_context_on_host(&state, host, name_eq);
            else
                resolve_all_contexts_on_host(&state, host);
        }
        dfe_done(host);
        rrd_rdunlock();
    } else {
        // NULL or specific guid -> localhost for chunk 1.
        // Phase 1 follow-up: resolve specific GUID/hostname.
        RRDHOST *host = localhost;
        if (host) {
            if (name_eq)
                resolve_named_context_on_host(&state, host, name_eq);
            else
                resolve_all_contexts_on_host(&state, host);
        }
    }

    if (state.overflowed) {
        set_err(err, err_len, "exceeded max_series; tighten the query or raise the cap");
        nd_pds_free(q);
        return NULL;
    }

    if (q->series_count == 0) {
        nd_pds_free(q);
        return NULL;
    }

    return q;
}

size_t nd_pds_series_count(const nd_pds_query *q) {
    return q ? q->series_count : 0;
}

void nd_pds_series_metadata(const nd_pds_query *q, size_t i,
                            const nd_pds_label **labels_out,
                            size_t *labels_len_out,
                            uint64_t *signature_out) {
    if (!q || i >= q->series_count) {
        if (labels_out) *labels_out = NULL;
        if (labels_len_out) *labels_len_out = 0;
        if (signature_out) *signature_out = 0;
        return;
    }
    const nd_pds_series_record *rec = &q->series[i];
    if (labels_out) *labels_out = rec->labels;
    if (labels_len_out) *labels_len_out = rec->labels_len;
    if (signature_out) *signature_out = rec->signature;
}

nd_pds_samples *
nd_pds_open_samples(const nd_pds_query *q, size_t i,
                    int64_t after_s, int64_t before_s,
                    int64_t step_ms __maybe_unused) {
    if (!q || i >= q->series_count) return NULL;
    nd_pds_series_record *rec = &q->series[i];
    RRDDIM *rd = rrddim_acquired_to_rrddim(rec->rda);
    if (!rd) return NULL;

    // Chunk 1: always tier 0. Tier selection is a chunk 5 refinement.
    size_t tier = 0;
    STORAGE_METRIC_HANDLE *smh = rd->tiers[tier].smh;
    if (!smh) return NULL;

    nd_pds_samples *it = callocz(1, sizeof(nd_pds_samples));
    if (!it) return NULL;
    it->algorithm = rec->algorithm;
    it->multiplier = rec->multiplier;
    it->divisor = rec->divisor;
    it->active = true;

    storage_engine_query_init(rd->tiers[tier].seb, smh, &it->seqh,
                              (time_t)after_s, (time_t)before_s,
                              STORAGE_PRIORITY_NORMAL);
    return it;
}

// Collapse a STORAGE_POINT to a single PromQL-friendly double based on the
// dimension's algorithm.
//   ABSOLUTE / PCENT_OVER_*: gauge -> average (sum/count).
//   INCREMENTAL: counter -> total delta in the bucket (sum is already a
//                sum of deltas; that's exactly what `rate()` wants).
static double collapse_storage_point(const STORAGE_POINT *sp, RRD_ALGORITHM algo) {
    if (sp->count == 0) return NAN;
    switch (algo) {
        case RRD_ALGORITHM_INCREMENTAL:
            return (double)sp->sum;
        case RRD_ALGORITHM_ABSOLUTE:
        case RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL:
        case RRD_ALGORITHM_PCENT_OVER_ROW_TOTAL:
        default:
            return (double)(sp->sum / (NETDATA_DOUBLE)sp->count);
    }
}

int nd_pds_samples_next(nd_pds_samples *it,
                        int64_t *ts_ms_out,
                        double *value_out,
                        uint32_t *flags_out) {
    if (!it || !it->active) return -1;
    if (storage_engine_query_is_finished(&it->seqh)) return 0;

    STORAGE_POINT sp = storage_engine_query_next_metric(&it->seqh);
    if (ts_ms_out)
        *ts_ms_out = (int64_t)sp.end_time_s * 1000;
    if (value_out)
        *value_out = collapse_storage_point(&sp, it->algorithm);
    if (flags_out)
        *flags_out = (uint32_t)sp.flags;
    return 1;
}

void nd_pds_samples_close(nd_pds_samples *it) {
    if (!it) return;
    if (it->active) {
        storage_engine_query_finalize(&it->seqh);
        it->active = false;
    }
    freez(it);
}

void nd_pds_free(nd_pds_query *q) {
    if (!q) return;
    for (size_t i = 0; i < q->series_count; i++)
        series_record_destroy(&q->series[i]);
    freez(q->series);
    freez(q);
}
