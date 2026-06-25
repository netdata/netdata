use opentelemetry_proto::tonic::logs::v1::LogRecord;
use serde::Deserialize;

pub mod args;
pub mod certstream;
pub mod github;
pub mod jetstream;
pub mod otel;
pub mod runner;
pub mod sender;
pub mod synth;
// PROOF SCAFFOLD (otel traces-proof SOW; revert with the skeleton).
pub mod synth_traces;
pub mod ws;

pub trait Source: Send + 'static {
    const SERVICE_NAME: &'static str;
    const SCOPE_NAME: &'static str;
    const SCOPE_VERSION: &'static str;

    type Event: for<'de> Deserialize<'de> + Send + 'static;

    fn event_to_log_record(event: &Self::Event, raw_json: &serde_json::Value) -> LogRecord;
}
