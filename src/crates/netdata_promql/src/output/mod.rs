// SPDX-License-Identifier: GPL-3.0-or-later

mod discovery_json;
mod prometheus_json;

pub use discovery_json::{
    serialize_metadata_map, serialize_series_list, serialize_string_list, MetadataEntry,
};
pub use prometheus_json::{serialize_error, serialize_scalar_at, serialize_success};
