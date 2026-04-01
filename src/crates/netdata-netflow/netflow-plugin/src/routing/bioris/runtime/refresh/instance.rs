use super::super::super::client::{bioris_peer_key, connect_bioris_client, parse_router_ip};
use super::super::super::{DynamicRoutingPeerKey, DynamicRoutingRuntime, GetRoutersRequest};
use super::super::model::{AfiSafi, ObserveTarget};
use super::dump::dump_peer_routes;
use crate::ingest::IngestMetrics;
use crate::plugin_config::RoutingDynamicBiorisRisInstanceConfig;
use anyhow::{Context, Result};
use std::collections::HashMap;
use std::sync::atomic::Ordering;
use std::time::Duration;
use tokio_util::sync::CancellationToken;

pub(super) async fn refresh_instance_once(
    instance: &RoutingDynamicBiorisRisInstanceConfig,
    timeout: Duration,
    refresh_timeout: Duration,
    runtime: &DynamicRoutingRuntime,
    metrics: &IngestMetrics,
    shutdown: &CancellationToken,
) -> Result<HashMap<DynamicRoutingPeerKey, ObserveTarget>> {
    let mut client = connect_bioris_client(instance, timeout).await?;
    let routers_response = tokio::time::timeout(timeout, client.get_routers(GetRoutersRequest {}))
        .await
        .context("BioRIS GetRouters request timed out")?
        .context("BioRIS GetRouters request failed")?;

    let mut expected_targets: HashMap<DynamicRoutingPeerKey, ObserveTarget> = HashMap::new();
    for router in routers_response.into_inner().routers {
        if shutdown.is_cancelled() {
            return Ok(expected_targets);
        }

        let Some(router_ip) = parse_router_ip(&router.address) else {
            tracing::warn!(
                "skipping BioRIS router '{}' from {}: invalid address '{}'",
                router.sys_name,
                instance.grpc_addr,
                router.address
            );
            continue;
        };
        for afisafi in [AfiSafi::Ipv4Unicast, AfiSafi::Ipv6Unicast] {
            let routing_key = bioris_peer_key(instance, router_ip, afisafi);
            expected_targets.insert(
                routing_key.clone(),
                ObserveTarget {
                    router: router.address.clone(),
                    afisafi,
                },
            );
            if let Err(err) = dump_peer_routes(
                &mut client,
                instance,
                &router.address,
                afisafi,
                refresh_timeout,
                runtime,
                routing_key,
            )
            .await
            {
                metrics.bioris_dump_errors.fetch_add(1, Ordering::Relaxed);
                tracing::warn!(
                    "BioRIS dump failed for {} router {} afi {}: {:#}",
                    instance.grpc_addr,
                    router.address,
                    afisafi.as_str(),
                    err
                );
            } else {
                metrics.bioris_dump_success.fetch_add(1, Ordering::Relaxed);
            }
        }
    }

    Ok(expected_targets)
}
