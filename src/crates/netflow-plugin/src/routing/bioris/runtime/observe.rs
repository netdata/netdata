use super::super::client::connect_bioris_client;
use super::super::route::{route_to_update, route_withdraw_keys};
use super::super::{DynamicRoutingPeerKey, DynamicRoutingRuntime, ObserveRibRequest, RibUpdate};
use super::model::{AfiSafi, ObserveTarget, ObserveTaskHandle};
use crate::ingest::IngestMetrics;
use crate::plugin_config::RoutingDynamicBiorisRisInstanceConfig;
use anyhow::{Context, Result};
use std::sync::Arc;
use std::sync::atomic::Ordering;
use std::time::Duration;
use tokio_stream::StreamExt;
use tokio_util::sync::CancellationToken;

const OBSERVE_RETRY_MIN_INTERVAL: Duration = Duration::from_secs(1);
const OBSERVE_RETRY_MAX_INTERVAL: Duration = Duration::from_secs(30);
const OBSERVE_TASK_STOP_TIMEOUT: Duration = Duration::from_secs(2);

pub(super) fn start_observe_task(
    instance: RoutingDynamicBiorisRisInstanceConfig,
    target: ObserveTarget,
    peer: DynamicRoutingPeerKey,
    timeout: Duration,
    runtime: DynamicRoutingRuntime,
    metrics: Arc<IngestMetrics>,
    shutdown: CancellationToken,
) -> ObserveTaskHandle {
    metrics
        .bioris_observe_stream_starts
        .fetch_add(1, Ordering::Relaxed);
    metrics
        .bioris_observe_streams_active
        .fetch_add(1, Ordering::Relaxed);
    let cancel = CancellationToken::new();
    let task_cancel = cancel.clone();
    let handle = tokio::spawn(async move {
        run_observe_loop(
            instance,
            target,
            peer,
            timeout,
            runtime,
            metrics,
            shutdown,
            task_cancel,
        )
        .await;
    });
    ObserveTaskHandle { cancel, handle }
}

pub(super) async fn stop_observe_task(
    peer: &DynamicRoutingPeerKey,
    task: ObserveTaskHandle,
    metrics: &IngestMetrics,
) {
    task.cancel.cancel();
    match tokio::time::timeout(OBSERVE_TASK_STOP_TIMEOUT, task.handle).await {
        Ok(Ok(())) => {}
        Ok(Err(err)) if !err.is_cancelled() => {
            tracing::warn!(
                "BioRIS observe task join error for {}: {}",
                peer.peer_id,
                err
            );
        }
        Ok(Err(_)) => {}
        Err(_) => {
            tracing::warn!(
                "BioRIS observe task stop timed out for {}, aborting task",
                peer.peer_id
            );
        }
    }
    let _ = metrics.bioris_observe_streams_active.fetch_update(
        Ordering::Relaxed,
        Ordering::Relaxed,
        |current| Some(current.saturating_sub(1)),
    );
}

async fn run_observe_loop(
    instance: RoutingDynamicBiorisRisInstanceConfig,
    target: ObserveTarget,
    peer: DynamicRoutingPeerKey,
    timeout: Duration,
    runtime: DynamicRoutingRuntime,
    metrics: Arc<IngestMetrics>,
    shutdown: CancellationToken,
    task_cancel: CancellationToken,
) {
    let mut retry_interval = OBSERVE_RETRY_MIN_INTERVAL;
    loop {
        if shutdown.is_cancelled() || task_cancel.is_cancelled() {
            return;
        }

        match observe_peer_stream(
            &instance,
            &target,
            &peer,
            timeout,
            &runtime,
            &shutdown,
            &task_cancel,
        )
        .await
        {
            Ok(()) => {
                retry_interval = OBSERVE_RETRY_MIN_INTERVAL;
            }
            Err(err) => {
                metrics
                    .bioris_observe_stream_errors
                    .fetch_add(1, Ordering::Relaxed);
                tracing::warn!(
                    "BioRIS observe stream failed for {} router {} afi {}: {:#}",
                    instance.grpc_addr,
                    target.router,
                    target.afisafi.as_str(),
                    err
                );
            }
        }

        tokio::select! {
            _ = shutdown.cancelled() => return,
            _ = task_cancel.cancelled() => return,
            _ = tokio::time::sleep(retry_interval) => {}
        }
        metrics
            .bioris_observe_stream_reconnects
            .fetch_add(1, Ordering::Relaxed);
        retry_interval = std::cmp::min(retry_interval * 2, OBSERVE_RETRY_MAX_INTERVAL);
    }
}

async fn observe_peer_stream(
    instance: &RoutingDynamicBiorisRisInstanceConfig,
    target: &ObserveTarget,
    peer: &DynamicRoutingPeerKey,
    timeout: Duration,
    runtime: &DynamicRoutingRuntime,
    shutdown: &CancellationToken,
    task_cancel: &CancellationToken,
) -> Result<()> {
    let mut client = connect_bioris_client(instance, timeout).await?;
    let request = ObserveRibRequest {
        router: target.router.clone(),
        vrf_id: instance.vrf_id,
        vrf: instance.vrf.clone(),
        afisafi: target.afisafi.as_observe_proto(),
        allow_unready_rib: true,
    };
    let response = tokio::time::timeout(timeout, client.observe_rib(request))
        .await
        .with_context(|| {
            format!(
                "BioRIS ObserveRIB request timed out for router {} afi {}",
                target.router,
                target.afisafi.as_str()
            )
        })?
        .with_context(|| {
            format!(
                "BioRIS ObserveRIB request failed for router {} afi {}",
                target.router,
                target.afisafi.as_str()
            )
        })?;
    let mut stream = response.into_inner();

    loop {
        tokio::select! {
            _ = shutdown.cancelled() => return Ok(()),
            _ = task_cancel.cancelled() => return Ok(()),
            next = stream.next() => {
                match next {
                    Some(Ok(update)) => {
                        apply_observe_update(update, peer, target.afisafi, runtime);
                    }
                    Some(Err(err)) => {
                        return Err(anyhow::anyhow!(
                            "BioRIS ObserveRIB stream read failed for router {} afi {}: {}",
                            target.router,
                            target.afisafi.as_str(),
                            err
                        ));
                    }
                    None => return Ok(()),
                }
            }
        }
    }
}

fn apply_observe_update(
    update: RibUpdate,
    peer: &DynamicRoutingPeerKey,
    afisafi: AfiSafi,
    runtime: &DynamicRoutingRuntime,
) {
    if update.end_of_rib {
        return;
    }
    let Some(route) = update.route else {
        return;
    };

    if update.advertisement {
        if let Some(update) = route_to_update(route, peer, afisafi) {
            runtime.upsert(update);
        }
        return;
    }

    if let Some((prefix, route_keys)) = route_withdraw_keys(&route, afisafi) {
        for route_key in route_keys {
            runtime.withdraw(peer, prefix, &route_key);
        }
    }
}
