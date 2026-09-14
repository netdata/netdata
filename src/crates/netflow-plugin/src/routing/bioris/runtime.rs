use crate::ingest::IngestMetrics;
use crate::plugin_config::RoutingDynamicBiorisConfig;
use anyhow::Result;
use std::sync::Arc;
use tokio::task::JoinSet;
use tokio_util::sync::CancellationToken;

mod model;
mod observe;
mod refresh;

pub(in crate::routing::bioris) use model::AfiSafi;
use refresh::run_instance_loop;

pub(crate) async fn run_bioris_listener(
    config: RoutingDynamicBiorisConfig,
    runtime: super::DynamicRoutingRuntime,
    metrics: Arc<IngestMetrics>,
    shutdown: CancellationToken,
) -> Result<()> {
    if !config.enabled {
        return Ok(());
    }
    if config.ris_instances.is_empty() {
        tracing::warn!("BioRIS routing is enabled but no RIS instances are configured");
        return Ok(());
    }

    let mut tasks = JoinSet::new();
    for instance in config.ris_instances {
        let instance_runtime = runtime.clone();
        let instance_metrics = Arc::clone(&metrics);
        let instance_shutdown = shutdown.clone();
        let timeout = config.timeout;
        let refresh = config.refresh;
        let refresh_timeout = config.refresh_timeout;
        tasks.spawn(async move {
            run_instance_loop(
                instance,
                timeout,
                refresh,
                refresh_timeout,
                instance_runtime,
                instance_metrics,
                instance_shutdown,
            )
            .await;
        });
    }

    shutdown.cancelled().await;
    tasks.abort_all();
    while let Some(result) = tasks.join_next().await {
        if let Err(err) = result
            && !err.is_cancelled()
        {
            tracing::warn!("BioRIS routing task join error: {}", err);
        }
    }

    Ok(())
}
