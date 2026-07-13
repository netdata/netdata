//! The minimal hand-rolled tempopb envelope (plan-consult resolution,
//! 2026-07-12, unanimous): `tempopb.Trace` is wire-identical to OTLP
//! `TracesData` — `message Trace { repeated tempopb.trace.v1.ResourceSpans
//! resourceSpans = 1; }` (`tempo/pkg/tempopb/tempo.proto:295-297`), and
//! `tempopb.trace.v1` IS the OTLP trace proto — so the payload reuses
//! the workspace `opentelemetry-proto` prost types and only the by-id
//! envelope (`tempo.proto:56-61`) is declared here. No tempopb codegen,
//! no build script.
//!
//! The `metrics` field (tag 2, `TraceByIDMetrics`) is deliberately not
//! declared: the plugin never reads it, and an absent optional message
//! field is wire-legal proto3.

use opentelemetry_proto::tonic::trace::v1::ResourceSpans;

/// `tempopb.Trace` — the v1 `/api/traces/{id}` response body is these
/// raw bytes (no envelope: the plugin `proto.Unmarshal`s straight into
/// `tempopb.Trace`, `grafana-tempo-datasource pkg/tempo/trace.go:91-92`).
#[derive(Clone, PartialEq, prost::Message)]
pub struct Trace {
    #[prost(message, repeated, tag = "1")]
    pub resource_spans: Vec<ResourceSpans>,
}

/// `tempopb.PartialStatus` (`tempo.proto:401-404`).
#[derive(Clone, Copy, Debug, PartialEq, Eq, prost::Enumeration)]
#[repr(i32)]
pub enum PartialStatus {
    Complete = 0,
    Partial = 1,
}

/// `tempopb.TraceByIDResponse` (`tempo.proto:56-61`) — the v2
/// `/api/v2/traces/{id}` response body. The plugin reads `trace`,
/// `status` (PARTIAL drives a UI indicator, `pkg/tempo/trace.go:143-145`)
/// and `message`.
#[derive(Clone, PartialEq, prost::Message)]
pub struct TraceByIdResponse {
    #[prost(message, optional, tag = "1")]
    pub trace: Option<Trace>,
    #[prost(enumeration = "PartialStatus", tag = "3")]
    pub status: i32,
    #[prost(string, tag = "4")]
    pub message: String,
}
