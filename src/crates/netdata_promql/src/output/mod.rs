// SPDX-License-Identifier: GPL-3.0-or-later

//! JSON serialization for Prometheus API responses.
//!
//! Serializes evaluator results into the Prometheus HTTP API JSON format
//! (`/api/v1/query` and `/api/v1/query_range`), plus label/series/metadata
//! discovery responses.

mod discovery_json;
mod prometheus_json;

pub use discovery_json::{
    MetadataEntry, serialize_metadata_map, serialize_series_list, serialize_string_list,
};
pub use prometheus_json::{serialize_error, serialize_scalar_at, serialize_success};
