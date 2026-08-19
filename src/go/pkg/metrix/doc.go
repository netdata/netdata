// SPDX-License-Identifier: GPL-3.0-or-later

// Package metrix provides metric stores, instruments, and snapshot readers for
// go.d V2 collectors and framework runtime instrumentation.
//
// Package layout:
//
//   - Public contracts live in interfaces.go, types.go, options.go,
//     read_options.go, errors.go, and host_scope.go.
//   - Shared committed-store state lives in store_model.go, descriptor_model.go,
//     series_helpers.go, identity.go, and retention.go.
//   - CollectorStore facade, cycle commit, descriptor authority, descriptor
//     registry, schema resolution, and descriptor retention live in
//     collector_store.go, collector_cycle.go, descriptor_*.go, and
//     collector_retention.go.
//   - RuntimeStore facade, model, immediate writes, overlay snapshots,
//     compaction, and retention live in runtime_store.go, runtime_model.go,
//     runtime_write.go, and runtime_commit.go.
//   - Writer, meter, Vec, seeded values, and instrument handles live in
//     backend.go, meter.go, vec*.go, seeded.go, and the per-instrument files.
//   - Label canonicalization and reusable label handles live in labels.go and
//     labelset.go.
//   - Snapshot reads and flattening live in reader.go and reader_flatten.go,
//     with metadata helpers in meta.go.
//
// The package intentionally keeps public API and implementation in one Go
// package so root-defined public type identity stays stable and hot-path callers
// do not pay facade/wrapper overhead. The selector expression API is the only
// subpackage under metrix and imports this package; metrix itself must not import
// selector.
package metrix
