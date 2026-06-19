use super::super::super::route::route_to_update;
use super::super::super::{
    DumpRibRequest, DynamicRoutingPeerKey, DynamicRoutingRuntime, RoutingInformationServiceClient,
};
use super::super::model::AfiSafi;
use crate::plugin_config::RoutingDynamicBiorisRisInstanceConfig;
use anyhow::{Context, Result};
use std::time::Duration;
use tokio_stream::StreamExt;
use tonic::transport::Channel;

pub(super) async fn dump_peer_routes(
    client: &mut RoutingInformationServiceClient<Channel>,
    instance: &RoutingDynamicBiorisRisInstanceConfig,
    router: &str,
    afisafi: AfiSafi,
    refresh_timeout: Duration,
    runtime: &DynamicRoutingRuntime,
    peer: DynamicRoutingPeerKey,
) -> Result<()> {
    let request = DumpRibRequest {
        router: router.to_string(),
        vrf_id: instance.vrf_id,
        vrf: instance.vrf.clone(),
        afisafi: afisafi.as_proto(),
        filter: None,
    };

    let mut updates = Vec::new();
    let response = tokio::time::timeout(refresh_timeout, client.dump_rib(request))
        .await
        .with_context(|| {
            format!(
                "BioRIS DumpRIB request timed out for router {} afi {}",
                router,
                afisafi.as_str()
            )
        })?
        .with_context(|| {
            format!(
                "BioRIS DumpRIB request failed for router {} afi {}",
                router,
                afisafi.as_str()
            )
        })?;
    let mut stream = response.into_inner();

    loop {
        let next = tokio::time::timeout(refresh_timeout, stream.next())
            .await
            .with_context(|| {
                format!(
                    "BioRIS DumpRIB stream timed out for router {} afi {}",
                    router,
                    afisafi.as_str()
                )
            })?;
        let Some(next) = next else {
            break;
        };
        let reply = next.with_context(|| {
            format!(
                "BioRIS DumpRIB stream read failed for router {} afi {}",
                router,
                afisafi.as_str()
            )
        })?;
        if let Some(route) = reply.route
            && let Some(update) = route_to_update(route, &peer, afisafi)
        {
            updates.push(update);
        }
    }

    runtime.clear_peer(&peer);
    for update in updates {
        runtime.upsert(update);
    }

    Ok(())
}
