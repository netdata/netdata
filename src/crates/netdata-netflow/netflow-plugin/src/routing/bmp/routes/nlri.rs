use super::*;

#[derive(Debug, Clone)]
pub(crate) struct NlriRoute {
    pub(crate) prefix: IpNet,
    pub(crate) route_key: String,
    pub(crate) rd: u64,
}

pub(crate) fn ipv4_unicast_to_prefix(
    value: &netgauze_bgp_pkt::nlri::Ipv4UnicastAddress,
) -> (IpNet, Option<u32>) {
    (IpNet::V4(value.network().address()), value.path_id())
}

pub(crate) fn mp_reach_to_routes(
    reach: &MpReach,
    peer_rd: u64,
) -> (Option<IpAddr>, Vec<NlriRoute>) {
    match reach {
        MpReach::Ipv4Unicast { next_hop, nlri, .. } => {
            (Some(*next_hop), ipv4_unicast_routes(nlri, peer_rd))
        }
        MpReach::Ipv6Unicast {
            next_hop_global,
            nlri,
            ..
        } => (
            Some(IpAddr::V6(*next_hop_global)),
            ipv6_unicast_routes(nlri, peer_rd),
        ),
        MpReach::Ipv4NlriMplsLabels { next_hop, nlri, .. } => {
            (Some(*next_hop), ipv4_mpls_label_routes(nlri, peer_rd))
        }
        MpReach::Ipv6NlriMplsLabels { next_hop, nlri, .. } => {
            (Some(*next_hop), ipv6_mpls_label_routes(nlri, peer_rd))
        }
        MpReach::Ipv4MplsVpnUnicast { next_hop, nlri } => {
            (Some(next_hop.next_hop()), ipv4_mpls_vpn_routes(nlri))
        }
        MpReach::Ipv6MplsVpnUnicast { next_hop, nlri } => {
            (Some(next_hop.next_hop()), ipv6_mpls_vpn_routes(nlri))
        }
        MpReach::L2Evpn { next_hop, nlri } => (Some(*next_hop), l2_evpn_routes(nlri)),
        _ => (None, Vec::new()),
    }
}

pub(crate) fn mp_unreach_to_routes(unreach: &MpUnreach, peer_rd: u64) -> Vec<NlriRoute> {
    match unreach {
        MpUnreach::Ipv4Unicast { nlri } => ipv4_unicast_routes(nlri, peer_rd),
        MpUnreach::Ipv6Unicast { nlri } => ipv6_unicast_routes(nlri, peer_rd),
        MpUnreach::Ipv4NlriMplsLabels { nlri } => ipv4_mpls_label_routes(nlri, peer_rd),
        MpUnreach::Ipv6NlriMplsLabels { nlri } => ipv6_mpls_label_routes(nlri, peer_rd),
        MpUnreach::Ipv4MplsVpnUnicast { nlri } => ipv4_mpls_vpn_routes(nlri),
        MpUnreach::Ipv6MplsVpnUnicast { nlri } => ipv6_mpls_vpn_routes(nlri),
        MpUnreach::L2Evpn { nlri } => l2_evpn_routes(nlri),
        _ => Vec::new(),
    }
}

fn ipv4_unicast_routes(
    nlri: &[netgauze_bgp_pkt::nlri::Ipv4UnicastAddress],
    rd: u64,
) -> Vec<NlriRoute> {
    nlri.iter()
        .map(|value| NlriRoute {
            prefix: IpNet::V4(value.network().address()),
            route_key: format!(
                "ipv4-unicast|path_id={}",
                super::path_id_component(value.path_id())
            ),
            rd,
        })
        .collect()
}

fn ipv6_unicast_routes(
    nlri: &[netgauze_bgp_pkt::nlri::Ipv6UnicastAddress],
    rd: u64,
) -> Vec<NlriRoute> {
    nlri.iter()
        .map(|value| NlriRoute {
            prefix: IpNet::V6(value.network().address()),
            route_key: format!(
                "ipv6-unicast|path_id={}",
                super::path_id_component(value.path_id())
            ),
            rd,
        })
        .collect()
}

pub(crate) fn ipv4_mpls_label_routes(
    nlri: &[netgauze_bgp_pkt::nlri::Ipv4NlriMplsLabelsAddress],
    rd: u64,
) -> Vec<NlriRoute> {
    nlri.iter()
        .map(|value| NlriRoute {
            prefix: IpNet::V4(value.prefix()),
            route_key: format!(
                "ipv4-nlri-mpls-labels|path_id={}",
                super::path_id_component(value.path_id())
            ),
            rd,
        })
        .collect()
}

fn ipv6_mpls_label_routes(
    nlri: &[netgauze_bgp_pkt::nlri::Ipv6NlriMplsLabelsAddress],
    rd: u64,
) -> Vec<NlriRoute> {
    nlri.iter()
        .map(|value| NlriRoute {
            prefix: IpNet::V6(value.prefix()),
            route_key: format!(
                "ipv6-nlri-mpls-labels|path_id={}",
                super::path_id_component(value.path_id())
            ),
            rd,
        })
        .collect()
}

pub(crate) fn ipv4_mpls_vpn_routes(
    nlri: &[netgauze_bgp_pkt::nlri::Ipv4MplsVpnUnicastAddress],
) -> Vec<NlriRoute> {
    nlri.iter()
        .map(|value| NlriRoute {
            prefix: IpNet::V4(value.network().address()),
            route_key: format!(
                "ipv4-mpls-vpn-unicast|path_id={}|rd={}",
                super::path_id_component(value.path_id()),
                value.rd()
            ),
            rd: super::route_distinguisher_to_u64(Some(value.rd())),
        })
        .collect()
}

fn ipv6_mpls_vpn_routes(
    nlri: &[netgauze_bgp_pkt::nlri::Ipv6MplsVpnUnicastAddress],
) -> Vec<NlriRoute> {
    nlri.iter()
        .map(|value| NlriRoute {
            prefix: IpNet::V6(value.network().address()),
            route_key: format!(
                "ipv6-mpls-vpn-unicast|path_id={}|rd={}",
                super::path_id_component(value.path_id()),
                value.rd()
            ),
            rd: super::route_distinguisher_to_u64(Some(value.rd())),
        })
        .collect()
}

pub(crate) fn l2_evpn_routes(nlri: &[L2EvpnAddress]) -> Vec<NlriRoute> {
    nlri.iter()
        .filter_map(|value| match value.route() {
            L2EvpnRoute::IpPrefixRoute(L2EvpnIpPrefixRoute::V4(route)) => Some(NlriRoute {
                prefix: IpNet::V4(route.prefix()),
                route_key: format!(
                    "l2-evpn-ip-prefix|path_id={}|rd={}|ip_version=4",
                    super::path_id_component(value.path_id()),
                    route.rd()
                ),
                rd: super::route_distinguisher_to_u64(Some(route.rd())),
            }),
            L2EvpnRoute::IpPrefixRoute(L2EvpnIpPrefixRoute::V6(route)) => Some(NlriRoute {
                prefix: IpNet::V6(route.prefix()),
                route_key: format!(
                    "l2-evpn-ip-prefix|path_id={}|rd={}|ip_version=6",
                    super::path_id_component(value.path_id()),
                    route.rd()
                ),
                rd: super::route_distinguisher_to_u64(Some(route.rd())),
            }),
            _ => None,
        })
        .collect()
}
