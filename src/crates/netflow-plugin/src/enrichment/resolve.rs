use super::*;

impl FlowEnricher {
    pub(super) fn lookup_routing(
        &self,
        address: IpAddr,
        preferred_next_hop: Option<IpAddr>,
        preferred_exporter: Option<IpAddr>,
    ) -> Option<StaticRoutingEntry> {
        if let Some(runtime) = &self.dynamic_routing
            && let Some(dynamic) = runtime.lookup(address, preferred_next_hop, preferred_exporter)
        {
            return Some(dynamic);
        }
        self.static_routing.lookup(address).cloned()
    }

    pub(super) fn resolve_network_attributes(&self, address: IpAddr) -> Option<NetworkAttributes> {
        let mut resolved = self
            .geoip
            .as_ref()
            .and_then(|geoip| geoip.lookup(address))
            .unwrap_or_default();

        // Merge all matching prefixes in ascending prefix length order (least specific first,
        // so more specific prefixes override). Network sources (priority 0) are applied before
        // static config (priority 1) at the same prefix length.
        let runtime_matches = self
            .network_sources_runtime
            .as_ref()
            .map(|runtime| runtime.matching_attributes_ascending(address))
            .unwrap_or_default();

        // Merge-walk: runtime (source_priority=0) then static (source_priority=1) at each level.
        let mut rt_iter = runtime_matches.iter().peekable();
        let mut static_iter = self.networks.matching_entries_ascending(address).peekable();

        loop {
            let rt_len = rt_iter.peek().map(|(len, _)| *len);
            let st_len = static_iter.peek().map(|e| e.prefix.prefix_len());

            match (rt_len, st_len) {
                (None, None) => break,
                (Some(_), None) => {
                    let (_, attrs) = rt_iter.next().unwrap();
                    resolved.merge_from(attrs);
                }
                (None, Some(_)) => {
                    let entry = static_iter.next().unwrap();
                    resolved.merge_from(&entry.value);
                }
                (Some(r), Some(s)) if r < s => {
                    let (_, attrs) = rt_iter.next().unwrap();
                    resolved.merge_from(attrs);
                }
                (Some(r), Some(s)) if r == s => {
                    // Same prefix length: runtime (priority 0) before static (priority 1).
                    let (_, attrs) = rt_iter.next().unwrap();
                    resolved.merge_from(attrs);
                }
                _ => {
                    let entry = static_iter.next().unwrap();
                    resolved.merge_from(&entry.value);
                }
            }
        }

        if resolved.is_empty() {
            None
        } else {
            Some(resolved)
        }
    }

    pub(super) fn get_as_number(&self, flow_as: u32, routing_as: u32, flow_net_mask: u8) -> u32 {
        let mut asn = 0_u32;
        for provider in &self.asn_providers {
            if asn != 0 {
                break;
            }
            match provider {
                AsnProviderConfig::Geoip => {
                    // Akvorado parity: GeoIP is a terminal shortcut in provider chain.
                    return 0;
                }
                AsnProviderConfig::Flow => asn = flow_as,
                AsnProviderConfig::FlowExceptPrivate => {
                    asn = flow_as;
                    if is_private_as(asn) {
                        asn = 0;
                    }
                }
                AsnProviderConfig::FlowExceptDefaultRoute => {
                    asn = flow_as;
                    if flow_net_mask == 0 {
                        asn = 0;
                    }
                }
                AsnProviderConfig::Routing => asn = routing_as,
                AsnProviderConfig::RoutingExceptPrivate => {
                    asn = routing_as;
                    if is_private_as(asn) {
                        asn = 0;
                    }
                }
            }
        }
        asn
    }

    pub(super) fn get_net_mask(&self, flow_mask: u8, routing_mask: u8) -> u8 {
        let mut mask = 0_u8;
        for provider in &self.net_providers {
            if mask != 0 {
                break;
            }
            match provider {
                NetProviderConfig::Flow => mask = flow_mask,
                NetProviderConfig::Routing => mask = routing_mask,
            }
        }
        mask
    }

    pub(super) fn get_next_hop(
        &self,
        flow_next_hop: Option<IpAddr>,
        routing_next_hop: Option<IpAddr>,
    ) -> Option<IpAddr> {
        let mut next_hop: Option<IpAddr> = None;
        for provider in &self.net_providers {
            if let Some(value) = next_hop
                && !value.is_unspecified()
            {
                break;
            }
            match provider {
                NetProviderConfig::Flow => {
                    next_hop = flow_next_hop;
                }
                NetProviderConfig::Routing => {
                    next_hop = routing_next_hop;
                }
            }
        }
        next_hop.filter(|addr| !addr.is_unspecified())
    }
}
