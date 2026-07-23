//! The `otel-traces` Function: netdata wire types ([`wire`]), live
//! source resolution ([`sources`]), and the `FunctionHandler` glue
//! ([`handler`]) over the wire-neutral [`sfsq::traces`] engine.

mod handler;
mod sources;
mod wire;

pub(crate) use handler::OtelTracesHandler;
