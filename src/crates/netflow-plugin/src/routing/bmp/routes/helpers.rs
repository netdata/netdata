use super::*;

pub(crate) fn path_id_component(path_id: Option<u32>) -> String {
    path_id
        .map(|value| value.to_string())
        .unwrap_or_else(|| "0".to_string())
}

pub(crate) fn is_l3vpn_peer_type(peer_type: BmpPeerType) -> bool {
    matches!(peer_type, BmpPeerType::RdInstancePeer { .. })
}

pub(crate) fn route_distinguisher_to_u64(rd: Option<RouteDistinguisher>) -> u64 {
    rd.map(u64::from).unwrap_or(0)
}

pub(crate) fn is_rd_accepted(accepted_rds: &HashSet<u64>, rd: u64) -> bool {
    accepted_rds.is_empty() || accepted_rds.contains(&rd)
}
