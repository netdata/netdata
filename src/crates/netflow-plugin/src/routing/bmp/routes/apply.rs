use super::*;

pub(crate) fn apply_update(
    peer: &DynamicRoutingPeerKey,
    peer_as: u32,
    peer_type: BmpPeerType,
    peer_rd: u64,
    update: &BgpUpdateMessage,
    config: &RoutingDynamicBmpConfig,
    accepted_rds: &HashSet<u64>,
    runtime: &DynamicRoutingRuntime,
) {
    let mut flow_next_hop: Option<IpAddr> = None;
    let mut as_path: Vec<u32> = Vec::new();
    let mut communities: Vec<u32> = Vec::new();
    let mut large_communities: Vec<(u32, u32, u32)> = Vec::new();

    for attribute in update.path_attributes() {
        match attribute.value() {
            PathAttributeValue::NextHop(next_hop) => {
                flow_next_hop = Some(IpAddr::V4(next_hop.next_hop()));
            }
            PathAttributeValue::AsPath(path) => {
                as_path = flatten_as_path(path);
            }
            PathAttributeValue::As4Path(path) => {
                as_path = flatten_as4_path(path);
            }
            PathAttributeValue::Communities(values) => {
                communities = values
                    .communities()
                    .iter()
                    .map(|value| value.value())
                    .collect();
            }
            PathAttributeValue::LargeCommunities(values) => {
                large_communities = values
                    .communities()
                    .iter()
                    .map(|value| {
                        (
                            value.global_admin(),
                            value.local_data1(),
                            value.local_data2(),
                        )
                    })
                    .collect();
            }
            _ => {}
        }
    }

    let asn = if config.collect_asns {
        as_path.last().copied().unwrap_or(peer_as)
    } else {
        0
    };
    if !config.collect_as_paths {
        as_path.clear();
    }
    if !config.collect_communities {
        communities.clear();
        large_communities.clear();
    }

    if is_l3vpn_peer_type(peer_type) || is_rd_accepted(accepted_rds, 0) {
        for (prefix, path_id) in update.nlri().iter().map(ipv4_unicast_to_prefix) {
            let route_key = format!("ipv4-unicast|path_id={}", path_id_component(path_id));
            runtime.upsert(DynamicRoutingUpdate {
                peer: peer.clone(),
                prefix,
                route_key,
                next_hop: flow_next_hop,
                asn,
                as_path: as_path.clone(),
                communities: communities.clone(),
                large_communities: large_communities.clone(),
            });
        }

        for (prefix, path_id) in update.withdraw_routes().iter().map(ipv4_unicast_to_prefix) {
            runtime.withdraw(
                peer,
                prefix,
                &format!("ipv4-unicast|path_id={}", path_id_component(path_id)),
            );
        }
    }

    for attribute in update.path_attributes() {
        match attribute.value() {
            PathAttributeValue::MpReach(reach) => {
                let (next_hop, routes) = mp_reach_to_routes(reach, peer_rd);
                for route in routes {
                    if !is_l3vpn_peer_type(peer_type) && !is_rd_accepted(accepted_rds, route.rd) {
                        continue;
                    }
                    runtime.upsert(DynamicRoutingUpdate {
                        peer: peer.clone(),
                        prefix: route.prefix,
                        route_key: route.route_key,
                        next_hop,
                        asn,
                        as_path: as_path.clone(),
                        communities: communities.clone(),
                        large_communities: large_communities.clone(),
                    });
                }
            }
            PathAttributeValue::MpUnreach(unreach) => {
                for route in mp_unreach_to_routes(unreach, peer_rd) {
                    if !is_l3vpn_peer_type(peer_type) && !is_rd_accepted(accepted_rds, route.rd) {
                        continue;
                    }
                    runtime.withdraw(peer, route.prefix, &route.route_key);
                }
            }
            _ => {}
        }
    }
}
