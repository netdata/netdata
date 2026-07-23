//! `OtelTracesHandler` — typed `FunctionHandler` implementation for the
//! `otel-traces` Function.
//!
//! Traces-ui phase-1 step 1.1: the Function is advertised and answers
//! `info` (capability discovery); every data mode resolves cleanly and
//! returns an explicit not-implemented error until its step lands. The
//! wire contract lives in [`super::wire`], the source-resolution
//! foundation the data modes will consume in [`super::sources`].
//!
//! Netdata-plugin glue only, like the logs handler: the engine
//! ([`sfsq::traces`]) stays wire-neutral; the bridge's `HandlerAdapter`
//! owns the JSON round-trip, progress ticker, and cancellation.

use std::sync::Arc;

use async_trait::async_trait;
use bridge::function::{FunctionCallContext, FunctionHandler};
use netdata_plugin_protocol::FunctionDeclaration;
use netdata_plugin_types::HttpAccess;
use tokio::sync::RwLock;

use file_lifecycle::chunk::ChunkCache;
use file_lifecycle::registry::TenantRegistries;

use super::sources::TracesSourceSupplier;
use super::wire::{InfoResponse, OtelTracesRequest, OtelTracesResponse, RequestMode};

pub(crate) struct OtelTracesHandler {
    /// Live source resolution (registries snapshot + WAL chunk builds).
    /// Consumed by the data modes as they land (steps 1.2+); `info` and
    /// the not-implemented errors never touch storage.
    #[allow(dead_code)]
    supplier: TracesSourceSupplier,
}

impl OtelTracesHandler {
    pub(crate) fn new(
        registries: Arc<RwLock<TenantRegistries>>,
        chunk_cache: Arc<ChunkCache>,
        min_entries: u64,
    ) -> Self {
        Self {
            supplier: TracesSourceSupplier::new(registries, chunk_cache, min_entries),
        }
    }
}

#[async_trait]
impl FunctionHandler for OtelTracesHandler {
    type Request = OtelTracesRequest;
    type Response = OtelTracesResponse;

    async fn on_call(
        &self,
        _ctx: FunctionCallContext,
        req: Self::Request,
    ) -> netdata_plugin_error::Result<Self::Response> {
        let mode = req.mode().map_err(|conflict| {
            netdata_plugin_error::NetdataPluginError::FunctionHandler {
                message: format!("invalid otel-traces request: {conflict}"),
            }
        })?;
        match mode {
            RequestMode::Info => Ok(OtelTracesResponse::Info(InfoResponse::default())),
            other => Err(netdata_plugin_error::NetdataPluginError::FunctionHandler {
                message: format!(
                    "otel-traces mode '{}' is not implemented yet",
                    other.name()
                ),
            }),
        }
    }

    fn declaration(&self) -> FunctionDeclaration {
        let mut d = FunctionDeclaration::new("otel-traces", "Query OpenTelemetry traces");
        d.global = true;
        d.tags = Some("traces".to_string());
        d.access =
            Some(HttpAccess::SIGNED_ID | HttpAccess::SAME_SPACE | HttpAccess::SENSITIVE_DATA);
        d
    }
}

#[cfg(test)]
mod tests;
