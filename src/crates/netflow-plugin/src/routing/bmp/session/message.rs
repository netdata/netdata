use super::super::{
    routes::{apply_update, is_l3vpn_peer_type, is_rd_accepted, route_distinguisher_to_u64},
    *,
};

pub(super) fn process_bmp_message(
    exporter: SocketAddr,
    session_id: u64,
    message: BmpMessageValue,
    config: &RoutingDynamicBmpConfig,
    accepted_rds: &HashSet<u64>,
    runtime: &DynamicRoutingRuntime,
) {
    match message {
        BmpMessageValue::PeerDownNotification(msg) => {
            let peer = peer_key(exporter, session_id, msg.peer_header());
            runtime.clear_peer(&peer);
        }
        BmpMessageValue::RouteMonitoring(msg) => {
            if let BgpMessage::Update(update) = msg.update_message() {
                let header = msg.peer_header();
                let peer = peer_key(exporter, session_id, header);
                let peer_rd = route_distinguisher_to_u64(header.rd());
                let l3vpn_peer = is_l3vpn_peer_type(header.peer_type());
                if l3vpn_peer && !is_rd_accepted(accepted_rds, peer_rd) {
                    return;
                }
                apply_update(
                    &peer,
                    header.peer_as(),
                    header.peer_type(),
                    peer_rd,
                    update,
                    config,
                    accepted_rds,
                    runtime,
                );
            }
        }
        _ => {}
    }
}

fn peer_key(exporter: SocketAddr, session_id: u64, header: &PeerHeader) -> DynamicRoutingPeerKey {
    DynamicRoutingPeerKey {
        exporter,
        session_id,
        peer_id: format!(
            "{:?}|{:?}|{:?}|{}|{}",
            header.peer_type(),
            header.rd(),
            header.address(),
            header.peer_as(),
            header.bgp_id()
        ),
    }
}
