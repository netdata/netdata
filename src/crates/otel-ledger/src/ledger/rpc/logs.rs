//! The `otel-logs` Function: netdata wire types ([`wire`]), the mapping
//! to and from the wire-neutral [`sfsq::logs`] engine ([`adapter`]), and
//! the `FunctionHandler` glue ([`handler`]).

mod adapter;
mod handler;
mod wire;

pub(crate) use handler::{OtelLogsHandler, RemoteRead};
