use super::context::ResolvedFlowContext;
use super::*;

impl FlowEnricher {
    pub(super) fn resolve_flow_context(
        &self,
        exporter_ip: IpAddr,
        src_addr: Option<IpAddr>,
        dst_addr: Option<IpAddr>,
        flow_next_hop: Option<IpAddr>,
        source_flow_mask: u8,
        dest_flow_mask: u8,
        source_flow_as: u32,
        dest_flow_as: u32,
    ) -> ResolvedFlowContext {
        let source_routing =
            src_addr.and_then(|address| self.lookup_routing(address, None, Some(exporter_ip)));
        let dest_routing = dst_addr
            .and_then(|address| self.lookup_routing(address, flow_next_hop, Some(exporter_ip)));

        let source_routing_as = source_routing.as_ref().map_or(0, |entry| entry.asn);
        let dest_routing_as = dest_routing.as_ref().map_or(0, |entry| entry.asn);
        let source_routing_mask = source_routing.as_ref().map_or(0, |entry| entry.net_mask);
        let dest_routing_mask = dest_routing.as_ref().map_or(0, |entry| entry.net_mask);
        let routing_next_hop = dest_routing.as_ref().and_then(|entry| entry.next_hop);

        let source_mask = self.get_net_mask(source_flow_mask, source_routing_mask);
        let dest_mask = self.get_net_mask(dest_flow_mask, dest_routing_mask);
        let source_network = src_addr.and_then(|address| self.resolve_network_attributes(address));
        let dest_network = dst_addr.and_then(|address| self.resolve_network_attributes(address));
        let source_as = apply_network_asn_override(
            self.get_as_number(source_flow_as, source_routing_as, source_mask),
            source_network.as_ref().map_or(0, |attrs| attrs.asn),
        );
        let dest_as = apply_network_asn_override(
            self.get_as_number(dest_flow_as, dest_routing_as, dest_mask),
            dest_network.as_ref().map_or(0, |attrs| attrs.asn),
        );

        ResolvedFlowContext {
            source_network,
            dest_network,
            dest_routing,
            source_mask,
            dest_mask,
            source_as,
            dest_as,
            next_hop: self.get_next_hop(flow_next_hop, routing_next_hop),
        }
    }
}
