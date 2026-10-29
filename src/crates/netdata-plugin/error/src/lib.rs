//! Shared error taxonomy for Netdata's Rust plugin crates: one enum
//! ([`NetdataPluginError`]) plus the [`Result`] alias every fallible
//! netdata-plugin API returns. The crate holds no logic; its only conversion
//! is the derived `From<std::io::Error>` on the `Transport` variant, so the
//! taxonomy depends on nothing beyond thiserror.
//!
//! Who builds what (grep-verified over src/crates):
//!
//! - `Transport` is never built by name — `#[from] std::io::Error` converts
//!   `?` on writer I/O in netdata-plugin-protocol's `MessageWriter`
//!   (`send`/`flush`/`write_raw`), which `rt`'s function-declaration and
//!   stdout-write paths call through.
//! - `Protocol` comes only from netdata-plugin-protocol's `MessageReader`
//!   decode path.
//! - `Schema` comes only from `ConfigDeclaration::try_from(&serde_json::Value)`
//!   in netdata-plugin-types.
//! - `BadRequest` is built by the otel-ledger otel-traces handler for the
//!   requests it rejects; `FunctionHandler` by the otel-ledger otel-logs and
//!   otel-traces handlers for everything else; `Other` by the function engines
//!   in `rt`/`otel-bridge` (cancellation) and the otel-legacy-logs and
//!   netflow-plugin handlers. `Runtime`, `Config` and `Closed` have no
//!   constructor in the tree.
//!
//! Consumers type function handlers with [`Result`]: the `FunctionHandler`
//! traits in `rt` and `otel-bridge`, `PluginRuntime::run`, and the otel-ledger,
//! otel-legacy-logs and netflow-plugin handlers. At the boundary, `otel-bridge`
//! renders `BadRequest` as an HTTP 400 result whose JSON body carries the
//! bare message, and any other handler error as an HTTP 500 result whose JSON
//! body carries the variant's Display text. `rt` renders every handler error,
//! `BadRequest` included, as 500 (499 when cancelled); only a request it cannot
//! deserialize is a 400 there.
//! The otel-plugin supervisor converts transport errors into anyhow with `?`
//! (adding its own context).
//!
//! netdata-plugin-protocol re-exports [`NetdataPluginError`] and [`Result`]
//! and defines the `TransportError` alias (in `transport.rs`); there is no
//! alias in this crate.
use thiserror::Error;

/// Error side of every fallible netdata-plugin API; re-exported by
/// netdata-plugin-protocol alongside the message types.
pub type Result<T> = std::result::Result<T, NetdataPluginError>;

/// Errors of the netdata-plugin framework.
///
/// Only `Transport` carries a `source()` — the wrapped `std::io::Error`;
/// every other variant is message-only, so a cause dropped when the error
/// was built cannot be recovered. Display conventions: the six categorized
/// variants prefix their message with a lowercase `<category>: ` label,
/// `Other` prints the bare message, and `Transport` is transparent. Callers
/// render with `Display` (`{}`) into agent-visible bodies and logs, and with
/// `Debug` (`{:?}`) where the variant name matters more than the text.
#[derive(Error, Debug)]
pub enum NetdataPluginError {
    /// I/O failure from the plugin's streams (its stdin/stdout wiring, or any
    /// other `AsyncRead`/`AsyncWrite` the transport wraps). Transparent so
    /// the I/O error prints once: `Display` and `source()` both delegate to
    /// the wrapped error, which anyhow chains already carry.
    #[error(transparent)]
    Transport(#[from] std::io::Error),

    /// The agent-to-plugin stream failed to decode. Built only by
    /// netdata-plugin-protocol's `MessageReader::recv` (and its `Stream`
    /// impl), which maps the codec decoder error here with the Debug form of
    /// the `line_parser` error as the message — in practice always `"Io"`,
    /// since the decoder turns `IncompleteLine` into "need more data" and
    /// `MalformedLine` is never raised. The cause is not preserved. The `rt`
    /// event loop logs it (`{:?}`) and keeps polling; the reader then yields
    /// `None` and the loop ends. Write-side I/O does not use this variant —
    /// it surfaces as `Transport`.
    #[error("protocol error: {message}")]
    Protocol { message: String },

    /// Failure during plugin runtime execution. No constructor in the current
    /// tree; handlers report such failures as `Other` or `FunctionHandler`.
    #[error("runtime error: {message}")]
    Runtime { message: String },

    /// Configuration error. No constructor in the current tree.
    #[error("configuration error: {message}")]
    Config { message: String },

    /// The caller's request is invalid: retrying it unchanged fails again.
    /// Built by the otel-ledger otel-traces handler when it rejects a request
    /// after deserialization (a bad chip, window or trace id, or a query that
    /// needs more remote data than the download cache holds). The message is
    /// user-facing: `otel-bridge` turns the error into an HTTP 400 result whose
    /// JSON body carries the bare message as `errorMessage`.
    #[error("bad request: {message}")]
    BadRequest { message: String },

    /// A function handler failed; the caller-visible bucket for `on_call`
    /// failures. Built by the otel-ledger otel-logs handler (request
    /// validation included) and the otel-traces handler (download-cache
    /// eviction and planning failures, internal source-set inconsistencies,
    /// engine rejections its own checks should have caught). Messages are
    /// user-facing: `otel-bridge` turns the error into an HTTP 500 result whose
    /// JSON body carries this Display text.
    #[error("function handler error: {message}")]
    FunctionHandler { message: String },

    /// A generated schema could not be parsed into its dyncfg declaration.
    /// Built only by `ConfigDeclaration::try_from(&serde_json::Value)` in
    /// netdata-plugin-types, when the schema JSON lacks `configDeclaration`
    /// or one of its required keys; the message names the key even when the
    /// key exists with an unknown value or wrong type. Messages read
    /// `Missing <key> ...`.
    #[error("schema validation error: {message}")]
    Schema { message: String },

    /// Terminal state of a shut-down transport. No constructor in the current
    /// tree.
    #[error("transport is closed")]
    Closed,

    /// Anything without a dedicated variant, displayed as the bare message.
    /// Built by the function engines in `rt` and `otel-bridge` when a handler is
    /// cancelled (`"Function cancelled"`), and by the otel-legacy-logs and
    /// netflow-plugin handlers for their stage failures; legacy-logs prefixes
    /// each message with the transaction id.
    #[error("{message}")]
    Other { message: String },
}
