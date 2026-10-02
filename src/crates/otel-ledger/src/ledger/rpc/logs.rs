//! The `otel-logs` Function (logs twin of [`super::traces`]): netdata wire types
//! ([`wire`]), the mapping to and from the wire-neutral [`sfsq::logs`] engine
//! ([`adapter`]), and the `FunctionHandler` glue ([`handler`]). Declaration-only
//! hub; its one export is re-exported by rpc/mod.rs and built in pipeline.rs.

mod adapter;
mod handler;
mod wire;

pub(crate) use handler::OtelLogsHandler;
