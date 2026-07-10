use std::future::Future;
use std::time::Duration;

use tokio::sync::mpsc;
use tracing::{error, info, warn};

use crate::Source;
use crate::args::CommonArgs;
use crate::sender::{OtelConfig, Sender};

pub async fn run<S, F, Fut>(name: &str, common: &CommonArgs, mut connect: F) -> anyhow::Result<()>
where
    S: Source,
    F: FnMut(mpsc::Sender<(S::Event, serde_json::Value)>) -> Fut + Send + 'static,
    Fut: Future<Output = anyhow::Result<()>> + Send,
{
    let (record_tx, record_rx) = mpsc::channel(1000);

    let flush_interval = Duration::from_millis(common.flush_interval_ms);
    let config = OtelConfig {
        endpoint: common.otel_endpoint.clone(),
        batch_size: common.batch_size,
        flush_interval,
        tenant_id: common.tenant_id.clone(),
        service_name: S::SERVICE_NAME.to_string(),
        service_namespace: None,
        scope_name: S::SCOPE_NAME,
        scope_version: S::SCOPE_VERSION,
    };
    let mut sender = Sender::new(config, record_rx).await?;
    let _sender_handle = tokio::spawn(async move { sender.run().await });

    let mut backoff = Duration::from_secs(1);
    const MAX_BACKOFF: Duration = Duration::from_secs(60);

    loop {
        let (event_tx, mut event_rx) = mpsc::channel(1000);
        let mapper_record_tx = record_tx.clone();

        let mapper_handle = tokio::spawn(async move {
            while let Some((event, raw_json)) = event_rx.recv().await {
                let record = S::event_to_log_record(&event, &raw_json);
                if mapper_record_tx.send(record).await.is_err() {
                    info!("Sender dropped, stopping mapper");
                    break;
                }
            }
        });

        match connect(event_tx).await {
            Ok(()) => {
                backoff = Duration::from_secs(1);
                warn!("{name} disconnected, reconnecting");
            }
            Err(e) => {
                error!(
                    error = %e,
                    backoff_secs = backoff.as_secs(),
                    "{name} connection error, retrying"
                );
                tokio::time::sleep(backoff).await;
                backoff = (backoff * 2).min(MAX_BACKOFF);
            }
        }

        let _ = mapper_handle.await;
    }
}
