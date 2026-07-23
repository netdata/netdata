//! The `otel-traces` Function: netdata wire types ([`wire`]), the
//! mapping to and from the wire-neutral [`sfsq::traces`] engine
//! ([`adapter`]), live source resolution ([`sources`]), and the
//! `FunctionHandler` glue ([`handler`]).

mod adapter;
#[cfg(test)]
pub(crate) mod fixtures;
mod handler;
mod sources;
mod wire;

pub(crate) use handler::OtelTracesHandler;
