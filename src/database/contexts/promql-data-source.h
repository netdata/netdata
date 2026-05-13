// SPDX-License-Identifier: GPL-3.0-or-later
//
// PromQL data-source shim: a flat-typed C surface that exposes Netdata's
// storage layer to the Rust PromQL evaluator without leaking internal types.
//
// Phase 1 of SOW-0017. Lifetimes: a `nd_pds_query` owns refcounted handles
// into the rrdcontext/rrddim layers and releases them at `nd_pds_free` time.
// A `nd_pds_samples` borrows from its parent query and must be closed before
// the query is freed. All strings returned through the API remain valid
// until the owning handle is freed.

#ifndef ND_PROMQL_DATA_SOURCE_H
#define ND_PROMQL_DATA_SOURCE_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef struct nd_pds_query   nd_pds_query;
typedef struct nd_pds_samples nd_pds_samples;

// PromQL matcher operators. Values are stable: the Rust side mirrors them
// in `repr(C)`. EQ and NE are evaluated inside the shim during enumeration.
// RE and NRE are passed through and applied by the Rust side after the
// candidate series list is returned -- the shim does no regex compilation.
typedef enum {
    ND_PDS_EQ  = 0,
    ND_PDS_NE  = 1,
    ND_PDS_RE  = 2,
    ND_PDS_NRE = 3,
} nd_pds_match_op;

typedef struct {
    const char *name;        // label name; "__name__" is special and matches the metric name
    int op;                  // nd_pds_match_op
    const char *value;       // EQ/NE: literal; RE/NRE: regex source, uncompiled
} nd_pds_matcher;

typedef struct {
    const char *name;
    const char *value;
} nd_pds_label;

// Resolve a query: scan the storage for series matching the matcher set,
// build their label sets, acquire stable handles for subsequent sample
// iteration, and return a query handle.
//
// host_machine_guid:
//   NULL  -> localhost only
//   "*"   -> localhost plus all known child hosts on this agent
//   other -> the host whose machine_guid or hostname matches
//
// matchers: zero or more label-equality / regex predicates. A matcher with
//   name="__name__" addresses the metric name (i.e. the Netdata context).
//   The shim resolves __name__ EQ matchers to specific contexts; non-EQ
//   __name__ matchers cause the shim to enumerate all contexts on each
//   host and post-filter.
//
// after_s / before_s: the query time range in Unix seconds. Used as a
//   retention hint for instance filtering; sample iteration takes its own
//   time range later.
//
// max_series: cardinality backstop. If resolution would yield more than
//   this many series, the call fails with an error.
//
// err / err_len: caller-provided buffer for an error message on failure.
//   NULL is permitted; the call still returns NULL on failure.
//
// Returns NULL on error or on no matching series. Caller must free with
// `nd_pds_free`.
nd_pds_query *
nd_pds_resolve(const char *host_machine_guid,
               const nd_pds_matcher *matchers, size_t matchers_len,
               int64_t after_s, int64_t before_s,
               size_t max_series,
               char *err, size_t err_len);

// Number of series resolved.
size_t nd_pds_series_count(const nd_pds_query *q);

// Read-only access to the label set of one resolved series. Sets
// `*labels_out` to a pointer into the query's owned storage and
// `*labels_len_out` to the array length. `*signature_out` is a stable
// 64-bit hash of the label set, sufficient for the evaluator to detect
// duplicate series after vector-matching and aggregation steps.
//
// The returned pointers remain valid until `nd_pds_free(q)` is called.
void nd_pds_series_metadata(const nd_pds_query *q, size_t i,
                            const nd_pds_label **labels_out,
                            size_t *labels_len_out,
                            uint64_t *signature_out);

// Open a sample iterator for one resolved series over a time range.
// step_ms == 0 requests native-resolution samples (one point per stored
// `STORAGE_POINT` in the time range). step_ms > 0 requests one point per
// step on the step grid; the shim selects the appropriate storage tier.
//
// Returns NULL on failure (out-of-range series index, allocation failure).
// Caller must close with `nd_pds_samples_close`.
nd_pds_samples *
nd_pds_open_samples(const nd_pds_query *q, size_t i,
                    int64_t after_s, int64_t before_s,
                    int64_t step_ms);

// Advance the sample iterator. Returns 1 if a point was written into
// `*ts_ms_out`/`*value_out`/`*flags_out`, 0 on end-of-stream, -1 on error.
//
// `*flags_out` carries `SN_FLAG_*` bits from the storage layer: the
// staleness marker and the reset marker are surfaced; other bits are not
// guaranteed to be meaningful.
int nd_pds_samples_next(nd_pds_samples *it,
                        int64_t *ts_ms_out,
                        double *value_out,
                        uint32_t *flags_out);

// Close a sample iterator. Safe to call on NULL.
void nd_pds_samples_close(nd_pds_samples *it);

// Release a query handle and all resources it owns. Any open sample
// iterators must be closed first. Safe to call on NULL.
void nd_pds_free(nd_pds_query *q);

#ifdef __cplusplus
}
#endif

#endif /* ND_PROMQL_DATA_SOURCE_H */
