use std::sync::Mutex;

use opentelemetry_proto::tonic::collector::logs::v1::{
    ExportLogsServiceRequest, ExportLogsServiceResponse, logs_service_server::LogsService,
};
use tonic::{Request, Response, Status};
use wal::WalWriter;

use crate::ledger_sender::LedgerSender;

pub struct NetdataLogsService {
    wal: Mutex<WalWriter>,
    sender: LedgerSender,
}

impl NetdataLogsService {
    pub fn new(wal: WalWriter, sender: LedgerSender) -> Self {
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
        let mut req = request.into_inner();

        let (ipc_bytes, count) = crate::arrow_bridge::encode_logs_arrow(&mut req).map_err(|e| {
            tracing::error!(%e, "failed to encode Arrow");
            Status::internal("Arrow encode error")
        })?;

        let mut wal = self.wal.lock().unwrap();
        wal.write_frame(&ipc_bytes, count).map_err(|e| {
            tracing::error!(%e, "failed to write WAL entry");
            Status::internal("WAL write error")
        })?;

        wal.sync().map_err(|e| {
            tracing::error!(%e, "failed to sync WAL");
            Status::internal("WAL sync error")
        })?;

        let events = wal.take_events();
        self.sender.send_events(events);

        Ok(Response::new(ExportLogsServiceResponse {
            partial_success: None,
        }))
    }
}
