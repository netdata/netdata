use std::collections::HashMap;
use std::sync::Mutex;

use opentelemetry_proto::tonic::collector::logs::v1::{
    ExportLogsServiceRequest, ExportLogsServiceResponse, logs_service_server::LogsService,
};
use opentelemetry_proto::tonic::common::v1::any_value::Value;
use opentelemetry_proto::tonic::logs::v1::ResourceLogs;
use tonic::{Request, Response, Status};
use wal::WalWriterMap;

use crate::ledger_sender::LedgerSender;

/// Extract `service.namespace` and `service.name` from a `ResourceLogs`
/// entry's resource attributes and compute the namespace hash.
fn ns_hash_from_resource(rl: &ResourceLogs) -> u64 {
    let attrs = match rl.resource.as_ref() {
        Some(r) => &r.attributes,
        None => return 0,
    };

    let mut namespace = None;
    let mut name = None;

    for kv in attrs {
        match kv.key.as_str() {
            "service.namespace" => {
                if let Some(Value::StringValue(s)) =
                    kv.value.as_ref().and_then(|v| v.value.as_ref())
                {
                    namespace = Some(s.as_str());
                }
            }
            "service.name" => {
                if let Some(Value::StringValue(s)) =
                    kv.value.as_ref().and_then(|v| v.value.as_ref())
                {
                    name = Some(s.as_str());
                }
            }
            _ => {}
        }
    }

    wal::compute_ns_hash(namespace, name)
}

pub struct NetdataLogsService {
    wal: Mutex<WalWriterMap>,
    sender: LedgerSender,
}

impl NetdataLogsService {
    pub fn new(wal: WalWriterMap, sender: LedgerSender) -> Self {
        Self {
            wal: Mutex::new(wal),
            sender,
        }
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

        // Group ResourceLogs by ns_hash.
        let mut groups: HashMap<u64, Vec<ResourceLogs>> = HashMap::new();
        for rl in req.resource_logs {
            let ns_hash = ns_hash_from_resource(&rl);
            groups.entry(ns_hash).or_default().push(rl);
        }

        let mut wal = self.wal.lock().unwrap();

        for (ns_hash, resource_logs) in groups {
            let mut sub_req = ExportLogsServiceRequest { resource_logs };
            let (ipc_bytes, count) =
                crate::arrow_bridge::encode_logs_arrow(&mut sub_req).map_err(|e| {
                    tracing::error!(%e, "failed to encode Arrow");
                    Status::internal("Arrow encode error")
                })?;

            let writer = wal.get_or_create(ns_hash);
            writer.write_frame(&ipc_bytes, count).map_err(|e| {
                tracing::error!(%e, "failed to write WAL entry");
                Status::internal("WAL write error")
            })?;
        }

        wal.sync_all().map_err(|e| {
            tracing::error!(%e, "failed to sync WAL");
            Status::internal("WAL sync error")
        })?;

        let events = wal.take_all_events();
        self.sender.send_events(events);

        Ok(Response::new(ExportLogsServiceResponse {
            partial_success: None,
        }))
    }
}
