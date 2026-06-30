use super::{runtime::AfiSafi, *};

pub(super) async fn connect_bioris_client(
    instance: &RoutingDynamicBiorisRisInstanceConfig,
    timeout: Duration,
) -> Result<RoutingInformationServiceClient<Channel>> {
    let endpoint_uri = build_endpoint_uri(instance);
    let mut endpoint = Endpoint::from_shared(endpoint_uri.clone())
        .with_context(|| format!("invalid BioRIS endpoint URI: {}", endpoint_uri))?;
    endpoint = endpoint.connect_timeout(timeout);
    if instance.grpc_secure {
        endpoint = endpoint
            .tls_config(ClientTlsConfig::new().with_native_roots())
            .context("failed to configure BioRIS TLS settings")?;
    }
    let channel = endpoint
        .connect()
        .await
        .with_context(|| format!("failed to connect to BioRIS endpoint {}", endpoint_uri))?;
    Ok(RoutingInformationServiceClient::new(channel))
}

pub(super) fn build_endpoint_uri(instance: &RoutingDynamicBiorisRisInstanceConfig) -> String {
    if instance.grpc_addr.contains("://") {
        instance.grpc_addr.clone()
    } else if instance.grpc_secure {
        format!("https://{}", instance.grpc_addr)
    } else {
        format!("http://{}", instance.grpc_addr)
    }
}

pub(super) fn parse_router_ip(value: &str) -> Option<IpAddr> {
    if let Ok(ip) = value.trim().parse::<IpAddr>() {
        return Some(ip);
    }
    value
        .trim()
        .parse::<SocketAddr>()
        .ok()
        .map(|addr| addr.ip())
}

pub(super) fn bioris_peer_key(
    instance: &RoutingDynamicBiorisRisInstanceConfig,
    router_ip: IpAddr,
    afisafi: AfiSafi,
) -> DynamicRoutingPeerKey {
    DynamicRoutingPeerKey {
        exporter: SocketAddr::new(router_ip, 0),
        session_id: 0,
        peer_id: format!(
            "bioris|grpc={}|router={}|vrf_id={}|vrf={}|afi={}",
            instance.grpc_addr,
            router_ip,
            instance.vrf_id,
            instance.vrf,
            afisafi.as_str()
        ),
    }
}
