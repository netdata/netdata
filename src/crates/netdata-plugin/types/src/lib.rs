//! Shared value types for Netdata's Rust plugin crates: the pluginsd
//! function-protocol payload structs, the dyncfg configuration vocabulary,
//! and the agent HTTP access bitmask.
//!
//! Deliberately dependency-light — no codec, async or transport code — so
//! crates that need the structs depend on this crate rather than
//! netdata-plugin-protocol, which re-exports all of them and maps them onto
//! the pluginsd keyword lines (FUNCTION, FUNCTION_RESULT_BEGIN, CONFIG, ...;
//! see netdata-plugin-protocol for the wire formats).
//!
//! - `functions`: the pluginsd function-protocol payload structs carried by
//!   protocol's `Message` variants. Their `Serialize`/`Deserialize` impls
//!   carry them inside the `bridge` crate's ferryboat IPC messages; the
//!   pluginsd wire mapping itself lives in netdata-plugin-protocol.
//! - `config` and `dyncfg_*`: [`ConfigDeclaration`] and the dyncfg
//!   vocabulary, parsed from a schema's `configDeclaration` JSON and emitted
//!   by the protocol as the `CONFIG` line; re-exported by
//!   netdata-plugin-schema for schema builders.
//! - `http_access`: [`HttpAccess`], the 11-bit access bitmask carried by
//!   function declarations, function calls and config declarations.
//!
//! All submodules are private; the re-exports below are the whole public API.
//! Direct consumers: netdata-plugin-protocol, netdata-plugin-schema, bridge,
//! otel-plugin, otel-ledger, otel-ingestor and otel-legacy-logs; `rt` reads
//! the structs through netdata-plugin-protocol's re-exports.
mod config;
mod dyncfg_cmds;
mod dyncfg_source_type;
mod dyncfg_status;
mod dyncfg_type;
mod functions;
mod http_access;

pub use config::ConfigDeclaration;
pub use dyncfg_cmds::DynCfgCmds;
pub use dyncfg_source_type::DynCfgSourceType;
pub use dyncfg_status::DynCfgStatus;
pub use dyncfg_type::DynCfgType;

pub use functions::{
    FunctionCall, FunctionCancel, FunctionDeclaration, FunctionProgressRequest,
    FunctionProgressResponse, FunctionResult,
};
pub use http_access::HttpAccess;
