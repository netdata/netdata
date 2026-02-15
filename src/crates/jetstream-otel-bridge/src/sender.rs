use std::time::Duration;

use opentelemetry_proto::tonic::{
    collector::logs::v1::logs_service_client::LogsServiceClient, logs::v1::LogRecord,
};
use tokio::sync::mpsc;
use tokio::time;
use tonic::transport::Channel;
use tracing::{error, info};

use crate::mapping::build_export_request;

pub struct Sender {
    client: LogsServiceClient<Channel>,
    batch: Vec<LogRecord>,
    batch_size: usize,
    flush_interval: Duration,
    rx: mpsc::Receiver<LogRecord>,
}

impl Sender {
    pub async fn new(
        endpoint: &str,
        batch_size: usize,
        flush_interval: Duration,
        rx: mpsc::Receiver<LogRecord>,
    ) -> anyhow::Result<Self> {
        info!("Connecting to OTel endpoint: {}", endpoint);
        let channel = Channel::from_shared(endpoint.to_string())?
            .connect()
            .await?;
        let client = LogsServiceClient::new(channel);
        info!("Connected to OTel endpoint");

        Ok(Self {
            client,
            batch: Vec::with_capacity(batch_size),
            batch_size,
            flush_interval,
            rx,
        })
    }

    pub async fn run(&mut self) {
        let mut flush_timer = time::interval(self.flush_interval);
        // The first tick completes immediately; consume it so the first real
        // flush happens after one full interval has elapsed.
        flush_timer.tick().await;

        loop {
            tokio::select! {
                record = self.rx.recv() => {
                    match record {
                        Some(record) => {
                            self.batch.push(record);
                            if self.batch.len() >= self.batch_size {
                                self.flush().await;
                                flush_timer.reset();
                            }
                        }
                        None => {
                            // Channel closed — flush remaining and exit.
                            if !self.batch.is_empty() {
                                self.flush().await;
                            }
                            info!("Event channel closed, sender shutting down");
                            return;
                        }
                    }
                }
                _ = flush_timer.tick() => {
                    if !self.batch.is_empty() {
                        self.flush().await;
                    }
                }
            }
        }
    }

    async fn flush(&mut self) {
        let records: Vec<LogRecord> = self.batch.drain(..).collect();
        let count = records.len();
        let request = build_export_request(records);

        match self.client.export(request).await {
            Ok(_response) => {
                info!(count, "Flushed batch");
            }
            Err(e) => {
                error!(count, error = %e, "Failed to send batch");
            }
        }
    }
}
