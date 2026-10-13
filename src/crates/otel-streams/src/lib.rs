use opentelemetry_proto::tonic::logs::v1::LogRecord;
use serde::Deserialize;

pub mod args;
pub mod certstream;
pub mod github;
pub mod jetstream;
pub mod otel;
pub mod ris;
pub mod runner;
pub mod sender;
pub mod sse;
pub mod synth;
// Deterministic synthetic-traces generator: regression-test tooling for edge
// cases (orphans, multi-root, skew, resends); real development traffic comes
// from the OTel demo (plan decision D6).
pub mod synth_traces;
pub mod wikimedia;
pub mod ws;

pub trait Source: Send + 'static {
    const SERVICE_NAME: &'static str;
    const SCOPE_NAME: &'static str;
    const SCOPE_VERSION: &'static str;

    type Event: for<'de> Deserialize<'de> + Send + 'static;

    fn event_to_log_record(event: &Self::Event, raw_json: &serde_json::Value) -> LogRecord;
}
