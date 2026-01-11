use anyhow::{Context, Result};
use flatten_otel::json_from_export_logs_service_request;
use journal_common::load_machine_id;
use journal_log_writer::{Config, Log, RetentionPolicy, RotationPolicy};
use journal_registry::Origin;
use opentelemetry_proto::tonic::collector::logs::v1::{
    ExportLogsServiceRequest, ExportLogsServiceResponse, logs_service_server::LogsService,
};
use serde_json::Value;
use std::sync::{Arc, Mutex};
use tonic::{Request, Response, Status};

use crate::plugin_config::PluginConfig;

pub struct NetdataLogsService {
    log: Arc<Mutex<Log>>,
    store_otlp_json: bool,
}

impl NetdataLogsService {
    pub fn new(plugin_config: PluginConfig) -> Result<Self> {
        let logs_config = plugin_config.logs;

        let rotation_policy = RotationPolicy::default()
            .with_size_of_journal_file(logs_config.size_of_journal_file.as_u64())
            .with_duration_of_journal_file(logs_config.duration_of_journal_file)
            .with_number_of_entries(logs_config.entries_of_journal_file);

        let retention_policy = RetentionPolicy::default()
            .with_number_of_journal_files(logs_config.number_of_journal_files)
            .with_size_of_journal_files(logs_config.size_of_journal_files.as_u64())
            .with_duration_of_journal_files(logs_config.duration_of_journal_files);

        let machine_id = load_machine_id()?;
        let origin = Origin {
            machine_id: Some(machine_id),
            namespace: None,
            source: journal_registry::Source::System,
        };

        let path = std::path::Path::new(&logs_config.journal_dir);
        let journal_config = Config::new(origin, rotation_policy, retention_policy);

        let journal_log = Arc::new(Mutex::new(Log::new(path, journal_config).with_context(
            || {
                format!(
                    "Failed to create journal log for directory: {}",
                    logs_config.journal_dir
                )
            },
        )?));
        Ok(NetdataLogsService {
            log: journal_log,
            store_otlp_json: logs_config.store_otlp_json,
        })
    }

    fn extract_timestamp_for_sorting(json_value: &Value) -> u64 {
        if let Value::Object(obj) = json_value {
            // Extract timestamp for sorting (same logic as json_to_entry_data)
            // Per OTLP spec: Use time_unix_nano if present (non-zero), otherwise use observed_time_unix_nano
            let time_unix_nano = obj
                .get("log.time_unix_nano")
                .and_then(|v| v.as_u64())
                .filter(|&t| t != 0);

            let observed_time_unix_nano = obj
                .get("log.observed_time_unix_nano")
                .and_then(|v| v.as_u64())
                .filter(|&t| t != 0);

            time_unix_nano.or(observed_time_unix_nano).unwrap_or(0)
        } else {
            0
        }
    }

    fn json_to_entry_data(&self, json_value: &Value) -> (Vec<Vec<u8>>, Option<u64>) {
        let mut entry_data = Vec::new();
        let mut source_timestamp_usec = None;

        if let Value::Object(obj) = json_value {
            // Extract timestamps for source realtime timestamp
            // Per OTLP spec: Use time_unix_nano if present (non-zero), otherwise use observed_time_unix_nano
            let time_unix_nano = obj
                .get("log.time_unix_nano")
                .and_then(|v| v.as_u64())
                .filter(|&t| t != 0);

            let observed_time_unix_nano = obj
                .get("log.observed_time_unix_nano")
                .and_then(|v| v.as_u64())
                .filter(|&t| t != 0);

            // Convert from nanoseconds to microseconds (systemd journal uses microseconds)
            source_timestamp_usec = time_unix_nano
                .or(observed_time_unix_nano)
                .map(|nano| nano / 1000);

            // Add OTLP_JSON field containing the complete JSON representation if enabled
            // This preserves the full original message for debugging and reprocessing
            if self.store_otlp_json {
                if let Ok(json_str) = serde_json::to_string(json_value) {
                    let kv_pair = format!("OTLP_JSON={}", json_str);
                    entry_data.push(kv_pair.into_bytes());
                }
            }

            for (key, value) in obj {
                let value_str = match value {
                    Value::String(s) => s.clone(),
                    Value::Number(n) => n.to_string(),
                    Value::Bool(b) => b.to_string(),
                    Value::Null => "null".to_string(),
                    _ => serde_json::to_string(value).unwrap_or_default(),
                };

                let kv_pair = format!("{}={}", key, value_str);
                entry_data.push(kv_pair.into_bytes());
            }
        }

        (entry_data, source_timestamp_usec)
    }
}

#[tonic::async_trait]
impl LogsService for NetdataLogsService {
    #[tracing::instrument(skip_all, fields(received_logs))]
    async fn export(
        &self,
        request: Request<ExportLogsServiceRequest>,
    ) -> Result<Response<ExportLogsServiceResponse>, Status> {
        let req = request.into_inner();

        let json_array = json_from_export_logs_service_request(&req);

        if let Value::Array(mut entries) = json_array {
            tracing::Span::current().record("received_logs", entries.len());

            // Sort entries by their creation timestamp before writing to journal
            // This ensures journal entries are written in chronological order, which:
            // - Optimizes journal file structure and indexing
            // - Improves query performance
            // - Enhances compression efficiency
            entries.sort_by_key(Self::extract_timestamp_for_sorting);

            for entry in entries {
                let (entry_data, source_timestamp_usec) = self.json_to_entry_data(&entry);

                if entry_data.is_empty() {
                    continue;
                }

                let entry_refs: Vec<&[u8]> = entry_data.iter().map(|v| v.as_slice()).collect();
                if let Err(e) = self.log.lock().unwrap().write_entry(&entry_refs, source_timestamp_usec) {
                    eprintln!("Failed to write log entry: {}", e);
                    return Err(Status::internal(format!(
                        "Failed to write log entry: {}",
                        e
                    )));
                }
            }

            if let Err(e) = self.log.lock().unwrap().sync() {
                eprintln!("Failed to sync journal file: {}", e);
                return Err(Status::internal(format!(
                    "Failed to sync journal file: {}",
                    e
                )));
            }
        }

        let reply = ExportLogsServiceResponse {
            partial_success: None,
        };

        Ok(Response::new(reply))
    }
}
