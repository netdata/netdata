use super::super::super::DynamicRoutingPeerKey;
use super::super::model::ObserveTaskHandle;
use super::super::observe::{start_observe_task, stop_observe_task};
use super::instance::refresh_instance_once;
use crate::ingest::IngestMetrics;
use crate::plugin_config::RoutingDynamicBiorisRisInstanceConfig;
use std::collections::{HashMap, HashSet};
use std::sync::Arc;
use std::sync::atomic::Ordering;
use std::time::Duration;
use tokio::time::MissedTickBehavior;
use tokio_util::sync::CancellationToken;

pub(in crate::routing::bioris::runtime) async fn run_instance_loop(
    instance: RoutingDynamicBiorisRisInstanceConfig,
    timeout: Duration,
    refresh: Duration,
    refresh_timeout: Duration,
    runtime: super::super::super::DynamicRoutingRuntime,
    metrics: Arc<IngestMetrics>,
    shutdown: CancellationToken,
) {
    let regular_interval = refresh.max(Duration::from_secs(10));
    let mut retry_interval = (regular_interval / 10).max(Duration::from_secs(1));
    let mut next_wait = Duration::ZERO;
    let mut ticker = tokio::time::interval(regular_interval);
    ticker.set_missed_tick_behavior(MissedTickBehavior::Skip);
    let mut known_peers: HashSet<DynamicRoutingPeerKey> = HashSet::new();
    let mut observe_tasks: HashMap<DynamicRoutingPeerKey, ObserveTaskHandle> = HashMap::new();

    loop {
        if next_wait.is_zero() {
            match refresh_instance_once(
                &instance,
                timeout,
                refresh_timeout,
                &runtime,
                &metrics,
                &shutdown,
            )
            .await
            {
                Ok(expected_targets) => {
                    metrics
                        .bioris_refresh_success
                        .fetch_add(1, Ordering::Relaxed);
                    for (peer, target) in &expected_targets {
                        if observe_tasks.contains_key(peer) {
                            continue;
                        }
                        let task = start_observe_task(
                            instance.clone(),
                            target.clone(),
                            peer.clone(),
                            timeout,
                            runtime.clone(),
                            Arc::clone(&metrics),
                            shutdown.clone(),
                        );
                        observe_tasks.insert(peer.clone(), task);
                    }

                    let expected_peers: HashSet<DynamicRoutingPeerKey> =
                        expected_targets.keys().cloned().collect();
                    for stale_peer in known_peers.difference(&expected_peers) {
                        if let Some(task) = observe_tasks.remove(stale_peer) {
                            stop_observe_task(stale_peer, task, &metrics).await;
                        }
                        runtime.clear_peer(stale_peer);
                    }
                    known_peers = expected_peers;
                    next_wait = regular_interval;
                    retry_interval = (regular_interval / 10).max(Duration::from_secs(1));
                }
                Err(err) => {
                    metrics
                        .bioris_refresh_errors
                        .fetch_add(1, Ordering::Relaxed);
                    tracing::warn!(
                        "BioRIS refresh failed for {}: {:#}",
                        instance.grpc_addr,
                        err
                    );
                    next_wait = retry_interval;
                    retry_interval = std::cmp::min(retry_interval * 2, regular_interval);
                }
            }
        }

        tokio::select! {
            _ = shutdown.cancelled() => break,
            _ = tokio::time::sleep(next_wait) => {
                next_wait = Duration::ZERO;
            }
            _ = ticker.tick() => {
                if next_wait > regular_interval {
                    next_wait = regular_interval;
                }
            }
        }
    }

    for (peer, task) in observe_tasks {
        stop_observe_task(&peer, task, &metrics).await;
    }
}
