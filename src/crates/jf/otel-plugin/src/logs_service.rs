use anyhow::{Context, Result};
use flatten_otel::json_from_export_logs_service_request;
use journal_log::{JournalLog, JournalLogConfig, RetentionPolicy, RotationPolicy};
use opentelemetry_proto::tonic::collector::logs::v1::{
    logs_service_server::LogsService, ExportLogsServiceRequest, ExportLogsServiceResponse,
};
use serde_json::Value;
use std::sync::{Arc, Mutex};
use tonic::{Request, Response, Status};

use crate::plugin_config::PluginConfig;

pub struct NetdataLogsService {
    journal_log: Arc<Mutex<JournalLog>>,
}

impl NetdataLogsService {
    pub fn new(plugin_config: PluginConfig) -> Result<Self> {
        let logs_config = plugin_config.logs;

        let rotation_policy = RotationPolicy::default()
            .with_size_of_journal_file(logs_config.size_of_journal_file.as_u64())
            .with_duration_of_journal_file(logs_config.duration_of_journal_file);

        let retention_policy = RetentionPolicy::default()
            .with_number_of_journal_files(logs_config.number_of_journal_files)
            .with_size_of_journal_files(logs_config.size_of_journal_files.as_u64())
            .with_duration_of_journal_files(logs_config.duration_of_journal_files);

        let journal_config = JournalLogConfig::new(&logs_config.journal_dir)
            .with_rotation_policy(rotation_policy)
            .with_retention_policy(retention_policy);

        let journal_log = Arc::new(Mutex::new(JournalLog::new(journal_config).with_context(
            || {
                format!(
                    "Failed to create journal log for directory: {}",
                    logs_config.journal_dir
                )
            },
        )?));
        Ok(NetdataLogsService { journal_log })
    }

    fn json_to_entry_data(&self, json_value: &Value) -> Vec<Vec<u8>> {
        let mut entry_data = Vec::new();

        if let Value::Object(obj) = json_value {
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

        entry_data
    }
}

#[tonic::async_trait]
impl LogsService for NetdataLogsService {
    async fn export(
        &self,
        request: Request<ExportLogsServiceRequest>,
    ) -> Result<Response<ExportLogsServiceResponse>, Status> {
        let req = request.into_inner();

        let json_array = json_from_export_logs_service_request(&req);

        if let Value::Array(entries) = json_array {
            for entry in entries {
                let entry_data = self.json_to_entry_data(&entry);
                if !entry_data.is_empty() {
                    let entry_refs: Vec<&[u8]> = entry_data.iter().map(|v| v.as_slice()).collect();
                    if let Err(e) = self.journal_log.lock().unwrap().write_entry(&entry_refs) {
                        eprintln!("Failed to write log entry: {}", e);
                        return Err(Status::internal(format!(
                            "Failed to write log entry: {}",
                            e
                        )));
                    }
                }
            }
        }

        let reply = ExportLogsServiceResponse {
            partial_success: None,
        };

        Ok(Response::new(reply))
    }
}
